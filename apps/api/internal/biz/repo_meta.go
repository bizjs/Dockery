package biz

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"api/internal/data/ent/schema"

	"github.com/go-kratos/kratos/v2/log"
)

// RepoMeta is the biz-layer view of the denormalized per-repository
// snapshot powering the Catalog page. Mirrors the ent schema 1:1 but
// exposes unix-second timestamps and concrete Go types so service/ and
// UI layers don't import schema.PlatformInfo transitively.
type RepoMeta struct {
	Repo         string
	LatestTag    string
	TagCount     int
	Size         int64
	Created      string // ISO 8601
	Platforms    []schema.PlatformInfo
	PullCount    int64
	LastPulledAt *int64 // unix seconds; nil when the repo has never been pulled since caching started
	RefreshedAt  int64  // unix seconds
}

// RepoMetaRepo is the data-layer contract. Implemented in
// internal/data/repo_meta.go.
type RepoMetaRepo interface {
	Upsert(ctx context.Context, m *RepoMeta) error
	Get(ctx context.Context, repo string) (*RepoMeta, error)
	Delete(ctx context.Context, repo string) error
	List(ctx context.Context) ([]*RepoMeta, error)
	AllRepos(ctx context.Context) ([]string, error)
	IncrementPull(ctx context.Context, repo string, at time.Time) error
}

// ErrRepoMetaNotFound is surfaced by Get when the row is missing.
var ErrRepoMetaNotFound = errors.New("repo_meta: not found")

// RepoMetaUsecase owns the write path to the cache: refreshes triggered
// by webhook events, reconciler diffs, and explicit admin calls all
// funnel into the single deduplicated refresh worker so the backend
// never hammers the upstream registry with concurrent fetches for the
// same repository.
type RepoMetaUsecase struct {
	repo        RepoMetaRepo
	tokens      *TokenIssuer
	upstreamURL string
	client      *http.Client
	logger      *log.Helper

	// Refresh worker plumbing.
	queue   chan string
	mu      sync.Mutex
	pending map[string]struct{}
	// workerCancel stops the background worker; bound to the usecase's
	// root context so shutdown drains it cleanly.
	workerCancel context.CancelFunc
}

// NewRepoMetaUsecase boots the usecase and starts its background
// refresh worker. The worker is intentionally long-lived: webhook
// events arrive throughout the process lifetime, and the single-goroutine
// model guarantees we never race on the same repo's state.
func NewRepoMetaUsecase(
	repo RepoMetaRepo,
	tokens *TokenIssuer,
	upstream RegistryUpstreamURL,
	logger log.Logger,
) *RepoMetaUsecase {
	ctx, cancel := context.WithCancel(context.Background())
	u := &RepoMetaUsecase{
		repo:         repo,
		tokens:       tokens,
		upstreamURL:  string(upstream),
		client:       &http.Client{Timeout: 30 * time.Second},
		logger:       log.NewHelper(log.With(logger, "module", "biz/repo_meta")),
		queue:        make(chan string, 256),
		pending:      make(map[string]struct{}),
		workerCancel: cancel,
	}
	go u.refreshWorker(ctx)
	return u
}

// Close stops the background refresh worker. Idempotent — callers
// can invoke on any cleanup path.
func (u *RepoMetaUsecase) Close() {
	if u.workerCancel != nil {
		u.workerCancel()
	}
}

// --- read-side (mirrors the ent repo, used by service/overview) ---

func (u *RepoMetaUsecase) List(ctx context.Context) ([]*RepoMeta, error) {
	return u.repo.List(ctx)
}

func (u *RepoMetaUsecase) Get(ctx context.Context, repo string) (*RepoMeta, error) {
	return u.repo.Get(ctx, repo)
}

func (u *RepoMetaUsecase) AllRepos(ctx context.Context) ([]string, error) {
	return u.repo.AllRepos(ctx)
}

// DeleteRepo is called by the reconciler when a repo has vanished
// upstream. Webhook-driven deletes go through EnqueueRefresh so the
// refresh path can detect "zero tags left" and delete as part of its
// own logic — keeping both paths converging on the same state.
func (u *RepoMetaUsecase) DeleteRepo(ctx context.Context, repo string) error {
	return u.repo.Delete(ctx, repo)
}

// IncrementPull bumps pull_count + last_pulled_at. Best-effort: errors
// are logged and swallowed because pull events are high-frequency and
// not worth propagating (a missing pull count is a statistics gap, not
// a correctness problem).
func (u *RepoMetaUsecase) IncrementPull(ctx context.Context, repo string) {
	if err := u.repo.IncrementPull(ctx, repo, time.Now()); err != nil {
		u.logger.Debugf("increment pull %s: %v", repo, err)
	}
}

// --- refresh worker ---

// EnqueueRefresh asks the background worker to re-fetch `repo`'s meta
// from upstream. Deduplicates: if a refresh for the same repo is
// already pending, this is a no-op. Safe to call from any goroutine.
func (u *RepoMetaUsecase) EnqueueRefresh(repo string) {
	if repo == "" {
		return
	}
	u.mu.Lock()
	if _, already := u.pending[repo]; already {
		u.mu.Unlock()
		return
	}
	u.pending[repo] = struct{}{}
	u.mu.Unlock()

	select {
	case u.queue <- repo:
	default:
		// Queue at capacity — drop the pending marker so a future
		// enqueue retries. Losing one refresh event is fine; the
		// reconciler will pick up the drift within 30 min.
		u.mu.Lock()
		delete(u.pending, repo)
		u.mu.Unlock()
		u.logger.Warnf("refresh queue full, dropping: %s", repo)
	}
}

func (u *RepoMetaUsecase) refreshWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case repo := <-u.queue:
			u.mu.Lock()
			delete(u.pending, repo)
			u.mu.Unlock()
			if err := u.RefreshOne(ctx, repo); err != nil {
				u.logger.Warnf("refresh %s: %v", repo, err)
			}
		}
	}
}

// RefreshOne synchronously re-fetches `repo`'s meta from upstream and
// writes it to the cache. Exposed (not just called from the worker) so
// the reconciler + admin "rebuild-cache" CLI can drive it too. If the
// repo has zero tags, the cache row is deleted.
func (u *RepoMetaUsecase) RefreshOne(ctx context.Context, repo string) error {
	tags, err := u.fetchTags(ctx, repo)
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		// Repo exists in catalog but has no tags (all deleted);
		// distribution's catalog will drop it eventually. Don't
		// keep a stale row around.
		if delErr := u.repo.Delete(ctx, repo); delErr != nil {
			u.logger.Debugf("delete empty repo %s: %v", repo, delErr)
		}
		return nil
	}
	latestTag := pickRepresentativeTag(tags)
	meta, err := u.fetchRepoMeta(ctx, repo, latestTag)
	if err != nil {
		return err
	}
	meta.Repo = repo
	meta.LatestTag = latestTag
	meta.TagCount = len(tags)
	meta.RefreshedAt = time.Now().Unix()
	return u.repo.Upsert(ctx, meta)
}

// pickRepresentativeTag is the same heuristic the UI used: prefer
// "latest" if present, otherwise take the lexicographically last tag.
// Registry doesn't expose push time so this is the best we can do
// without a full per-tag manifest scan.
func pickRepresentativeTag(tags []string) string {
	for _, t := range tags {
		if t == "latest" {
			return "latest"
		}
	}
	return tags[len(tags)-1]
}
