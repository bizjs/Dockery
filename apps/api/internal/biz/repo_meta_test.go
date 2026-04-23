package biz

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// --- in-memory fake for RepoMetaRepo --------------------------------

// fakeRepoMetaRepo is goroutine-safe because the refresh worker runs
// on its own goroutine and may touch the repo concurrently with the
// test's read assertions.
type fakeRepoMetaRepo struct {
	mu             sync.Mutex
	upsertCalls    int
	getCalls       int
	deleteCalls    int
	incrementCalls int
	// incrementMiss controls whether IncrementPull reports "no row"
	// for the next call — simulates an un-cached repo.
	incrementMiss bool
	rows          map[string]*RepoMeta
}

func newFakeRepoMetaRepo() *fakeRepoMetaRepo {
	return &fakeRepoMetaRepo{rows: make(map[string]*RepoMeta)}
}

func (f *fakeRepoMetaRepo) Upsert(_ context.Context, m *RepoMeta) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upsertCalls++
	copy := *m
	f.rows[m.Repo] = &copy
	return nil
}

func (f *fakeRepoMetaRepo) Get(_ context.Context, repo string) (*RepoMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	m, ok := f.rows[repo]
	if !ok {
		return nil, ErrRepoMetaNotFound
	}
	return m, nil
}

func (f *fakeRepoMetaRepo) Delete(_ context.Context, repo string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls++
	delete(f.rows, repo)
	return nil
}

func (f *fakeRepoMetaRepo) List(_ context.Context) ([]*RepoMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*RepoMeta, 0, len(f.rows))
	for _, m := range f.rows {
		out = append(out, m)
	}
	return out, nil
}

func (f *fakeRepoMetaRepo) AllRepos(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.rows))
	for r := range f.rows {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeRepoMetaRepo) QueryPage(_ context.Context, filter OverviewFilter) (*OverviewPage, error) {
	// Only need enough to keep callers that happen to use this method
	// working in tests — current biz_test callers don't exercise this
	// path, so a minimal List-and-paginate is fine.
	f.mu.Lock()
	defer f.mu.Unlock()
	all := make([]*RepoMeta, 0, len(f.rows))
	for _, m := range f.rows {
		all = append(all, m)
	}
	return &OverviewPage{Items: all, Total: len(all)}, nil
}

func (f *fakeRepoMetaRepo) IncrementPull(_ context.Context, repo string, at time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.incrementCalls++
	if f.incrementMiss {
		// Simulate "row not found" — the update affects 0 rows.
		return 0, nil
	}
	m, ok := f.rows[repo]
	if !ok {
		return 0, nil
	}
	m.PullCount++
	ts := at.Unix()
	m.LastPulledAt = &ts
	return 1, nil
}

