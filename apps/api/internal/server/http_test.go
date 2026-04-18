package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"api/internal/conf"
	"api/internal/data"
	"api/internal/server"
	"api/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/durationpb"
)

// harness wraps the Dockery HTTP handler chain (kratoscarf + ent +
// SQLite) inside a standard httptest.Server. This sidesteps Kratos's
// Start/Stop lifecycle (Shutdown waits indefinitely for idle keep-alive
// connections, which deadlocks tests that share an http.Client pool).
// httptest.Close forcibly tears down every connection, so teardown is
// deterministic. Each harness uses its own temp SQLite file.
type harness struct {
	t       *testing.T
	baseURL string
	client  *http.Client
	stop    func()
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	logger := log.NewStdLogger(io.Discard)

	tmpDB := filepath.Join(t.TempDir(), "dockery-test.db")
	confData := &conf.Data{
		Database: &conf.Data_Database{
			Driver: "sqlite",
			Source: fmt.Sprintf("file:%s?cache=shared&_pragma=foreign_keys(1)", tmpDB),
		},
	}
	d, cleanup, err := data.NewData(confData, logger)
	if err != nil {
		t.Fatalf("data.NewData: %v", err)
	}

	svcs := &service.Services{
		System:     service.NewSystemService(),
		Auth:       service.NewAuthService(d),
		User:       service.NewUserService(d),
		Permission: service.NewPermissionService(d),
		Registry:   service.NewRegistryService(d),
		Token:      service.NewTokenService(d),
		Admin:      service.NewAdminService(d),
	}

	// We still build a kratos http.Server for its option chain
	// (ErrorEncoder, filters, middleware, router wiring), but we never
	// call Start — we mount its Handler inside httptest instead.
	confSrv := &conf.Server{
		Http: &conf.Server_HTTP{
			Network: "tcp",
			Addr:    "127.0.0.1:0",
			Timeout: durationpb.New(5 * time.Second),
		},
	}
	kratosSrv := server.NewHTTPServer(confSrv, svcs, logger)

	testSrv := httptest.NewServer(kratosSrv.Handler)
	client := testSrv.Client()
	client.Timeout = 5 * time.Second

	return &harness{
		t:       t,
		baseURL: testSrv.URL,
		client:  client,
		stop: func() {
			testSrv.Close()
			cleanup()
		},
	}
}

// --- request helpers ---------------------------------------------------

type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (h *harness) do(method, path string, body any) (*http.Response, []byte) {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, h.baseURL+path, rdr)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.client.Do(req)
	if err != nil {
		h.t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("read body: %v", err)
	}
	return resp, raw
}

func (h *harness) decode(raw []byte) envelope {
	h.t.Helper()
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		h.t.Fatalf("decode envelope: %v; body=%s", err, string(raw))
	}
	return env
}

// --- tests -------------------------------------------------------------

func TestHealthz_WrapsPayload(t *testing.T) {
	h := newHarness(t)
	defer h.stop()

	resp, raw := h.do(http.MethodGet, "/healthz", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d; body=%s", resp.StatusCode, raw)
	}
	env := h.decode(raw)
	if env.Code != 0 || env.Message != "ok" {
		t.Fatalf("expected success envelope, got %+v", env)
	}
	var data struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(env.Data, &data)
	if data.Status != "ok" {
		t.Fatalf("expected status=ok, got %q", data.Status)
	}
}

func TestReadyz(t *testing.T) {
	h := newHarness(t)
	defer h.stop()

	resp, _ := h.do(http.MethodGet, "/readyz", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestPing_ValidationRejectsMissingName(t *testing.T) {
	h := newHarness(t)
	defer h.stop()

	resp, raw := h.do(http.MethodGet, "/ping", nil)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d; body=%s", resp.StatusCode, raw)
	}
	env := h.decode(raw)
	if env.Code == 0 {
		t.Fatalf("want non-zero business code on validation failure, got %+v", env)
	}
}

func TestPing_BindsAndWraps(t *testing.T) {
	h := newHarness(t)
	defer h.stop()

	resp, raw := h.do(http.MethodGet, "/ping?name=hello", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d; body=%s", resp.StatusCode, raw)
	}
	env := h.decode(raw)
	if env.Code != 0 {
		t.Fatalf("want code 0, got %+v", env)
	}
	var data struct {
		Pong string `json:"pong"`
	}
	_ = json.Unmarshal(env.Data, &data)
	if data.Pong != "hello" {
		t.Fatalf("expected pong=hello, got %q", data.Pong)
	}
}

func TestSecurityHeadersApplied(t *testing.T) {
	h := newHarness(t)
	defer h.stop()

	resp, _ := h.do(http.MethodGet, "/healthz", nil)
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	h := newHarness(t)
	defer h.stop()

	req, _ := http.NewRequest(http.MethodOptions, h.baseURL+"/healthz", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatalf("do preflight: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got == "" {
		t.Errorf("Access-Control-Allow-Origin header missing")
	}
}

// loginSmoke verifies:
//   - route is reachable (no 404)
//   - validation runs (empty body → 422)
//   - with a valid body, handler stub returns 501 (M2 will replace)
func TestAuthLogin_ValidationAndStub(t *testing.T) {
	h := newHarness(t)
	defer h.stop()

	// empty body → 422
	resp, _ := h.do(http.MethodPost, "/api/auth/login", map[string]string{})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("empty body want 422, got %d", resp.StatusCode)
	}

	// valid body → 501 (stub)
	resp, raw := h.do(http.MethodPost, "/api/auth/login", map[string]string{
		"username": "alice",
		"password": "password1234",
	})
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("valid body want 501, got %d; body=%s", resp.StatusCode, raw)
	}
	env := h.decode(raw)
	if env.Code != 50100 {
		t.Fatalf("want business code 50100, got %+v", env)
	}
}

