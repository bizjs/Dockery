package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"api/internal/biz"
	"api/internal/util/registryfetch"
	"api/internal/util/scope"

	"github.com/bizjs/kratoscarf/response"
	"github.com/bizjs/kratoscarf/router"
)

// RegistryService is the UI-facing proxy that lets the Web UI consume
// the Distribution Registry API without having to deal with Docker
// token auth in the browser.
//
//   1. kratoscarf session middleware has already authenticated the user.
//   2. RegistryService checks the user's Dockery-level permissions
//      (biz.PermissionUsecase) for each requested operation.
//   3. For every upstream call, we mint a short-lived (30s) Ed25519 JWT
//      with admin-level scope and forward the request to the local
//      distribution daemon on 127.0.0.1:5001; this is safe because
//      dockery-api is the trust anchor and we've already enforced the
//      user's actual role/permission at step 2.
//   4. Responses (or list contents) are passed through to the UI after
//      optional filtering (catalog is filtered for non-admins).
type RegistryService struct {
	users       *biz.UserUsecase
	perms       *biz.PermissionUsecase
	tokens      *biz.TokenIssuer
	audit       *biz.AuditUsecase
	maintenance *biz.Maintenance
	meta        *biz.RepoMetaUsecase
	// fetcher is used only by the UI-proxy enrichment path (manifest
	// list size/created aggregation). The UI proxy's transparent
	// passthrough (forward method) mints its own per-request token
	// because it preserves upstream headers — that's a different shape
	// from what registryfetch.Client offers.
	fetcher  *registryfetch.Client
	upstream string
	client   *http.Client
}

