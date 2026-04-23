package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// Reconciler bridges any drift between distribution's /v2/_catalog and
// our repo_meta cache. Runs once on startup + every 30 minutes
// thereafter. For every discrepancy (upstream has a repo we don't, or
// vice versa) it enqueues a refresh / issues a delete and writes a row
// to the audit log — making event loss visible to operators rather
// than a silent cache slowly going out of date.
type Reconciler struct {
	meta        *RepoMetaUsecase
	tokens      *TokenIssuer
	audit       *AuditUsecase
	upstreamURL string
	client      *http.Client
	logger      *log.Helper
	interval    time.Duration

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
	tokens *TokenIssuer,
	audit *AuditUsecase,
	upstream RegistryUpstreamURL,
	logger log.Logger,
) *Reconciler {
	return &Reconciler{
		meta:        meta,
		tokens:      tokens,
		audit:       audit,
		upstreamURL: string(upstream),
		client:      &http.Client{Timeout: 30 * time.Second},
		logger:      log.NewHelper(log.With(logger, "module", "biz/reconciler")),
		interval:    30 * time.Minute,
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

// ReconcileOnce walks /v2/_catalog, compares to repo_meta, and reacts
// to the diff:
//
//   - repo in upstream but not in cache → enqueue refresh (was missed —
//     probably a lost push webhook, or the cache is brand-new)
//   - repo in cache but not in upstream → delete row (was missed —
//     probably a lost delete webhook, or the repo was GC'd)
//   - no discrepancy                    → no-op, no audit noise
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
	if added > 0 || removed > 0 {
		r.logger.Infof("reconcile: +%d / -%d (upstream=%d, cache=%d)",
			added, removed, len(upstream), len(cached))
	}
}

// fetchCatalog walks /v2/_catalog with keyset pagination. The upstream
// Link header carries `<next-url>; rel="next"` when more pages exist;
// we follow until it disappears.
func (r *Reconciler) fetchCatalog(ctx context.Context) ([]string, error) {
	const pageSize = 1000
	path := fmt.Sprintf("/v2/_catalog?n=%d", pageSize)
	var all []string
	for path != "" {
		body, linkHeader, err := r.getUpstream(ctx, path)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Repositories []string `json:"repositories"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("parse catalog: %w", err)
		}
		all = append(all, resp.Repositories...)
		path = nextPagePath(linkHeader)
	}
	return all, nil
}

// getUpstream mints a registry:catalog:* JWT and fetches the given path
// (may be relative). Returns body + Link header for pagination follow.
func (r *Reconciler) getUpstream(ctx context.Context, path string) ([]byte, string, error) {
	token, err := r.tokens.IssueRegistryToken("dockery-reconciler",
		[]RegistryAccess{{Type: "registry", Name: "catalog", Actions: []string{"*"}}})
	if err != nil {
		return nil, "", fmt.Errorf("sign: %w", err)
	}
	fullURL := r.upstreamURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("upstream %s → %d", fullURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return body, resp.Header.Get("Link"), nil
}

// linkNextRe matches the conventional distribution Link header:
// `<http://…?n=…&last=…>; rel="next"`.
var linkNextRe = regexp.MustCompile(`<([^>]+)>\s*;\s*rel="next"`)

// nextPagePath extracts the URL path from a registry Link header. Only
// the path is returned — host/scheme are stripped so the follow-up
// request stays pinned to r.upstreamURL regardless of whatever
// distribution advertises (which can be wrong in proxied setups).
func nextPagePath(link string) string {
	m := linkNextRe.FindStringSubmatch(link)
	if len(m) < 2 {
		return ""
	}
	if u, err := url.Parse(m[1]); err == nil && (u.Path != "" || u.RawQuery != "") {
		path := u.Path
		if u.RawQuery != "" {
			path += "?" + u.RawQuery
		}
		return path
	}
	return m[1]
}

func toSet(xs []string) map[string]struct{} {
	s := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		s[x] = struct{}{}
	}
	return s
}
