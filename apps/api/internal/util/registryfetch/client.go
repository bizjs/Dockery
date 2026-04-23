package registryfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

// Client is a thin authenticated HTTP client for the upstream registry.
// Every call mints a short-lived JWT scoped to the exact repository it
// touches so revocation is cheap (TTL expiry) and the registry's audit
// log shows the right caller.
type Client struct {
	tokens  TokenIssuer
	baseURL string
	http    *http.Client
	// subject is baked into every minted token ("sub" claim) for
	// upstream log correlation — webhook-driven refresh and UI-driven
	// enrichment get distinct subjects so `registry` logs tell them apart.
	subject string
}

// Option is the variadic knob for New.
type Option func(*Client)

// WithHTTPClient swaps the default *http.Client — useful in tests.
func WithHTTPClient(c *http.Client) Option { return func(x *Client) { x.http = c } }

// WithSubject sets the JWT subject used for outbound tokens.
func WithSubject(s string) Option { return func(x *Client) { x.subject = s } }

// New constructs a ready-to-use Client.
func New(tokens TokenIssuer, baseURL string, opts ...Option) *Client {
	c := &Client{
		tokens:  tokens,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
		subject: "dockery",
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Tags fetches /v2/<name>/tags/list. Distribution returns
// `{"tags": null}` once the last tag of a repo is removed; this
// flattens to an empty slice so every caller can rely on a non-nil
// return.
func (c *Client) Tags(ctx context.Context, repo string) ([]string, error) {
	body, err := c.get(ctx, repo, fmt.Sprintf("/v2/%s/tags/list", repo), "")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse tags: %w", err)
	}
	if resp.Tags == nil {
		return []string{}, nil
	}
	return resp.Tags, nil
}

// Manifest fetches /v2/<name>/manifests/<ref> with the full Accept
// header so registry hands back the richest format (manifest list or
// v2) rather than downconverting.
func (c *Client) Manifest(ctx context.Context, repo, ref string) (*Manifest, error) {
	body, err := c.get(ctx, repo, fmt.Sprintf("/v2/%s/manifests/%s", repo, ref), ManifestAcceptHeader)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// ConfigBlob fetches /v2/<name>/blobs/<digest> for an image config
// JSON. Layer blob fetches go through the service.RegistryService
// proxy directly — we only read config blobs here, never layers.
func (c *Client) ConfigBlob(ctx context.Context, repo, digest string) (*ConfigBlob, error) {
	body, err := c.get(ctx, repo, fmt.Sprintf("/v2/%s/blobs/%s", repo, digest), "")
	if err != nil {
		return nil, err
	}
	var out ConfigBlob
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse config blob: %w", err)
	}
	return &out, nil
}

// ChildMeta pulls one manifest-list child + its config blob and
// returns {size, created}. Best-effort: any upstream failure returns
// a zero-value ChildMeta so callers can keep walking other children.
func (c *Client) ChildMeta(ctx context.Context, repo, digest string) ChildMeta {
	m, err := c.Manifest(ctx, repo, digest)
	if err != nil {
		return ChildMeta{}
	}
	var out ChildMeta
	if m.Config != nil {
		out.Size += m.Config.Size
	}
	for _, l := range m.Layers {
		out.Size += l.Size
	}
	if m.Config != nil && m.Config.Digest != "" {
		if cfg, err := c.ConfigBlob(ctx, repo, m.Config.Digest); err == nil {
			out.Created = cfg.Created
		}
	}
	return out
}

// Catalog fetches one page of /v2/_catalog using the keyset scheme
// distribution exposes. `cursor` is the last repo seen (empty for the
// first page); `pageSize` caps items per response. Returns the repos
// and the next-page cursor — empty cursor means we're at the end.
//
// Scoped registry:catalog:* so it's admin-only upstream; dockery-api
// is the trust anchor that gates exposure of the result to end users.
func (c *Client) Catalog(ctx context.Context, cursor string, pageSize int) (repos []string, next string, err error) {
	path := fmt.Sprintf("/v2/_catalog?n=%d", pageSize)
	if cursor != "" {
		path += "&last=" + url.QueryEscape(cursor)
	}
	body, hdr, err := c.do(ctx, path, "", []Access{
		{Type: "registry", Name: "catalog", Actions: []string{"*"}},
	})
	if err != nil {
		return nil, "", err
	}
	var resp struct {
		Repositories []string `json:"repositories"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, "", fmt.Errorf("parse catalog: %w", err)
	}
	return resp.Repositories, nextCursor(hdr.Get("Link")), nil
}

// get is the repository-scoped shortcut most methods in this file
// use — most of them only need pull on a single repo.
func (c *Client) get(ctx context.Context, repo, path, accept string) ([]byte, error) {
	body, _, err := c.do(ctx, path, accept, []Access{
		{Type: "repository", Name: repo, Actions: []string{"pull"}},
	})
	return body, err
}

// do mints a JWT with the given access claim and issues the request.
// Returns body + response headers so callers that need Link / digest
// headers can read them. Non-200 status codes surface as an error
// with a truncated body for log readability.
func (c *Client) do(ctx context.Context, path, accept string, access []Access) ([]byte, http.Header, error) {
	token, err := c.tokens.IssueRegistryToken(c.subject, access)
	if err != nil {
		return nil, nil, fmt.Errorf("sign: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("upstream %s → %d: %s", path, resp.StatusCode, truncate(body))
	}
	return body, resp.Header, nil
}

// linkNextRe pulls the URL out of a standard `Link: <url>; rel="next"`
// header. distribution sometimes proxies through an advertised
// hostname that isn't reachable from our side, so we only parse the
// URL for the `last` query param and rebuild the next path ourselves.
var linkNextRe = regexp.MustCompile(`<([^>]+)>\s*;\s*rel="next"`)

// nextCursor extracts the `last` query param from a Link: rel="next"
// URL — that's the opaque cursor we hand back on the next Catalog call.
// Returns "" on any parse failure so pagination terminates cleanly
// instead of recursing with garbage.
func nextCursor(link string) string {
	m := linkNextRe.FindStringSubmatch(link)
	if len(m) < 2 {
		return ""
	}
	u, err := url.Parse(m[1])
	if err != nil {
		return ""
	}
	return u.Query().Get("last")
}

func truncate(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}
