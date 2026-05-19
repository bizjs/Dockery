package biz

import (
	"context"
	"sync"
	"time"

	"api/internal/util/registryfetch"

	"github.com/go-kratos/kratos/v2/log"
)

// Reconciler bridges any drift between distribution's /v2/_catalog and
// our repo_meta cache. Runs once on startup + every 30 minutes
// thereafter. For every discrepancy (upstream has a repo we don't, or
// vice versa) it enqueues a refresh / issues a delete and writes a row
// to the audit log — making event loss visible to operators rather
// than a silent cache slowly going out of date.
type Reconciler struct {
	meta     *RepoMetaUsecase
	registry *registryfetch.Client
	audit    *AuditUsecase
	logger   *log.Helper
	interval time.Duration

	once   sync.Once
	cancel context.CancelFunc
}

// ReconcilerConfig exposes the one knob callers might want to tweak.
// 30 minutes is the default — long enough that a well-behaved webhook
// setup never triggers it, short enough that bugs / restarts recover
// within a working-hour window.
type ReconcilerConfig struct {
	Interval time.Duration
}

// NewReconciler constructs the reconciler and is wired via the biz
// ProviderSet. Its background loop doesn't start until Start() is
// called, so tests can instantiate without a goroutine side effect.
func NewReconciler(
	meta *RepoMetaUsecase,
	registry *registryfetch.Client,
	audit *AuditUsecase,
	cfg ReconcilerConfig,
	logger log.Logger,
) *Reconciler {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	return &Reconciler{
		meta:     meta,
		registry: registry,
		audit:    audit,
		logger:   log.NewHelper(log.With(logger, "module", "biz/reconciler")),
		interval: interval,
	}
}

// Start kicks off the async first scan and the periodic ticker. Safe to
// call multiple times; only the first call has effect.
func (r *Reconciler) Start() {
	r.once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		r.cancel = cancel
		go r.run(ctx)
	})
}

// Stop halts the loop; typically called from the app's cleanup func.
func (r *Reconciler) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *Reconciler) run(ctx context.Context) {
	// First pass delayed a touch so dockery-api + registry are both
	// ready. 3s is longer than supervisord's startsecs for registry
	// but comfortably under the 30s wait-for-jwks timeout.
	select {
	case <-ctx.Done():
		return
	case <-time.After(3 * time.Second):
	}

	r.ReconcileOnce(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.ReconcileOnce(ctx)
		}
	}
}

// staleRefreshAge is how long a row may go without being refreshed
// before the reconciler proactively re-fetches it. Bounds the worst-
// case "frozen at old value" window — relevant when an algorithm change
// (e.g. semver-aware pickRepresentativeTag) would otherwise leave
// historical rows displaying stale derived data forever.
//
// 24h means a row touched by any webhook within the last day is left
// alone; only the truly cold ones get re-fetched. At a 30-min
// reconcile cadence that's at most ~1/48 of the cache per cycle, so
// upstream load stays light even for big registries.
const staleRefreshAge = 24 * time.Hour

// ReconcileOnce walks /v2/_catalog, compares to repo_meta, and reacts
// to the diff:
//
//   - repo in upstream but not in cache → enqueue refresh (was missed —
//     probably a lost push webhook, or the cache is brand-new)
//   - repo in cache but not in upstream → delete row (was missed —
//     probably a lost delete webhook, or the repo was GC'd)
//   - no discrepancy                    → no-op, no audit noise
//
// After the membership diff, it also enqueues a refresh for any cached
// row whose refreshed_at is older than staleRefreshAge. These don't get
// individual audit entries (that would flood the log on every cycle);
// the count is rolled into the info-level summary at the bottom.
//
// Discrepancies are audited so operators notice repeated drift
// (indicating webhooks are broken) instead of silently self-healing.
func (r *Reconciler) ReconcileOnce(ctx context.Context) {
	upstream, err := r.fetchCatalog(ctx)
	if err != nil {
		r.logger.Warnf("catalog fetch: %v", err)
		return
	}
	cached, err := r.meta.AllRepos(ctx)
	if err != nil {
		r.logger.Warnf("cached list: %v", err)
		return
	}

	upstreamSet := toSet(upstream)
	cachedSet := toSet(cached)

	var added, removed int
	for _, repo := range upstream {
		if _, ok := cachedSet[repo]; ok {
			continue
		}
		added++
		r.meta.EnqueueRefresh(repo)
		r.audit.Write(ctx, AuditEntry{
			Actor:  "reconciler",
			Action: ActionReconcileAdded,
			Target: "repository:" + repo,
			Detail: map[string]any{"reason": "present upstream, absent in cache"},
		})
	}
	for _, repo := range cached {
		if _, ok := upstreamSet[repo]; ok {
			continue
		}
		removed++
		if err := r.meta.DeleteRepo(ctx, repo); err != nil {
			r.logger.Warnf("delete %s: %v", repo, err)
			continue
		}
		r.audit.Write(ctx, AuditEntry{
			Actor:  "reconciler",
			Action: ActionReconcileRemoved,
			Target: "repository:" + repo,
			Detail: map[string]any{"reason": "absent upstream, present in cache"},
		})
	}

	stale, err := r.meta.ListStale(ctx, time.Now().Add(-staleRefreshAge))
	if err != nil {
		// Don't bail — the add/remove work above already happened and
		// stale-refresh is a self-healing optimization, not a hard
		// requirement. Just log and let the next cycle retry.
		r.logger.Warnf("list stale: %v", err)
	}
	for _, repo := range stale {
		r.meta.EnqueueRefresh(repo)
	}

	if added > 0 || removed > 0 || len(stale) > 0 {
		r.logger.Infof("reconcile: +%d / -%d / stale=%d (upstream=%d, cache=%d)",
			added, removed, len(stale), len(upstream), len(cached))
	}
}

// fetchCatalog drains every page of /v2/_catalog via registryfetch's
// keyset-cursor pagination.
func (r *Reconciler) fetchCatalog(ctx context.Context) ([]string, error) {
	const pageSize = 1000
	var all []string
	cursor := ""
	for {
		page, next, err := r.registry.Catalog(ctx, cursor, pageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if next == "" {
			return all, nil
		}
		cursor = next
	}
}

func toSet(xs []string) map[string]struct{} {
	s := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		s[x] = struct{}{}
	}
	return s
}
