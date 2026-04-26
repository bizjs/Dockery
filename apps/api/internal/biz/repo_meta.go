package biz

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"api/internal/data/ent/schema"
	"api/internal/util/registryfetch"

	"github.com/go-kratos/kratos/v2/log"
	"golang.org/x/mod/semver"
)

// RepoMeta is the biz-layer view of the denormalized per-repository
// snapshot powering the Catalog page. Mirrors the ent schema 1:1 but
// exposes unix-second timestamps. Platforms is the raw schema type
// (shared across layers) — the service layer still copies it into its
// own response DTO rather than re-exporting, so API clients never see
// the ent package's shape.
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

// OverviewSort selects which column to sort by in QueryPage.
type OverviewSort int

const (
	OverviewSortName OverviewSort = iota
	OverviewSortUpdated
	OverviewSortSize
	OverviewSortTagCount
)

// OverviewDir selects sort direction.
type OverviewDir int

const (
	OverviewAsc OverviewDir = iota
	OverviewDesc
)

// OverviewFilter is the input to QueryPage. Zero values mean sensible
// defaults (name asc, first page of 50).
type OverviewFilter struct {
	// Query is a case-insensitive substring match on repo name.
	// Empty string = no search filter.
	Query string
	// Sort is which column to ORDER BY.
	Sort OverviewSort
	// Direction is asc/desc. Sort is stabilized by a secondary repo-asc
	// so results don't reshuffle across equal keys.
	Direction OverviewDir
	// Page is 0-based.
	Page int
	// PageSize is the LIMIT. Clamped to [1, 500] by the biz layer.
	PageSize int
}

// OverviewPage is the result of QueryPage.
type OverviewPage struct {
	Items []*RepoMeta
	// Total is the count after filter but before pagination, so the
	// UI can render "Showing X–Y of N".
	Total int
}

// RepoMetaRepo is the data-layer contract. Implemented in
// internal/data/repo_meta.go.
type RepoMetaRepo interface {
	Upsert(ctx context.Context, m *RepoMeta) error
	Get(ctx context.Context, repo string) (*RepoMeta, error)
	Delete(ctx context.Context, repo string) error
	List(ctx context.Context) ([]*RepoMeta, error)
	AllRepos(ctx context.Context) ([]string, error)
	// QueryPage filters/sorts/paginates at the SQL layer for the
	// Overview endpoint. Used when no per-user pattern filter applies
	// (admin users or non-admins with no patterns) — the pattern case
	// still goes through List() and in-memory filtering because the
	// segment-aware glob semantics don't map cleanly onto SQLite.
	QueryPage(ctx context.Context, f OverviewFilter) (*OverviewPage, error)
	// IncrementPull bumps pull_count + last_pulled_at. Returns the
	// number of rows affected — zero means the repo isn't in cache yet
	// (never refreshed). Callers decide whether to enqueue a refresh
	// in that case.
	IncrementPull(ctx context.Context, repo string, at time.Time) (int, error)
}

// ErrRepoMetaNotFound is surfaced by Get when the row is missing.
var ErrRepoMetaNotFound = errors.New("repo_meta: not found")

