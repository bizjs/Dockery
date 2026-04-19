package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"api/internal/biz"
	"api/internal/pkg/scope"

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
	upstream    string
	client      *http.Client
}

// NewRegistryService wires the proxy. Upstream is pinned to the
// loopback address the supervisord starts registry on; when the
// container moves to multi-host, this becomes configurable.
func NewRegistryService(users *biz.UserUsecase, perms *biz.PermissionUsecase, tokens *biz.TokenIssuer, audit *biz.AuditUsecase, maintenance *biz.Maintenance) *RegistryService {
	return &RegistryService{
		users:       users,
		perms:       perms,
		tokens:      tokens,
		audit:       audit,
		maintenance: maintenance,
		upstream:    "http://127.0.0.1:5001",
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
	var manifest any
	if err := json.Unmarshal(body, &manifest); err != nil {
		return response.ErrInternal.WithCause(err)
	}
	return ctx.Success(manifest)
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
// Admin callers should never reach this path.
func filterByPatterns(repos, patterns []string) []string {
	if len(patterns) == 0 {
		return []string{}
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