func TestTokenEndpoint_Stub(t *testing.T) {
	h := newHarness(t)
	defer h.stop()

	resp, _ := h.do(http.MethodGet, "/token", nil)
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d", resp.StatusCode)
	}
}

// routeMatrix confirms every public endpoint is reachable AND that
// middleware stubs (RequireSession / RequireAdmin) currently pass
// through, so downstream stubs can return 501. When M3 replaces the
// middleware with real auth, the expected code for session routes
// flips to 401; update this table then.
func TestRouteMatrix(t *testing.T) {
	h := newHarness(t)
	defer h.stop()

	cases := []struct {
		method string
		path   string
		body   any
		want   int
	}{
		// Public
		{http.MethodGet, "/healthz", nil, 200},
		{http.MethodGet, "/readyz", nil, 200},
		{http.MethodGet, "/token", nil, 501},
		// Session stubs pass through in M1
		{http.MethodPost, "/api/auth/logout", nil, 501},
		{http.MethodGet, "/api/auth/me", nil, 501},
		{http.MethodGet, "/api/registry/catalog", nil, 501},
		// NOTE: repo names here are single-segment; see M2 TODO in routes.go.
		{http.MethodGet, "/api/registry/aliceapp/tags", nil, 501},
		{http.MethodGet, "/api/registry/aliceapp/manifests/latest", nil, 501},
		{http.MethodDelete, "/api/registry/aliceapp/manifests/latest", nil, 501},
		{http.MethodGet, "/api/registry/aliceapp/blobs/sha256:abc", nil, 501},
		// Password change needs a valid body to pass validation
		{http.MethodPut, "/api/users/1/password", map[string]string{
			"new_password": "a-strong-password-42",
		}, 501},
		// Admin stubs pass through in M1
		{http.MethodGet, "/api/users", nil, 501},
		{http.MethodGet, "/api/users/1", nil, 501},
		{http.MethodDelete, "/api/users/1", nil, 501},
		{http.MethodGet, "/api/users/1/permissions", nil, 501},
		{http.MethodPost, "/api/admin/gc", nil, 501},
		{http.MethodPost, "/api/admin/rotate-signing-key", nil, 501},
		{http.MethodGet, "/api/audit", nil, 501},
	}

	for _, tc := range cases {
		tc := tc
		name := fmt.Sprintf("%s_%s", tc.method, strings.ReplaceAll(tc.path, "/", "_"))
		t.Run(name, func(t *testing.T) {
			resp, raw := h.do(tc.method, tc.path, tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("%s %s: want %d got %d; body=%s",
					tc.method, tc.path, tc.want, resp.StatusCode, raw)
			}
		})
	}
}

// Unknown routes should 404, not 500 or envelope noise.
func TestUnknownRoute_404(t *testing.T) {
	h := newHarness(t)
	defer h.stop()

	resp, _ := h.do(http.MethodGet, "/this-route-does-not-exist", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

// Admin POST endpoints that take bodies with required fields should
// reject empty bodies at the validator layer, proving kratoscarf's
// WithValidator is actually attached to admin-group routes too.
func TestAdminCreateUser_ValidatorWiredOnGroupedRoutes(t *testing.T) {
	h := newHarness(t)
	defer h.stop()

	resp, raw := h.do(http.MethodPost, "/api/users", map[string]any{})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 on empty body, got %d; body=%s", resp.StatusCode, raw)
	}

	resp, raw = h.do(http.MethodPost, "/api/users", map[string]string{
		"username": "alice",
		"password": "a-strong-password-42",
		"role":     "invalid-role",
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid role want 422, got %d; body=%s", resp.StatusCode, raw)
	}
}
