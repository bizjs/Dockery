package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"api/internal/biz"

	"github.com/bizjs/kratoscarf/response"
	"github.com/bizjs/kratoscarf/router"
	"github.com/go-kratos/kratos/v2/log"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// --- stub refresher -------------------------------------------------

// stubRefresher records every call so assertions can read the exact
// side-effects the webhook produced, without needing a running refresh
// worker or upstream registry.
type stubRefresher struct {
	mu        sync.Mutex
	refreshed []string
	pulled    []string
}

func (s *stubRefresher) EnqueueRefresh(repo string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshed = append(s.refreshed, repo)
}

func (s *stubRefresher) IncrementPull(_ context.Context, repo string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pulled = append(s.pulled, repo)
}

func (s *stubRefresher) snapshot() (refreshed, pulled []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := append([]string(nil), s.refreshed...)
	p := append([]string(nil), s.pulled...)
	sort.Strings(r)
	sort.Strings(p)
	return r, p
}

// --- test harness ---------------------------------------------------

func newWebhookRig(t *testing.T) (baseURL string, secret string, stub *stubRefresher) {
	t.Helper()

	ws, err := biz.NewWebhookSecret(biz.WebhookSecretConfig{
		Path: filepath.Join(t.TempDir(), "webhook-secret"),
	})
	if err != nil {
		t.Fatalf("webhook secret: %v", err)
	}
	stub = &stubRefresher{}
	svc := &WebhookService{secret: ws, meta: stub, seen: newSeenEvents(seenEventCap)}

	// Minimal kratos HTTP server: no session middleware, no CORS, no
	// filters — the webhook route is meant to be reached only through
	// loopback and relies on the Bearer header for auth, nothing else.
	srv := kratoshttp.NewServer(
		kratoshttp.Logger(log.NewStdLogger(io.Discard)),
		kratoshttp.ErrorEncoder(response.NewHTTPErrorEncoder()),
	)
	r := router.NewRouter(srv, router.WithResponseWrapper(response.Wrap))
	r.POST("/api/internal/registry-events", svc.Handle)

	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)

	return ts.URL, ws.Value(), stub
}