// NewRegistryService wires the proxy. Upstream comes from conf so dev
// setups with different ports share the same code path as the bundled
// container (where supervisord starts registry on 127.0.0.1:5001).
func NewRegistryService(
	users *biz.UserUsecase,
	perms *biz.PermissionUsecase,
	tokens *biz.TokenIssuer,
	audit *biz.AuditUsecase,
	maintenance *biz.Maintenance,
	meta *biz.RepoMetaUsecase,
	fetcher *registryfetch.Client,
	upstream biz.RegistryUpstreamURL,
) *RegistryService {
	return &RegistryService{
		users:       users,
		perms:       perms,
		tokens:      tokens,
		audit:       audit,
		maintenance: maintenance,
		meta:        meta,
		fetcher:     fetcher,
		upstream:    string(upstream),
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

// --- DTOs exposed to UI ------------------------------------------------

type CatalogView struct {
	Repositories []string `json:"repositories"`
}

type TagsView struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// --- Handlers ---------------------------------------------------------

// Catalog proxies /v2/_catalog. Admins see every repository; non-admin
// users see only repos their permission patterns match.
func (s *RegistryService) Catalog(ctx *router.Context) error {
	user, err := s.currentUser(ctx)
	if err != nil {
		return err
	}

	status, body, _, err := s.forward(ctx, http.MethodGet, "/v2/_catalog",
		biz.RegistryAccess{Type: "registry", Name: "catalog", Actions: []string{"*"}})
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	if status != http.StatusOK {
		return response.NewBizError(status, 50000, fmt.Sprintf("upstream: %s", trimBody(body)))
	}

	var out CatalogView
	if err := json.Unmarshal(body, &out); err != nil {
		return response.ErrInternal.WithCause(err)
	}

	// Non-admin: filter catalog to repos this user actually has any
	// (pull / push / delete) grant on.
	if user.Role != "admin" {
		patterns, err := s.listPatterns(ctx, user.ID)
		if err != nil {
			return response.ErrInternal.WithCause(err)
		}
		out.Repositories = filterByPatterns(out.Repositories, patterns)
	}

	// Normalize nil → empty slice so clients get `[]` instead of `null`
	// after the last repo's blobs were swept by GC. Downstream callers
	// (including our own UI) frequently destructure with defaults that
	// only cover undefined — not null — so this keeps the contract
	// stable for any consumer.
	if out.Repositories == nil {
		out.Repositories = []string{}
	}

	return ctx.Success(out)
}

// --- Overview: cache-backed aggregated view for the Catalog UI ---

// OverviewPlatform mirrors schema.PlatformInfo but stays in the service
// layer so the frontend doesn't reach across packages to type its
// response. `omitempty` keeps responses compact when a field is empty.
type OverviewPlatform struct {
	Os           string `json:"os,omitempty"`
	Architecture string `json:"architecture,omitempty"`
	Variant      string `json:"variant,omitempty"`
}

// OverviewItem is one row in the Catalog table.
type OverviewItem struct {
	Repo         string             `json:"repo"`
	LatestTag    string             `json:"latest_tag,omitempty"`
	TagCount     int                `json:"tag_count"`
	Size         int64              `json:"size"`
	Created      string             `json:"created,omitempty"`
	Platforms    []OverviewPlatform `json:"platforms,omitempty"`
	PullCount    int64              `json:"pull_count"`
	LastPulledAt *int64             `json:"last_pulled_at,omitempty"`
	RefreshedAt  int64              `json:"refreshed_at"`
}

// OverviewResponse is paginated.
type OverviewResponse struct {
	Items    []OverviewItem `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// Overview returns the cached RepoMeta rows that the session user is
// allowed to see, filtered + sorted + paginated server-side. The
// catalog page used to fan out one manifest fetch per card — now it
// makes one call here and the backend reads from repo_meta (kept in
// sync by webhooks + reconciler). No upstream registry traffic on
// this path.
//
// Query params:
//   - q          — substring match on repo name (case-insensitive)
//   - sort       — name | updated | size | tags  (default name)
//   - direction  — asc | desc  (default depends on sort column)
//   - page       — 0-based (default 0)
//   - page_size  — default 50, clamped to [1, 500]
func (s *RegistryService) Overview(ctx *router.Context) error {
	user, err := s.currentUser(ctx)
	if err != nil {
		return err
	}

	q := strings.TrimSpace(ctx.Query("q"))
	sortName := ctx.Query("sort")
	direction := ctx.Query("direction")
	page := queryIntDefault(ctx, "page", 0)
	pageSize := queryIntDefault(ctx, "page_size", 50)

	// Resolve per-user pattern list. Admin bypasses the table; an
	// empty pattern list for non-admin users also means "unrestricted"
	// (see scope.Match Rule 3 / design §7.3).
	var patterns []string
	if user.Role != "admin" {
		patterns, err = s.listPatterns(ctx, user.ID)
		if err != nil {
			return response.ErrInternal.WithCause(err)
		}
	}

	// Fast path: no pattern restriction (admin or unrestricted
	// non-admin) → filter/sort/paginate entirely in SQL. Scales to
	// large registries without loading every row into process memory.
	if user.Role == "admin" || len(patterns) == 0 {
		bizPage, err := s.meta.QueryPage(ctx.Context(), biz.OverviewFilter{
			Query:     q,
			Sort:      parseSort(sortName),
			Direction: parseDirection(direction, sortName),
			Page:      clampNonNeg(page),
			PageSize:  pageSize,
		})
		if err != nil {
			return response.ErrInternal.WithCause(err)
		}
		return ctx.Success(toOverviewResponse(bizPage, page, pageSize))
	}

	// Pattern-restricted path: segment-aware globs (e.g. `alice/*`
	// matches `alice/X` but not `alice/X/Y`) don't translate cleanly
	// to SQL, so we keep the Go-side filter here. Same upper bound as
	// SQL path (500 page size); practical pattern users have tiny
	// match sets so the full-list pull is cheap.
	items, err := s.meta.List(ctx.Context())
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	items = filterMetaByPatterns(items, patterns)
	if q != "" {
		lower := strings.ToLower(q)
		filtered := make([]*biz.RepoMeta, 0, len(items))
		for _, m := range items {
			if strings.Contains(strings.ToLower(m.Repo), lower) {
				filtered = append(filtered, m)
			}
		}
		items = filtered
	}
	if sortName == "" {
		sortName = "name"
	}
	if direction == "" {
		if sortName == "name" {
			direction = "asc"
		} else {
			direction = "desc"
		}
	}
	sortMeta(items, sortName, direction)
	total := len(items)

	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	if page < 0 {
		page = 0
	}
	start := page * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	out := OverviewResponse{
		Items:    make([]OverviewItem, 0, end-start),
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}
	for _, m := range items[start:end] {
		out.Items = append(out.Items, toOverviewItem(m))
	}
	return ctx.Success(out)
}

// parseSort maps the `sort` query param to the biz enum. Unknown values
// fall back to name so a typo can't make the handler error out.
func parseSort(s string) biz.OverviewSort {
	switch s {
	case "size":
		return biz.OverviewSortSize
	case "updated":
		return biz.OverviewSortUpdated
	case "tags":
		return biz.OverviewSortTagCount
	default:
		return biz.OverviewSortName
	}
}

// parseDirection defaults based on the sort column — asc for name
// (A→Z), desc for everything else (biggest / newest / most first is
// what people usually want on first click).
func parseDirection(dir, sortName string) biz.OverviewDir {
	if dir == "asc" {
		return biz.OverviewAsc
	}
	if dir == "desc" {
		return biz.OverviewDesc
	}
	if sortName == "" || sortName == "name" {
		return biz.OverviewAsc
	}
	return biz.OverviewDesc
}

func clampNonNeg(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func toOverviewResponse(p *biz.OverviewPage, page, pageSize int) OverviewResponse {
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	if page < 0 {
		page = 0
	}
	out := OverviewResponse{
		Items:    make([]OverviewItem, 0, len(p.Items)),
		Total:    p.Total,
		Page:     page,
		PageSize: pageSize,
	}
	for _, m := range p.Items {
		out.Items = append(out.Items, toOverviewItem(m))
	}
	return out
}

func toOverviewItem(m *biz.RepoMeta) OverviewItem {
	platforms := make([]OverviewPlatform, 0, len(m.Platforms))
	for _, p := range m.Platforms {
		platforms = append(platforms, OverviewPlatform{
			Os:           p.Os,
			Architecture: p.Architecture,
			Variant:      p.Variant,
		})
	}
	return OverviewItem{
		Repo:         m.Repo,
		LatestTag:    m.LatestTag,
		TagCount:     m.TagCount,
		Size:         m.Size,
		Created:      m.Created,
		Platforms:    platforms,
		PullCount:    m.PullCount,
		LastPulledAt: m.LastPulledAt,
		RefreshedAt:  m.RefreshedAt,
	}
}

// filterMetaByPatterns mirrors filterByPatterns but on *biz.RepoMeta
// slices. Empty patterns = unrestricted (matches the rule in
// scope.Match for zero-pattern non-admin users). Allocates a new slice
// to avoid aliasing with the caller's view of `items`.
func filterMetaByPatterns(items []*biz.RepoMeta, patterns []string) []*biz.RepoMeta {
	if len(patterns) == 0 {
		return items
	}
	out := make([]*biz.RepoMeta, 0, len(items))
	for _, m := range items {
		for _, p := range patterns {
			if scope.MatchPattern(p, m.Repo) {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

// sortMeta orders items in place. Mirrors the SQL ORDER BY in
// data/repo_meta.go QueryPage so admins/unrestricted users (SQL path)
// and pattern-restricted users (this path) see the same ordering for
// the same query — repo ASC as a stable tiebreak regardless of the
// primary direction. (Previously the helper flipped the entire
// comparator for desc, which inverted the tiebreak too — page
// boundaries shifted between the two paths and the UI's "Showing
// X–Y of N" could disagree.)
func sortMeta(items []*biz.RepoMeta, field, direction string) {
	desc := direction == "desc"
	primary := primaryComparator(field)
	sort.SliceStable(items, func(i, j int) bool {
		if c := primary(items[i], items[j]); c != 0 {
			if desc {
				return c > 0
			}
			return c < 0
		}
		// Tiebreak: repo ASC, always — matches the SQL secondary order.
		return items[i].Repo < items[j].Repo
	})
}

// primaryComparator returns a -1/0/+1 comparator for the requested
// sort column. Unknown column names — and the explicit "name" column —
// compare on Repo, which mirrors data/repo_meta.go's sortFieldFor (it
// also defaults to FieldRepo). Without this, sort=name&direction=desc
// would silently return ASC because every primary comparison would be
// zero and only the repo-ASC tiebreak would fire.
func primaryComparator(field string) func(a, b *biz.RepoMeta) int {
	switch field {
	case "updated":
		// Created is ISO-8601; lexicographic compare is date-order.
		return func(a, b *biz.RepoMeta) int { return strings.Compare(a.Created, b.Created) }
	case "size":
		return func(a, b *biz.RepoMeta) int {
			switch {
			case a.Size < b.Size:
				return -1
			case a.Size > b.Size:
				return 1
			}
			return 0
		}
	case "tags":
		return func(a, b *biz.RepoMeta) int {
			switch {
			case a.TagCount < b.TagCount:
				return -1
			case a.TagCount > b.TagCount:
				return 1
			}
			return 0
		}
	default:
		// Includes "" and "name". Repo names are unique so the
		// repo-ASC tiebreak in sortMeta is harmless dead code on this
		// branch (good — it stays consistent with the other branches).
		return func(a, b *biz.RepoMeta) int { return strings.Compare(a.Repo, b.Repo) }
	}
}

// queryIntDefault reads an int query param, falling back on parse
// errors. strconv.Atoi rejects trailing non-digits (`42abc` → default)
// which fmt.Sscanf would silently accept.
func queryIntDefault(ctx *router.Context, key string, def int) int {
	raw := ctx.Query(key)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

// Tags proxies /v2/<name>/tags/list. User must have pull on <name>.
func (s *RegistryService) Tags(ctx *router.Context) error {
	name := ctx.Param("name")
	if err := s.authorize(ctx, name, "pull"); err != nil {
		return err
	}
	status, body, _, err := s.forward(ctx, http.MethodGet,
		fmt.Sprintf("/v2/%s/tags/list", name),
		biz.RegistryAccess{Type: "repository", Name: name, Actions: []string{"pull"}})
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	if status != http.StatusOK {
		return response.NewBizError(status, 50000, fmt.Sprintf("upstream: %s", trimBody(body)))
	}
	var out TagsView
	if err := json.Unmarshal(body, &out); err != nil {
		return response.ErrInternal.WithCause(err)
	}
	// Upstream returns {"tags": null} after the last tag is deleted;
	// flatten to `[]` so every client sees the same shape. See Catalog.
	if out.Tags == nil {
		out.Tags = []string{}
	}
	return ctx.Success(out)
}

// GetManifest proxies /v2/<name>/manifests/<ref>. The Docker-Content-Digest
// response header is preserved in the envelope so UI can perform
// downstream operations that need it.
func (s *RegistryService) GetManifest(ctx *router.Context) error {
	name := ctx.Param("name")
	ref := ctx.Param("ref")
	if err := s.authorize(ctx, name, "pull"); err != nil {
		return err
	}
	status, body, hdr, err := s.forward(ctx, http.MethodGet,
		fmt.Sprintf("/v2/%s/manifests/%s", name, ref),
		biz.RegistryAccess{Type: "repository", Name: name, Actions: []string{"pull"}})
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	if status != http.StatusOK {
		return response.NewBizError(status, 50000, fmt.Sprintf("upstream: %s", trimBody(body)))
	}
	// Preserve the digest — UI needs it for subsequent delete calls.
	if digest := hdr.Get("Docker-Content-Digest"); digest != "" {
		ctx.SetHeader("Docker-Content-Digest", digest)
	}
	// Manifest is raw JSON; pass through as a structured map so the
	// envelope is still valid JSON and UI can traverse fields.
	var manifest map[string]any
	if err := json.Unmarshal(body, &manifest); err != nil {
		return response.ErrInternal.WithCause(err)
	}
	// For manifest lists / OCI image indexes, the per-entry `size` is the
	// size of the child manifest JSON (a few KB), not the image size. Fetch
	// each child manifest and inject `imageSize` (config + layers) so the
	// UI can show a meaningful total.
	s.enrichManifestList(ctx, name, manifest)
	return ctx.Success(manifest)
}

// enrichManifestList mutates a manifest-list response in place, adding
// `imageSize` to each entry in `manifests[]` and at the top level, plus
// a top-level `created` (the latest config.created across runnable
// children). No-op if the response isn't a manifest list. Errors
// fetching individual children leave that entry at zero; the aggregate
// still reflects whichever fetches succeeded.
//
// Uses registryfetch.Client (shared with biz/repo_meta) for the per-
// child fetches — the UI proxy's forward() is for transparent
// passthrough (headers + response body), which isn't what we need
// here.
func (s *RegistryService) enrichManifestList(ctx *router.Context, name string, manifest map[string]any) {
	entries, ok := manifest["manifests"].([]any)
	if !ok || len(entries) == 0 {
		return
	}

	metas := make([]registryfetch.ChildMeta, len(entries))
	var wg sync.WaitGroup
	for i, entryAny := range entries {
		entry, ok := entryAny.(map[string]any)
		if !ok {
			continue
		}
		digest, _ := entry["digest"].(string)
		if digest == "" {
			continue
		}
		wg.Add(1)
		go func(idx int, digest string) {
			defer wg.Done()
			metas[idx] = s.fetcher.ChildMeta(ctx.Context(), name, digest)
		}(i, digest)
	}
	wg.Wait()

	var total int64
	var latestCreated string
	for i, m := range metas {
		entry, ok := entries[i].(map[string]any)
		if !ok {
			continue
		}
		entry["imageSize"] = m.Size
		total += m.Size
		// Attestation manifests (platform.os == "unknown") have bogus
		// config.created (generator timestamps, not image builds).
		// Exclude them from the repo's representative timestamp.
		if isAttestationEntry(entry) {
			continue
		}
		// ISO-8601 timestamps compare lexicographically as dates.
		if m.Created != "" && m.Created > latestCreated {
			latestCreated = m.Created
		}
	}
	manifest["imageSize"] = total
	if latestCreated != "" {
		manifest["created"] = latestCreated
	}
}

// isAttestationEntry reports whether a manifest-list entry is a
// BuildKit attestation (SBOM / provenance) rather than a runnable
// image. Attestations carry platform.os == "unknown" by convention.
func isAttestationEntry(entry map[string]any) bool {
	platform, ok := entry["platform"].(map[string]any)
	if !ok {
		return false
	}
	os, _ := platform["os"].(string)
	return os == "unknown"
}

// DeleteManifest supports deletion by either tag or digest. If the
// client supplies a tag, we resolve to digest via HEAD first because
// distribution rejects DELETE-by-tag ("manifests are only deletable
// by digest").
func (s *RegistryService) DeleteManifest(ctx *router.Context) error {
	// Reject cleanly during GC. Without this, the upstream delete would
	// fail with a confusing 502 (registry is stopped).
	if s.maintenance.Active() {
		return response.NewBizError(http.StatusServiceUnavailable, 50301, "registry is in maintenance")
	}
	name := ctx.Param("name")
	ref := ctx.Param("ref")
	if err := s.authorize(ctx, name, "delete"); err != nil {
		return err
	}

	// If ref is a tag (no "sha256:" prefix), resolve to digest.
	digest := ref
	if !isDigest(ref) {
		d, err := s.resolveDigest(ctx, name, ref)
		if err != nil {
			return err
		}
		digest = d
	}

	status, body, _, err := s.forward(ctx, http.MethodDelete,
		fmt.Sprintf("/v2/%s/manifests/%s", name, digest),
		biz.RegistryAccess{Type: "repository", Name: name, Actions: []string{"delete"}})
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	if status != http.StatusAccepted && status != http.StatusOK {
		return response.NewBizError(status, 50000, fmt.Sprintf("upstream: %s", trimBody(body)))
	}
	s.audit.Write(ctx.Context(), biz.AuditEntry{
		Actor:    sessionUsername(ctx),
		Action:   biz.ActionImageDeleted,
		Target:   "repository:" + name,
		ClientIP: ctx.ClientIP(),
		Success:  true,
		Detail:   map[string]any{"ref": ref, "digest": digest},
	})
	return ctx.Success(map[string]string{"digest": digest})
}

// GetBlob proxies /v2/<name>/blobs/<digest>. Used by UI to fetch the
// image config blob (Cmd / Env / Labels / ExposedPorts).
func (s *RegistryService) GetBlob(ctx *router.Context) error {
	name := ctx.Param("name")
	digest := ctx.Param("digest")
	if err := s.authorize(ctx, name, "pull"); err != nil {
		return err
	}
	status, body, _, err := s.forward(ctx, http.MethodGet,
		fmt.Sprintf("/v2/%s/blobs/%s", name, digest),
		biz.RegistryAccess{Type: "repository", Name: name, Actions: []string{"pull"}})
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	if status != http.StatusOK {
		return response.NewBizError(status, 50000, fmt.Sprintf("upstream: %s", trimBody(body)))
	}
	// Blobs are (for config) JSON. Return as structured any so the
	// kratoscarf envelope wraps it correctly.
	var blob any
	if err := json.Unmarshal(body, &blob); err != nil {
		// Non-JSON blob (rare for UI path) — return as base64 string? Not
		// needed yet; UI only asks for config blobs which are JSON.
		return response.ErrInternal.WithCause(fmt.Errorf("blob is not JSON: %w", err))
	}
	return ctx.Success(blob)
}

// --- internals --------------------------------------------------------

// currentUser returns the full biz.User from the session.
// Loading from DB each call is fine at this scale; avoids caching
// staleness after admin updates the user.
func (s *RegistryService) currentUser(ctx *router.Context) (*biz.User, error) {
	id := sessionUserID(ctx)
	if id == 0 {
		return nil, response.ErrUnauthorized
	}
	u, err := s.users.GetByID(ctx.Context(), id)
	if err != nil {
		return nil, response.ErrUnauthorized
	}
	return u, nil
}

// authorize rejects the request if the session user cannot perform
// `action` on repository `name`. Admins always pass.
func (s *RegistryService) authorize(ctx *router.Context, name, action string) error {
	user, err := s.currentUser(ctx)
	if err != nil {
		return err
	}
	if user.Role == "admin" {
		return nil
	}
	patterns, err := s.listPatterns(ctx, user.ID)
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	granted := scope.Match(scope.Role(user.Role), patterns,
		scope.Scope{Type: "repository", Name: name, Actions: []string{action}})
	if len(granted) == 0 {
		return response.ErrForbidden.WithMessage(fmt.Sprintf("%s on %s not permitted", action, name))
	}
	return nil
}

func (s *RegistryService) listPatterns(ctx *router.Context, userID int) ([]string, error) {
	perms, err := s.perms.ListForUser(ctx.Context(), userID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(perms))
	for _, p := range perms {
		out = append(out, p.RepoPattern)
	}
	return out, nil
}

// forward mints a registry JWT for the given admin-level scope (the
// caller has already checked user-level authorization) and replays
// the request to the upstream registry.
func (s *RegistryService) forward(
	ctx *router.Context,
	method, upstreamPath string,
	access biz.RegistryAccess,
) (int, []byte, http.Header, error) {
	token, err := s.tokens.IssueRegistryToken("dockery-proxy", []biz.RegistryAccess{access})
	if err != nil {
		return 0, nil, nil, fmt.Errorf("sign proxy token: %w", err)
	}

	fullURL := s.upstream + upstreamPath
	if q := ctx.Request().URL.RawQuery; q != "" {
		fullURL += "?" + q
	}

	req, err := http.NewRequestWithContext(ctx.Context(), method, fullURL, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	// Forward OCI + Docker v2 Accept headers so registry returns the
	// manifest format the UI wants (vs. silently converting to v1).
	req.Header.Set("Accept",
		"application/vnd.oci.image.manifest.v1+json,"+
			"application/vnd.docker.distribution.manifest.v2+json,"+
			"application/vnd.docker.distribution.manifest.list.v2+json,"+
			"application/vnd.oci.image.index.v1+json")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, body, resp.Header, err
}

// resolveDigest issues a HEAD against /v2/<name>/manifests/<tag> to
// discover the Docker-Content-Digest, so DeleteManifest can then do
// a DELETE by digest (the only form registry accepts).
func (s *RegistryService) resolveDigest(ctx *router.Context, name, tag string) (string, error) {
	token, err := s.tokens.IssueRegistryToken("dockery-proxy",
		[]biz.RegistryAccess{{Type: "repository", Name: name, Actions: []string{"pull"}}})
	if err != nil {
		return "", response.ErrInternal.WithCause(fmt.Errorf("sign proxy token: %w", err))
	}
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", s.upstream, name, tag)
	req, err := http.NewRequestWithContext(ctx.Context(), http.MethodHead, url, nil)
	if err != nil {
		return "", response.ErrInternal.WithCause(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept",
		"application/vnd.oci.image.manifest.v1+json,"+
			"application/vnd.docker.distribution.manifest.v2+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", response.ErrInternal.WithCause(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", response.NewBizError(resp.StatusCode, 50000, "cannot resolve digest for tag")
	}
	d := resp.Header.Get("Docker-Content-Digest")
	if d == "" {
		return "", response.ErrInternal.WithMessage("upstream returned no Docker-Content-Digest")
	}
	return d, nil
}

// isDigest reports whether ref looks like a content digest (sha256:…)
// as opposed to a human-readable tag.
func isDigest(ref string) bool {
	return len(ref) > 7 && ref[:7] == "sha256:"
}

// filterByPatterns keeps only repo names matched by any of patterns.
// Admin callers should never reach this path. An empty pattern list
// means "no restriction" — the user sees every repo (mirrors
// scope.Match Rule 3). Admin narrows this by adding patterns.
func filterByPatterns(repos, patterns []string) []string {
	if len(patterns) == 0 {
		out := make([]string, len(repos))
		copy(out, repos)
		return out
	}
	out := make([]string, 0, len(repos))
	for _, r := range repos {
		for _, p := range patterns {
			if scope.MatchPattern(p, r) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// trimBody clips an upstream error payload so it doesn't flood our logs.
func trimBody(b []byte) string {
	if len(b) > 300 {
		return string(b[:300]) + "…"
	}
	return string(b)
}
