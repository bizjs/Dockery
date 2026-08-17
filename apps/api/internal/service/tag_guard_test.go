package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"api/internal/biz"
	"api/internal/data/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/bizjs/kratoscarf/router"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"

	_ "modernc.org/sqlite"
)

func newGuardPolicy(t *testing.T, persisted *bool) *biz.RegistryPolicyBiz {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "guard.db") + "?_pragma=foreign_keys(1)"
	stdDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, stdDB)))
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if persisted != nil {
		value, err := json.Marshal(*persisted)
		if err != nil {
			t.Fatalf("marshal setting: %v", err)
		}
		if _, err := client.SystemSetting.Create().
			SetKey(biz.SettingKeyPreventTagOverwrite).
			SetValue(value).
			SetVersion(1).
			SetUpdatedBy("admin").
			Save(context.Background()); err != nil {
			t.Fatalf("seed setting: %v", err)
		}
	}
	policy := biz.NewRegistryPolicyBiz(client)
	if err := policy.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize policy: %v", err)
	}
	return policy
}

func newGuardIssuer(t *testing.T) *biz.TokenIssuer {
	t.Helper()
	dir := t.TempDir()
	keys, err := biz.NewKeystore(biz.KeystoreConfig{
		PrivatePath: filepath.Join(dir, "registry.key"),
		JWKSPath:    filepath.Join(dir, "jwks.json"),
	})
	if err != nil {
		t.Fatalf("new keystore: %v", err)
	}
	issuer, err := biz.NewTokenIssuer(keys, biz.TokenIssuerConfig{
		Issuer: "dockery-api", Audience: "dockery", TTL: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("new token issuer: %v", err)
	}
	return issuer
}

func newGuardHTTPServer(t *testing.T, guard *TagGuardService) *httptest.Server {
	t.Helper()
	server := kratoshttp.NewServer()
	r := router.NewRouter(server)
	r.PUT("/v2/{name:.+}/manifests/{reference}", guard.PutManifest)
	return httptest.NewServer(server.Handler)
}

func registryDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func doManifestPut(t *testing.T, client *http.Client, baseURL, token string, body []byte) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, baseURL+"/v2/team/app/manifests/latest", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("manifest PUT: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp, raw
}

func TestTagGuardRejectsOverwriteAndAllowsIdempotentPush(t *testing.T) {
	body := []byte(`{"schemaVersion":2}`)
	digest := registryDigest(body)
	var mu sync.Mutex
	currentDigest := "sha256:" + strings.Repeat("a", 64)
	putCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Docker-Content-Digest", currentDigest)
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			putCount++
			w.Header().Set("Docker-Content-Digest", digest)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer upstream.Close()

	enabled := true
	policy := newGuardPolicy(t, &enabled)
	issuer := newGuardIssuer(t)
	guard := NewTagGuardService(policy, issuer, nil, biz.RegistryUpstreamURL(upstream.URL), "http://dockery.test/token")
	api := newGuardHTTPServer(t, guard)
	defer api.Close()
	token, err := issuer.IssueRegistryToken("alice", []biz.RegistryAccess{{
		Type: "repository", Name: "team/app", Actions: []string{"push"},
	}})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	resp, raw := doManifestPut(t, api.Client(), api.URL, token, body)
	if resp.StatusCode != http.StatusConflict || !bytes.Contains(raw, []byte(`"code":"DENIED"`)) {
		t.Fatalf("overwrite want registry 409/DENIED, got %d body=%s", resp.StatusCode, raw)
	}
	mu.Lock()
	if putCount != 0 {
		t.Fatalf("rejected overwrite reached upstream PUT %d times", putCount)
	}
	currentDigest = digest
	mu.Unlock()

	resp, raw = doManifestPut(t, api.Client(), api.URL, token, body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("same-digest PUT want 201, got %d body=%s", resp.StatusCode, raw)
	}
	mu.Lock()
	defer mu.Unlock()
	if putCount != 1 {
		t.Fatalf("idempotent PUT count=%d, want 1", putCount)
	}
}

func TestTagGuardAllowsConfiguredOverwriteExclusion(t *testing.T) {
	body := []byte(`{"schemaVersion":2}`)
	headCount := 0
	putCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			headCount++
			w.Header().Set("Docker-Content-Digest", "sha256:"+strings.Repeat("a", 64))
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			putCount++
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer upstream.Close()

	policy := newGuardPolicy(t, nil)
	if _, err := policy.Update(context.Background(), 0, true, []string{"latest"}, "admin"); err != nil {
		t.Fatalf("enable policy with exclusion: %v", err)
	}
	issuer := newGuardIssuer(t)
	api := newGuardHTTPServer(t, NewTagGuardService(
		policy,
		issuer,
		nil,
		biz.RegistryUpstreamURL(upstream.URL),
		"http://dockery.test/token",
	))
	defer api.Close()

	// Excluded tags bypass the overwrite HEAD check and are transparently
	// forwarded. Distribution remains responsible for final authentication.
	resp, raw := doManifestPut(t, api.Client(), api.URL, "", body)
	if resp.StatusCode != http.StatusCreated || headCount != 0 || putCount != 1 {
		t.Fatalf("excluded latest want transparent 201, got %d head=%d put=%d body=%s",
			resp.StatusCode, headCount, putCount, raw)
	}
}

func TestTagGuardAllowsNewTagAndFailsClosedOnHeadError(t *testing.T) {
	body := []byte(`{"schemaVersion":2}`)
	headStatus := http.StatusNotFound
	putCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(headStatus)
		case http.MethodPut:
			putCount++
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer upstream.Close()
	enabled := true
	policy := newGuardPolicy(t, &enabled)
	issuer := newGuardIssuer(t)
	api := newGuardHTTPServer(t, NewTagGuardService(policy, issuer, nil, biz.RegistryUpstreamURL(upstream.URL), ""))
	defer api.Close()
	token, err := issuer.IssueRegistryToken("alice", []biz.RegistryAccess{{Type: "repository", Name: "team/app", Actions: []string{"push"}}})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	resp, raw := doManifestPut(t, api.Client(), api.URL, token, body)
	if resp.StatusCode != http.StatusCreated || putCount != 1 {
		t.Fatalf("new tag want 201 and one PUT, got %d count=%d body=%s", resp.StatusCode, putCount, raw)
	}
	headStatus = http.StatusInternalServerError
	resp, raw = doManifestPut(t, api.Client(), api.URL, token, body)
	if resp.StatusCode != http.StatusServiceUnavailable || putCount != 1 {
		t.Fatalf("HEAD failure want fail-closed 503, got %d count=%d body=%s", resp.StatusCode, putCount, raw)
	}
}

func TestTagGuardDisabledIsTransparentAndEnabledRequiresToken(t *testing.T) {
	body := []byte(`{"schemaVersion":2}`)
	putCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putCount++
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer upstream.Close()
	policy := newGuardPolicy(t, nil)
	issuer := newGuardIssuer(t)
	api := newGuardHTTPServer(t, NewTagGuardService(policy, issuer, nil, biz.RegistryUpstreamURL(upstream.URL), "http://dockery.test/token"))
	defer api.Close()

	resp, raw := doManifestPut(t, api.Client(), api.URL, "", body)
	if resp.StatusCode != http.StatusCreated || putCount != 1 {
		t.Fatalf("disabled guard want transparent 201, got %d count=%d body=%s", resp.StatusCode, putCount, raw)
	}
	if _, err := policy.Update(context.Background(), 0, true, nil, "admin"); err != nil {
		t.Fatalf("enable policy: %v", err)
	}
	resp, raw = doManifestPut(t, api.Client(), api.URL, "", body)
	if resp.StatusCode != http.StatusUnauthorized || putCount != 1 {
		t.Fatalf("enabled guard without token want 401, got %d count=%d body=%s", resp.StatusCode, putCount, raw)
	}
	if got := resp.Header.Get("WWW-Authenticate"); !strings.Contains(got, `scope="repository:team/app:pull,push"`) {
		t.Fatalf("unexpected registry auth challenge %q", got)
	}
}