func postEvents(t *testing.T, baseURL, bearer string, events any) *http.Response {
	t.Helper()
	body, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost,
		baseURL+"/api/internal/registry-events", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// --- tests ----------------------------------------------------------

func TestWebhook_MissingBearer_Returns401(t *testing.T) {
	baseURL, _, stub := newWebhookRig(t)
	resp := postEvents(t, baseURL, "", map[string]any{"events": []any{}})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	r, p := stub.snapshot()
	if len(r) != 0 || len(p) != 0 {
		t.Errorf("no side effects expected; got refreshed=%v pulled=%v", r, p)
	}
}

func TestWebhook_WrongBearer_Returns401(t *testing.T) {
	baseURL, _, stub := newWebhookRig(t)
	resp := postEvents(t, baseURL, "not-the-real-secret",
		map[string]any{"events": []any{}})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	r, p := stub.snapshot()
	if len(r) != 0 || len(p) != 0 {
		t.Errorf("no side effects expected; got refreshed=%v pulled=%v", r, p)
	}
}

func TestWebhook_BlobMediaType_Ignored(t *testing.T) {
	baseURL, secret, stub := newWebhookRig(t)
	// A layer push: distribution fires events for every blob *and* for
	// the manifest that ties them together. We must act only on the
	// manifest event or we'd trigger N+1 refreshes per push.
	resp := postEvents(t, baseURL, secret, map[string]any{
		"events": []any{
			map[string]any{
				"action": "push",
				"target": map[string]any{
					"mediaType":  "application/vnd.docker.image.rootfs.diff.tar.gzip",
					"repository": "alice/app",
					"digest":     "sha256:aaaa",
				},
			},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	r, p := stub.snapshot()
	if len(r) != 0 {
		t.Errorf("blob push should not refresh; got %v", r)
	}
	if len(p) != 0 {
		t.Errorf("blob push should not pull; got %v", p)
	}
}

func TestWebhook_ManifestPush_DedupsRepo(t *testing.T) {
	baseURL, secret, stub := newWebhookRig(t)
	// Single push of a manifest list fires one event for the list
	// plus one for each child. All carry the same repository, so the
	// handler must enqueue refresh exactly once per repo within a batch.
	resp := postEvents(t, baseURL, secret, map[string]any{
		"events": []any{
			map[string]any{
				"action": "push",
				"target": map[string]any{
					"mediaType":  "application/vnd.oci.image.index.v1+json",
					"repository": "alice/app",
					"tag":        "v1",
				},
			},
			map[string]any{
				"action": "push",
				"target": map[string]any{
					"mediaType":  "application/vnd.oci.image.manifest.v1+json",
					"repository": "alice/app",
				},
			},
			map[string]any{
				"action": "push",
				"target": map[string]any{
					"mediaType":  "application/vnd.docker.distribution.manifest.v2+json",
					"repository": "bob/other",
				},
			},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	refreshed, _ := stub.snapshot()
	want := []string{"alice/app", "bob/other"}
	if len(refreshed) != len(want) {
		t.Fatalf("refreshed = %v, want exactly %v", refreshed, want)
	}
	for i, r := range refreshed {
		if r != want[i] {
			t.Errorf("refreshed[%d] = %q, want %q", i, r, want[i])
		}
	}
}

func TestWebhook_ManifestPull_IncrementsPull(t *testing.T) {
	baseURL, secret, stub := newWebhookRig(t)
	// Pull events fire per blob + once for the manifest. Only the
	// manifest-level one should become an IncrementPull call.
	resp := postEvents(t, baseURL, secret, map[string]any{
		"events": []any{
			map[string]any{
				"action": "pull",
				"target": map[string]any{
					"mediaType":  "application/vnd.docker.image.rootfs.diff.tar.gzip",
					"repository": "alice/app",
				},
			},
			map[string]any{
				"action": "pull",
				"target": map[string]any{
					"mediaType":  "application/vnd.oci.image.manifest.v1+json",
					"repository": "alice/app",
				},
			},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	refreshed, pulled := stub.snapshot()
	if len(refreshed) != 0 {
		t.Errorf("pull must not refresh; got %v", refreshed)
	}
	if len(pulled) != 1 || pulled[0] != "alice/app" {
		t.Errorf("pulled = %v, want [alice/app]", pulled)
	}
}

func TestWebhook_ManifestDelete_Refreshes(t *testing.T) {
	baseURL, secret, stub := newWebhookRig(t)
	// Tag deletion fires a delete event with a manifest mediaType.
	// The refresh path handles the empty-tags case → row deletion.
	resp := postEvents(t, baseURL, secret, map[string]any{
		"events": []any{
			map[string]any{
				"action": "delete",
				"target": map[string]any{
					"mediaType":  "application/vnd.docker.distribution.manifest.v2+json",
					"repository": "alice/app",
					"tag":        "v1",
				},
			},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	refreshed, pulled := stub.snapshot()
	if len(refreshed) != 1 || refreshed[0] != "alice/app" {
		t.Errorf("refreshed = %v, want [alice/app]", refreshed)
	}
	if len(pulled) != 0 {
		t.Errorf("delete must not pull; got %v", pulled)
	}
}

func TestWebhook_EmptyRepo_Skipped(t *testing.T) {
	baseURL, secret, stub := newWebhookRig(t)
	// Malformed event with empty repository must not panic / dispatch.
	resp := postEvents(t, baseURL, secret, map[string]any{
		"events": []any{
			map[string]any{
				"action": "push",
				"target": map[string]any{
					"mediaType":  "application/vnd.oci.image.manifest.v1+json",
					"repository": "",
				},
			},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	refreshed, pulled := stub.snapshot()
	if len(refreshed) != 0 || len(pulled) != 0 {
		t.Errorf("empty repo must be skipped; got refreshed=%v pulled=%v", refreshed, pulled)
	}
}

func TestWebhook_DuplicateEventID_SkippedOnReplay(t *testing.T) {
	// Distribution retries delivery on transport failures (5xx, timeout),
	// so the same event ID can land twice. Push/delete replays would
	// waste upstream refreshes; pull replays would double-count. Both
	// must be suppressed by the seen-events cache.
	baseURL, secret, stub := newWebhookRig(t)

	pushEvent := map[string]any{
		"id":     "evt-push-1",
		"action": "push",
		"target": map[string]any{
			"mediaType":  "application/vnd.oci.image.manifest.v1+json",
			"repository": "alice/app",
		},
	}
	pullEvent := map[string]any{
		"id":     "evt-pull-1",
		"action": "pull",
		"target": map[string]any{
			"mediaType":  "application/vnd.oci.image.manifest.v1+json",
			"repository": "alice/app",
		},
	}

	// First delivery: both fire.
	resp := postEvents(t, baseURL, secret, map[string]any{
		"events": []any{pushEvent, pullEvent},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first delivery status = %d, want 200", resp.StatusCode)
	}

	// Replay of the same envelope: handler must drop both events.
	resp = postEvents(t, baseURL, secret, map[string]any{
		"events": []any{pushEvent, pullEvent},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", resp.StatusCode)
	}

	refreshed, pulled := stub.snapshot()
	if len(refreshed) != 1 || refreshed[0] != "alice/app" {
		t.Errorf("refresh fired %v times after replay; want exactly one alice/app", refreshed)
	}
	if len(pulled) != 1 || pulled[0] != "alice/app" {
		t.Errorf("pull fired %v times after replay; want exactly one alice/app", pulled)
	}
}

func TestWebhook_NoEventID_StillProcessed(t *testing.T) {
	// Older distribution builds may emit events without an `id`. We must
	// still process them — dedup is a best-effort optimization, not a
	// gate on functionality.
	baseURL, secret, stub := newWebhookRig(t)
	resp := postEvents(t, baseURL, secret, map[string]any{
		"events": []any{
			map[string]any{
				// No "id" field.
				"action": "push",
				"target": map[string]any{
					"mediaType":  "application/vnd.oci.image.manifest.v1+json",
					"repository": "alice/app",
				},
			},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	refreshed, _ := stub.snapshot()
	if len(refreshed) != 1 || refreshed[0] != "alice/app" {
		t.Errorf("refreshed = %v, want [alice/app]", refreshed)
	}
}

func TestWebhook_MediaTypeWithParams(t *testing.T) {
	baseURL, secret, stub := newWebhookRig(t)
	// distribution can emit mediaType with `; charset=utf-8` tail; the
	// handler must strip parameters before matching.
	resp := postEvents(t, baseURL, secret, map[string]any{
		"events": []any{
			map[string]any{
				"action": "push",
				"target": map[string]any{
					"mediaType":  "application/vnd.oci.image.manifest.v1+json; charset=utf-8",
					"repository": "alice/app",
				},
			},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	refreshed, _ := stub.snapshot()
	if len(refreshed) != 1 || refreshed[0] != "alice/app" {
		t.Errorf("refreshed = %v, want [alice/app]", refreshed)
	}
}
