package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type deleteCacheRegistry struct {
	mu   sync.Mutex
	tags map[string]string
}

func newDeleteCacheRegistry() *deleteCacheRegistry {
	return newDeleteCacheRegistryWithTags(map[string]string{
		"1.0.0": "sha256:one",
		"2.0.0": "sha256:two",
	})
}

func newDeleteCacheRegistryWithTags(tags map[string]string) *deleteCacheRegistry {
	return &deleteCacheRegistry{tags: tags}
}

func (r *deleteCacheRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch {
	case req.Method == http.MethodGet && req.URL.Path == "/v2/team/app/tags/list":
		tags := make([]string, 0, len(r.tags))
		for tag := range r.tags {
			tags = append(tags, tag)
		}
		slices.Sort(tags)
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "team/app", "tags": tags})

	case (req.Method == http.MethodGet || req.Method == http.MethodHead) &&
		strings.HasPrefix(req.URL.Path, "/v2/team/app/manifests/"):
		ref := strings.TrimPrefix(req.URL.Path, "/v2/team/app/manifests/")
		digest, ok := r.tags[ref]
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		if req.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaVersion": 2,
				"mediaType":     "application/vnd.oci.image.manifest.v1+json",
				"config": map[string]any{
					"digest": "sha256:config-" + ref,
					"size":   100,
				},
				"layers": []any{},
			})
		}

	case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/v2/team/app/blobs/"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"architecture": "amd64",
			"os":           "linux",
			"created":      "2026-08-18T00:00:00Z",
		})

	case req.Method == http.MethodDelete && strings.HasPrefix(req.URL.Path, "/v2/team/app/manifests/"):
		digest := strings.TrimPrefix(req.URL.Path, "/v2/team/app/manifests/")
		for tag, current := range r.tags {
			if current == digest {
				delete(r.tags, tag)
			}
		}
		w.WriteHeader(http.StatusAccepted)

	default:
		http.NotFound(w, req)
	}
}

func TestDeleteManifestRefreshesCatalogRepresentativeTag(t *testing.T) {
	registry := httptest.NewServer(newDeleteCacheRegistry())
	defer registry.Close()

	h := newHarnessWithUpstream(t, registry.URL)
	defer h.stop()

	ctx := context.Background()
	if err := h.users.EnsureAdmin(ctx, "admin", "a-strong-password-42"); err != nil {
		t.Fatalf("ensure admin: %v", err)
	}
	if err := h.meta.RefreshOne(ctx, "team/app"); err != nil {
		t.Fatalf("seed catalog cache: %v", err)
	}

	// Prime the session cookie before login; see TestAdminFlow.
	h.do(http.MethodGet, "/api/auth/me", nil)
	resp, raw := h.do(http.MethodPost, "/api/auth/login", map[string]string{
		"username": "admin", "password": "a-strong-password-42",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login want 200, got %d; body=%s", resp.StatusCode, raw)
	}

	assertLatestTag(t, h, "2.0.0")

	resp, raw = h.do(http.MethodDelete, "/api/registry/team/app/manifests/2.0.0", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete want 200, got %d; body=%s", resp.StatusCode, raw)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if latestTag(t, h) == "1.0.0" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("catalog latest_tag remained %q after deleting representative tag; want 1.0.0", latestTag(t, h))
}

func TestDeleteLastManifestRemovesRepositoryFromCatalog(t *testing.T) {
	registry := httptest.NewServer(newDeleteCacheRegistryWithTags(map[string]string{
		"1.0.0": "sha256:one",
	}))
	defer registry.Close()

	h := newHarnessWithUpstream(t, registry.URL)
	defer h.stop()

	ctx := context.Background()
	if err := h.users.EnsureAdmin(ctx, "admin", "a-strong-password-42"); err != nil {
		t.Fatalf("ensure admin: %v", err)
	}
	if err := h.meta.RefreshOne(ctx, "team/app"); err != nil {
		t.Fatalf("seed catalog cache: %v", err)
	}
	h.do(http.MethodGet, "/api/auth/me", nil)
	resp, raw := h.do(http.MethodPost, "/api/auth/login", map[string]string{
		"username": "admin", "password": "a-strong-password-42",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login want 200, got %d; body=%s", resp.StatusCode, raw)
	}

	assertLatestTag(t, h, "1.0.0")

	resp, raw = h.do(http.MethodDelete, "/api/registry/team/app/manifests/1.0.0", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete want 200, got %d; body=%s", resp.StatusCode, raw)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, found := catalogLatestTag(t, h); !found {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("repository remained in catalog after deleting its last tag")
}

func assertLatestTag(t *testing.T, h *harness, want string) {
	t.Helper()
	if got := latestTag(t, h); got != want {
		t.Fatalf("latest_tag = %q, want %q", got, want)
	}
}

func latestTag(t *testing.T, h *harness) string {
	t.Helper()
	tag, _ := catalogLatestTag(t, h)
	return tag
}

func catalogLatestTag(t *testing.T, h *harness) (string, bool) {
	t.Helper()
	resp, raw := h.do(http.MethodGet, "/api/registry/overview", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overview want 200, got %d; body=%s", resp.StatusCode, raw)
	}
	var page struct {
		Items []struct {
			Repo      string `json:"repo"`
			LatestTag string `json:"latest_tag"`
		} `json:"items"`
	}
	if err := json.Unmarshal(h.decode(raw).Data, &page); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	for _, item := range page.Items {
		if item.Repo == "team/app" {
			return item.LatestTag, true
		}
	}
	return "", false
}
