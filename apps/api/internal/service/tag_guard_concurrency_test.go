package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"api/internal/biz"
)

type concurrentManifestResult struct {
	status int
	body   []byte
	err    error
}

func concurrentManifestPut(
	client *http.Client,
	baseURL, token, repository, reference string,
	body []byte,
) concurrentManifestResult {
	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/v2/%s/manifests/%s", baseURL, repository, reference),
		bytes.NewReader(body),
	)
	if err != nil {
		return concurrentManifestResult{err: err}
	}
	req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return concurrentManifestResult{err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return concurrentManifestResult{status: resp.StatusCode, body: raw, err: err}
}

func newConcurrentGuard(t *testing.T, upstreamURL string) (*httptest.Server, string) {
	t.Helper()
	enabled := true
	policy := newGuardPolicy(t, &enabled)
	issuer := newGuardIssuer(t)
	api := newGuardHTTPServer(t, NewTagGuardService(
		policy,
		issuer,
		nil,
		biz.RegistryUpstreamURL(upstreamURL),
		"http://dockery.test/token",
	))
	t.Cleanup(api.Close)
	token, err := issuer.IssueRegistryToken("alice", []biz.RegistryAccess{{
		Type: "repository", Name: "team/app", Actions: []string{"push"},
	}})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return api, token
}

func collectConcurrentResults(
	t *testing.T,
	results <-chan concurrentManifestResult,
	count int,
) []concurrentManifestResult {
	t.Helper()
	collected := make([]concurrentManifestResult, 0, count)
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for len(collected) < count {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("manifest PUT: %v", result.err)
			}
			collected = append(collected, result)
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d concurrent manifest PUTs; received %d", count, len(collected))
		}
	}
	return collected
}

func TestTagGuardConcurrentFirstWritesAllowOnlyOneDigest(t *testing.T) {
	bodyA := []byte(`{"schemaVersion":2,"config":{"digest":"sha256:a"}}`)
	bodyB := []byte(`{"schemaVersion":2,"config":{"digest":"sha256:b"}}`)

	var mu sync.Mutex
	currentDigest := ""
	headCount := 0
	putCount := 0
	firstPutStarted := make(chan struct{})
	releaseFirstPut := make(chan struct{})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			mu.Lock()
			headCount++
			digest := currentDigest
			mu.Unlock()
			if digest == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Docker-Content-Digest", digest)
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			putCount++
			ordinal := putCount
			mu.Unlock()
			if ordinal == 1 {
				close(firstPutStarted)
				<-releaseFirstPut
			}
			digest := registryDigest(raw)
			mu.Lock()
			currentDigest = digest
			mu.Unlock()
			w.Header().Set("Docker-Content-Digest", digest)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer upstream.Close()

	api, token := newConcurrentGuard(t, upstream.URL)
	client := api.Client()
	client.Timeout = 2 * time.Second
	start := make(chan struct{})
	results := make(chan concurrentManifestResult, 2)
	for _, body := range [][]byte{bodyA, bodyB} {
		body := body
		go func() {
			<-start
			results <- concurrentManifestPut(client, api.URL, token, "team/app", "latest", body)
		}()
	}
	close(start)

	select {
	case <-firstPutStarted:
	case <-time.After(time.Second):
		close(releaseFirstPut)
		t.Fatal("first upstream PUT did not start")
	}
	// Give the competing request time to reach the same tag lock while the
	// first check+PUT critical section is deliberately held open.
	time.Sleep(75 * time.Millisecond)
	close(releaseFirstPut)

	got := collectConcurrentResults(t, results, 2)
	statuses := []int{got[0].status, got[1].status}
	sort.Ints(statuses)
	if statuses[0] != http.StatusCreated || statuses[1] != http.StatusConflict {
		t.Fatalf("concurrent first writes statuses = %v, want [201 409]; bodies=%q, %q",
			statuses, got[0].body, got[1].body)
	}

	mu.Lock()
	finalDigest := currentDigest
	finalHeadCount := headCount
	finalPutCount := putCount
	mu.Unlock()
	if finalHeadCount != 2 || finalPutCount != 1 {
		t.Fatalf("upstream calls: HEAD=%d PUT=%d, want HEAD=2 PUT=1", finalHeadCount, finalPutCount)
	}
	if finalDigest != registryDigest(bodyA) && finalDigest != registryDigest(bodyB) {
		t.Fatalf("final digest %q does not match either submitted manifest", finalDigest)
	}
}