// newTestUsecase builds a RepoMetaUsecase with plausibly-configured but
// unreachable upstream (http://127.0.0.1:1). We don't want the refresh
// worker to actually hit an upstream in these unit tests — we probe the
// observable state (queue, dedup, repo writes) instead.
func newTestUsecase(t *testing.T, repo RepoMetaRepo) *RepoMetaUsecase {
	t.Helper()
	ks, err := NewKeystore(KeystoreConfig{
		PrivatePath: filepath.Join(t.TempDir(), "priv.pem"),
		JWKSPath:    filepath.Join(t.TempDir(), "jwks.json"),
	})
	if err != nil {
		t.Fatalf("keystore: %v", err)
	}
	iss, err := NewTokenIssuer(ks, TokenIssuerConfig{
		Issuer: "dockery-api", Audience: "dockery", TTL: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("token issuer: %v", err)
	}
	fetcher := NewRegistryFetchClient(iss, RegistryUpstreamURL("http://127.0.0.1:1"))
	u := NewRepoMetaUsecase(repo, fetcher, log.DefaultLogger)
	t.Cleanup(u.Close)
	return u
}

// --- tests ----------------------------------------------------------

func TestEnqueueRefresh_Dedup(t *testing.T) {
	// Rapid-fire enqueues for the same repo must collapse into exactly
	// one pending entry — otherwise a noisy push (manifest list + 3
	// children) would schedule 4 refreshes of the same repo.
	repo := newFakeRepoMetaRepo()
	u := newTestUsecase(t, repo)

	// Stop the worker so we can inspect the queue without races. The
	// worker is already started by NewRepoMetaUsecase — immediately
	// close to freeze.
	u.Close()

	for i := 0; i < 20; i++ {
		u.EnqueueRefresh("alice/app")
	}
	// The queue length reflects items waiting for the worker; since
	// the worker is stopped we can read it directly.
	if got := len(u.queue); got != 1 {
		t.Errorf("queue length = %d after 20 enqueues of same repo, want 1", got)
	}
}

func TestEnqueueRefresh_DifferentRepos(t *testing.T) {
	repo := newFakeRepoMetaRepo()
	u := newTestUsecase(t, repo)
	u.Close()

	for _, r := range []string{"a", "b", "c", "a", "b"} {
		u.EnqueueRefresh(r)
	}
	if got := len(u.queue); got != 3 {
		t.Errorf("queue length = %d, want 3 (distinct a/b/c)", got)
	}
}

func TestEnqueueRefresh_EmptyRepoIgnored(t *testing.T) {
	repo := newFakeRepoMetaRepo()
	u := newTestUsecase(t, repo)
	u.Close()

	u.EnqueueRefresh("")
	if got := len(u.queue); got != 0 {
		t.Errorf("empty repo must be ignored; queue=%d", got)
	}
}

func TestIncrementPull_MissingRowTriggersRefresh(t *testing.T) {
	// When the repo has never been refreshed, IncrementPull's underlying
	// update affects zero rows. The usecase must enqueue a refresh so
	// subsequent pulls start counting against a real row.
	repo := newFakeRepoMetaRepo()
	repo.incrementMiss = true
	u := newTestUsecase(t, repo)
	u.Close()

	u.IncrementPull(context.Background(), "ghost/repo")

	// Flush memory through the lock so we see the increment even
	// without a channel handshake. Fake uses a mutex so read-after-write
	// is consistent.
	if repo.incrementCalls != 1 {
		t.Errorf("increment calls = %d, want 1", repo.incrementCalls)
	}
	// The missing-row path should have enqueued a refresh.
	if got := len(u.queue); got != 1 {
		t.Errorf("expected refresh enqueue after missing-row increment; queue=%d", got)
	}
}

func TestIncrementPull_ExistingRowNoRefresh(t *testing.T) {
	repo := newFakeRepoMetaRepo()
	repo.rows["alice/app"] = &RepoMeta{Repo: "alice/app"}
	u := newTestUsecase(t, repo)
	u.Close()

	u.IncrementPull(context.Background(), "alice/app")

	if repo.rows["alice/app"].PullCount != 1 {
		t.Errorf("pull count = %d, want 1", repo.rows["alice/app"].PullCount)
	}
	if got := len(u.queue); got != 0 {
		t.Errorf("hit-row increment must NOT enqueue; queue=%d", got)
	}
}

// TestClose_Idempotent guards against a shutdown bug where double-Close
// panics on a closed-channel cancel.
func TestClose_Idempotent(t *testing.T) {
	repo := newFakeRepoMetaRepo()
	u := newTestUsecase(t, repo)
	// t.Cleanup already registers Close; explicitly calling it twice
	// here proves the second call is a no-op.
	u.Close()
	u.Close()
}

// TestRefreshWorker_DrainsQueue shows that the background worker pulls
// items off the queue and invokes RefreshOne, which (with an unreachable
// upstream) fails but still removes the item from `pending` so a future
// enqueue gets scheduled again.
func TestRefreshWorker_DrainsQueue(t *testing.T) {
	repo := newFakeRepoMetaRepo()
	u := newTestUsecase(t, repo)

	u.EnqueueRefresh("alice/app")

	// Spin briefly until the worker drains the queue. RefreshOne will
	// fail (upstream unreachable) but the worker still removes the
	// pending marker. Total time bounded by refreshBackoff (1s+3s)
	// plus the final failure — cap the wait generously.
	deadline := time.Now().Add(8 * time.Second)
	var drained atomic.Bool
	for time.Now().Before(deadline) {
		u.mu.Lock()
		pendingLen := len(u.pending)
		u.mu.Unlock()
		queueLen := len(u.queue)
		if pendingLen == 0 && queueLen == 0 {
			drained.Store(true)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !drained.Load() {
		t.Fatalf("worker did not drain queue within deadline")
	}
}
