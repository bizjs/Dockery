package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// --- fake upstream --------------------------------------------------

// fakeCatalogServer serves a /v2/_catalog response from a controlled
// list of repos and supports keyset pagination so the reconciler's
// Link-header follow logic is exercised.
func fakeCatalogServer(t *testing.T, repos []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/_catalog" {
			http.NotFound(w, r)
			return
		}
		last := r.URL.Query().Get("last")
		start := 0
		if last != "" {
			for i, repo := range repos {
				if repo == last {
					start = i + 1
					break
				}
			}
		}
		// Serve up to 2 per page so pagination actually fires even
		// with small lists.
		const pageSize = 2
		end := start + pageSize
		if end > len(repos) {
			end = len(repos)
		}
		page := repos[start:end]
		if end < len(repos) {
			// Link header carries the cursor for the next page.
			next := fmt.Sprintf("</v2/_catalog?n=%d&last=%s>; rel=\"next\"",
				pageSize, repos[end-1])
			w.Header().Set("Link", next)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"repositories": page})
	}))
}

// --- in-memory audit (shadow of data/audit.go for tests) -----------

type memoryAuditRepo struct {
	entries []AuditEntry
}

func (m *memoryAuditRepo) Create(_ context.Context, e *AuditEntry) error {
	m.entries = append(m.entries, *e)
	return nil
}
func (m *memoryAuditRepo) Query(_ context.Context, _ AuditFilter) ([]*AuditEntry, int, error) {
	return nil, 0, nil
}

// --- test harness ---------------------------------------------------

func newReconcilerRig(t *testing.T, upstreamURL string, cached []string) (*Reconciler, *fakeRepoMetaRepo, *memoryAuditRepo, *RepoMetaUsecase) {
	t.Helper()

	repo := newFakeRepoMetaRepo()
	for _, r := range cached {
		repo.rows[r] = &RepoMeta{Repo: r}
	}

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

	audit := &memoryAuditRepo{}
	auditUC := NewAuditUsecase(audit, log.DefaultLogger)

	// Refresh worker is pointed at an unreachable upstream so the
	// async refreshes we enqueue can't escape the test box — they'll
	// fail their retries and the queue drains without side effects.
	metaFetcher := NewRegistryFetchClient(iss, RegistryUpstreamURL("http://127.0.0.1:1"))
	metaUC := NewRepoMetaUsecase(repo, metaFetcher, log.DefaultLogger)
	t.Cleanup(metaUC.Close)

	// Reconciler reads from the fake httptest server.
	reconFetcher := NewRegistryFetchClient(iss, RegistryUpstreamURL(upstreamURL))
	r := NewReconciler(metaUC, reconFetcher, auditUC,
		ReconcilerConfig{Interval: time.Hour}, // won't tick during the test
		log.DefaultLogger)

	return r, repo, audit, metaUC
}

// --- tests ----------------------------------------------------------

func TestReconcile_AddsMissingRepos(t *testing.T) {
	// Upstream has repos that aren't in the cache yet (e.g. fresh boot
	// or a push webhook got lost). Reconciler must enqueue refreshes
	// and audit each addition.
	upstream := fakeCatalogServer(t, []string{"alice/app", "bob/tool"})
	t.Cleanup(upstream.Close)

	rec, _, audit, metaUC := newReconcilerRig(t, upstream.URL, nil)
	metaUC.Close() // freeze worker so we can inspect the queue
	rec.ReconcileOnce(context.Background())

	// Both upstream repos should have been enqueued for refresh.
	// Drain the queue into a slice so we don't have to care about
	// ordering vs the map iteration in the reconciler.
	queued := drainQueue(metaUC)
	if len(queued) != 2 {
		t.Errorf("queued %d, want 2 (alice/app, bob/tool)", len(queued))
	}

	var adds int
	for _, e := range audit.entries {
		if e.Action == ActionReconcileAdded {
			adds++
		}
	}
	if adds != 2 {
		t.Errorf("audit add count = %d, want 2", adds)
	}
}

func TestReconcile_RemovesStaleRepos(t *testing.T) {
	// Cache has a repo that no longer exists upstream (the delete
	// webhook was lost or the repo was GC'd out-of-band). Reconciler
	// must delete the row and audit the removal.
	upstream := fakeCatalogServer(t, []string{"alice/app"})
	t.Cleanup(upstream.Close)

	rec, repo, audit, metaUC := newReconcilerRig(t, upstream.URL,
		[]string{"alice/app", "ghost/repo"})
	metaUC.Close()
	rec.ReconcileOnce(context.Background())

	repo.mu.Lock()
	_, ghostStill := repo.rows["ghost/repo"]
	_, aliceStill := repo.rows["alice/app"]
	repo.mu.Unlock()
	if ghostStill {
		t.Errorf("ghost/repo should have been deleted")
	}
	if !aliceStill {
		t.Errorf("alice/app must still be present")
	}

	var removes int
	for _, e := range audit.entries {
		if e.Action == ActionReconcileRemoved {
			removes++
		}
	}
	if removes != 1 {
		t.Errorf("audit remove count = %d, want 1", removes)
	}
}

func TestReconcile_NoDrift_NoAudit(t *testing.T) {
	// Steady state: webhook events kept the cache in sync, reconciler
	// finds nothing to do. Must not write audit rows for every
	// matching repo — those would drown the log.
	upstream := fakeCatalogServer(t, []string{"alice/app"})
	t.Cleanup(upstream.Close)

	rec, _, audit, metaUC := newReconcilerRig(t, upstream.URL,
		[]string{"alice/app"})
	metaUC.Close()
	rec.ReconcileOnce(context.Background())

	if len(audit.entries) != 0 {
		t.Errorf("no drift → no audit entries; got %d", len(audit.entries))
	}
}

func TestReconcile_PaginatedCatalog(t *testing.T) {
	// 5 repos with pageSize=2 forces 3 upstream round-trips. Reconciler
	// must follow Link: rel="next" to fully enumerate before diffing.
	upstream := fakeCatalogServer(t, []string{"a", "b", "c", "d", "e"})
	t.Cleanup(upstream.Close)

	rec, _, _, metaUC := newReconcilerRig(t, upstream.URL, nil)
	metaUC.Close()
	rec.ReconcileOnce(context.Background())

	queued := drainQueue(metaUC)
	if len(queued) != 5 {
		t.Errorf("queued %d repos, want 5 after pagination", len(queued))
	}
}

// --- helpers --------------------------------------------------------

// drainQueue snapshots the usecase's pending-refresh set and clears it.
// Returns repos in sorted order so assertions are deterministic. Safe
// to call only after the worker has been stopped (metaUC.Close).
func drainQueue(u *RepoMetaUsecase) []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]string, 0, len(u.pending))
	for r := range u.pending {
		out = append(out, r)
	}
	for r := range u.pending {
		delete(u.pending, r)
	}
	sort.Strings(out)
	return out
}