func TestTagGuardConcurrentSameDigestWritesBothSucceed(t *testing.T) {
	body := []byte(`{"schemaVersion":2,"config":{"digest":"sha256:same"}}`)
	wantDigest := registryDigest(body)
	var mu sync.Mutex
	currentDigest := ""
	putCount := 0

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			mu.Lock()
			digest := currentDigest
			mu.Unlock()
			if digest == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Docker-Content-Digest", digest)
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			putCount++
			currentDigest = registryDigest(raw)
			digest := currentDigest
			mu.Unlock()
			w.Header().Set("Docker-Content-Digest", digest)
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer upstream.Close()

	api, token := newConcurrentGuard(t, upstream.URL)
	start := make(chan struct{})
	results := make(chan concurrentManifestResult, 2)
	for range 2 {
		go func() {
			<-start
			results <- concurrentManifestPut(api.Client(), api.URL, token, "team/app", "latest", body)
		}()
	}
	close(start)

	got := collectConcurrentResults(t, results, 2)
	if got[0].status != http.StatusCreated || got[1].status != http.StatusCreated {
		t.Fatalf("same-digest concurrent statuses = [%d %d], want [201 201]", got[0].status, got[1].status)
	}
	mu.Lock()
	defer mu.Unlock()
	if putCount != 2 || currentDigest != wantDigest {
		t.Fatalf("same-digest upstream state: PUT=%d digest=%q, want PUT=2 digest=%q",
			putCount, currentDigest, wantDigest)
	}
}

func TestTagGuardConcurrentSecondWriteCanProceedAfterFirstUpstreamFailure(t *testing.T) {
	bodyA := []byte(`{"schemaVersion":2,"config":{"digest":"sha256:a"}}`)
	bodyB := []byte(`{"schemaVersion":2,"config":{"digest":"sha256:b"}}`)
	var mu sync.Mutex
	currentDigest := ""
	putCount := 0

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			mu.Lock()
			digest := currentDigest
			mu.Unlock()
			if digest == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Docker-Content-Digest", digest)
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			putCount++
			ordinal := putCount
			mu.Unlock()
			if ordinal == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			digest := registryDigest(raw)
			mu.Lock()
			currentDigest = digest
			mu.Unlock()
			w.Header().Set("Docker-Content-Digest", digest)
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer upstream.Close()

	api, token := newConcurrentGuard(t, upstream.URL)
	start := make(chan struct{})
	results := make(chan concurrentManifestResult, 2)
	for _, body := range [][]byte{bodyA, bodyB} {
		body := body
		go func() {
			<-start
			results <- concurrentManifestPut(api.Client(), api.URL, token, "team/app", "latest", body)
		}()
	}
	close(start)

	got := collectConcurrentResults(t, results, 2)
	statuses := []int{got[0].status, got[1].status}
	sort.Ints(statuses)
	if statuses[0] != http.StatusCreated || statuses[1] != http.StatusInternalServerError {
		t.Fatalf("upstream-failure statuses = %v, want [201 500]", statuses)
	}
	mu.Lock()
	defer mu.Unlock()
	if putCount != 2 {
		t.Fatalf("upstream PUT count = %d, want 2", putCount)
	}
	if currentDigest != registryDigest(bodyA) && currentDigest != registryDigest(bodyB) {
		t.Fatalf("final digest %q does not match the successful manifest", currentDigest)
	}
}

func TestTagGuardConcurrentDifferentTagsDoNotBlockEachOther(t *testing.T) {
	var mu sync.Mutex
	putEntered := 0
	bothPutsEntered := make(chan struct{})
	releasePuts := make(chan struct{})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPut:
			mu.Lock()
			putEntered++
			if putEntered == 2 {
				close(bothPutsEntered)
			}
			mu.Unlock()
			<-releasePuts
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer upstream.Close()

	api, token := newConcurrentGuard(t, upstream.URL)
	client := api.Client()
	client.Timeout = 2 * time.Second
	start := make(chan struct{})
	results := make(chan concurrentManifestResult, 2)
	for _, tag := range []string{"release-a", "release-b"} {
		tag := tag
		go func() {
			<-start
			results <- concurrentManifestPut(
				client,
				api.URL,
				token,
				"team/app",
				tag,
				[]byte(`{"schemaVersion":2}`),
			)
		}()
	}
	close(start)

	overlapped := false
	select {
	case <-bothPutsEntered:
		overlapped = true
	case <-time.After(time.Second):
	}
	close(releasePuts)
	got := collectConcurrentResults(t, results, 2)
	if !overlapped {
		t.Fatal("different tags did not reach upstream concurrently")
	}
	if got[0].status != http.StatusCreated || got[1].status != http.StatusCreated {
		t.Fatalf("different-tag concurrent statuses = [%d %d], want [201 201]", got[0].status, got[1].status)
	}
}

func TestTagGuardConcurrentCrossedMultiTagRequestsDoNotDeadlock(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPut:
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer upstream.Close()

	api, token := newConcurrentGuard(t, upstream.URL)
	client := api.Client()
	client.Timeout = 2 * time.Second
	digest := "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	references := []string{
		digest + "?tag=release-a&tag=release-b",
		digest + "?tag=release-b&tag=release-a",
	}
	start := make(chan struct{})
	results := make(chan concurrentManifestResult, 2)
	for _, reference := range references {
		reference := reference
		go func() {
			<-start
			results <- concurrentManifestPut(
				client,
				api.URL,
				token,
				"team/app",
				reference,
				[]byte(`{"schemaVersion":2}`),
			)
		}()
	}
	close(start)

	got := collectConcurrentResults(t, results, 2)
	if got[0].status != http.StatusCreated || got[1].status != http.StatusCreated {
		t.Fatalf("crossed multi-tag statuses = [%d %d], want [201 201]", got[0].status, got[1].status)
	}
}
