package registryfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// get mints a pull-scoped JWT for the repo and issues the request.
// Non-200 status codes surface as an error with a truncated body for
// log readability.
func (c *Client) get(ctx context.Context, repo, path, accept string) ([]byte, error) {
	token, err := c.tokens.IssueRegistryToken(c.subject, []Access{
		{Type: "repository", Name: repo, Actions: []string{"pull"}},
	})
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %s → %d: %s", path, resp.StatusCode, truncate(body))
	}
	return body, nil
}

func truncate(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}