// RepoMetaUsecase owns the write path to the cache: refreshes triggered
// by webhook events, reconciler diffs, and explicit admin calls all
// funnel into the single deduplicated refresh worker so the backend
// never hammers the upstream registry with concurrent fetches for the
// same repository.
//
// Pending-set design: `pending` is the source of truth for "what needs
// a refresh", and `signal` is a capacity-1 wake-up channel. Enqueues
// just add to the set and poke the signal; the worker drains the set on
// every wake-up. There is no per-item channel buffer, so an enqueue
// burst can never overflow and lose work — the bound is the number of
// distinct repos in the registry, which is what we want.
type RepoMetaUsecase struct {
	repo     RepoMetaRepo
	registry *registryfetch.Client
	logger   *log.Helper

	// Refresh worker plumbing.
	signal  chan struct{}
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
	registry *registryfetch.Client,
	logger log.Logger,
) *RepoMetaUsecase {
	ctx, cancel := context.WithCancel(context.Background())
	u := &RepoMetaUsecase{
		repo:         repo,
		registry:     registry,
		logger:       log.NewHelper(log.With(logger, "module", "biz/repo_meta")),
		signal:       make(chan struct{}, 1),
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

// QueryPage is the read-path for /api/registry/overview when the caller
// has no per-user pattern restriction (admin or non-admin-with-no-
// patterns). Clamps PageSize and delegates to the DB layer.
func (u *RepoMetaUsecase) QueryPage(ctx context.Context, f OverviewFilter) (*OverviewPage, error) {
	if f.PageSize <= 0 {
		f.PageSize = 50
	}
	if f.PageSize > 500 {
		f.PageSize = 500
	}
	if f.Page < 0 {
		f.Page = 0
	}
	return u.repo.QueryPage(ctx, f)
}

// DeleteRepo is called by the reconciler when a repo has vanished
// upstream. Webhook-driven deletes go through EnqueueRefresh so the
// refresh path can detect "zero tags left" and delete as part of its
// own logic — keeping both paths converging on the same state.
func (u *RepoMetaUsecase) DeleteRepo(ctx context.Context, repo string) error {
	return u.repo.Delete(ctx, repo)
}

// IncrementPull bumps pull_count + last_pulled_at. If the row doesn't
// exist yet (the repo has only ever been pulled, never pushed during
// dockery's lifetime), enqueue a refresh so it gets materialized and
// subsequent pulls start counting. Errors on the counter update are
// logged + swallowed — pull events are high-frequency and a missing
// pull count is a statistics gap, not a correctness problem.
func (u *RepoMetaUsecase) IncrementPull(ctx context.Context, repo string) {
	n, err := u.repo.IncrementPull(ctx, repo, time.Now())
	if err != nil {
		u.logger.Debugf("increment pull %s: %v", repo, err)
		return
	}
	if n == 0 {
		u.EnqueueRefresh(repo)
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
	_, already := u.pending[repo]
	u.pending[repo] = struct{}{}
	u.mu.Unlock()
	if already {
		// Already on the worker's todo list; no need to re-poke.
		return
	}
	// Non-blocking poke: signal has capacity 1 so a coalesced burst of
	// wake-ups collapses into a single iteration of the worker, which
	// drains everything currently in `pending`.
	select {
	case u.signal <- struct{}{}:
	default:
	}
}

func (u *RepoMetaUsecase) refreshWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-u.signal:
		}
		// Drain the pending set. Pop one repo at a time under the lock,
		// release the lock for the network call, then loop. New
		// EnqueueRefresh calls during the network call land in `pending`
		// and we'll see them on the next iteration without needing
		// another wake-up.
		for {
			if ctx.Err() != nil {
				return
			}
			u.mu.Lock()
			var repo string
			for r := range u.pending {
				repo = r
				break
			}
			if repo == "" {
				u.mu.Unlock()
				break
			}
			delete(u.pending, repo)
			u.mu.Unlock()

			if err := u.RefreshOne(ctx, repo); err != nil {
				u.logger.Warnf("refresh %s: %v", repo, err)
			}
		}
	}
}

// refreshBackoff holds the short retry schedule for RefreshOne.
// Brief transient failures (registry restart, momentary network hiccup)
// resolve within a few seconds — retry before giving up to the
// 30-minute reconciler. Not exponential-to-infinity on purpose: the
// reconciler is the real fallback.
var refreshBackoff = []time.Duration{1 * time.Second, 3 * time.Second}

// RefreshOne synchronously re-fetches `repo`'s meta from upstream and
// writes it to the cache. Exposed (not just called from the worker) so
// the reconciler + admin "rebuild-cache" CLI can drive it too. If the
// repo has zero tags, the cache row is deleted.
//
// Upstream fetch errors retry on a short backoff before propagating —
// decode errors and empty-tag outcomes don't, since they'd just repeat
// the same failure.
func (u *RepoMetaUsecase) RefreshOne(ctx context.Context, repo string) error {
	var lastErr error
	for attempt := 0; ; attempt++ {
		err := u.refreshOnce(ctx, repo)
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt >= len(refreshBackoff) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(refreshBackoff[attempt]):
		}
	}
}

// refreshOnce is the single-attempt body of RefreshOne. Split out so
// the retry loop stays readable.
func (u *RepoMetaUsecase) refreshOnce(ctx context.Context, repo string) error {
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

// pickRepresentativeTag chooses which tag's manifest the cache row
// represents. Registry doesn't expose push time, so we layer three
// heuristics in order of confidence:
//
//  1. `latest` if present — explicit operator intent.
//  2. The highest semver tag if any tags parse as semver. Uses
//     golang.org/x/mod/semver so v0.0.10 correctly orders after v0.0.9
//     (lexicographic alone gets this wrong because '1' < '9'). Tags
//     without the leading 'v' are accepted by normalising before
//     comparison; the original tag string is what we return.
//  3. Lexicographic max as a last resort for non-semver schemes
//     (date-prefixed builds, branch-named tags). Imperfect for
//     numeric date schemes, but matches the previous behaviour.
func pickRepresentativeTag(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	for _, t := range tags {
		if t == "latest" {
			return "latest"
		}
	}

	// Semver pass — track both the canonical (v-prefixed) form for
	// comparison and the original tag string for the return value.
	var bestTag, bestSemver string
	for _, t := range tags {
		canon := t
		if !strings.HasPrefix(canon, "v") {
			canon = "v" + canon
		}
		if !semver.IsValid(canon) {
			continue
		}
		if bestSemver == "" || semver.Compare(canon, bestSemver) > 0 {
			bestSemver = canon
			bestTag = t
		}
	}
	if bestTag != "" {
		return bestTag
	}

	// Fallback: linear scan for the lexicographic max so we don't
	// mutate the caller's slice via sort.Strings.
	max := tags[0]
	for _, t := range tags[1:] {
		if t > max {
			max = t
		}
	}
	return max
}
