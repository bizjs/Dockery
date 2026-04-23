package service

import (
	"context"
	"strings"

	"api/internal/biz"

	"github.com/bizjs/kratoscarf/response"
	"github.com/bizjs/kratoscarf/router"
)

// repoMetaRefresher is the slice of RepoMetaUsecase this handler uses.
// Declaring it as an interface here (Go's structural typing) keeps
// webhook.go testable without spinning up the real refresh worker.
// *biz.RepoMetaUsecase satisfies it naturally.
type repoMetaRefresher interface {
	EnqueueRefresh(repo string)
	IncrementPull(ctx context.Context, repo string)
}

// WebhookService receives distribution's notification events and
// translates them into RepoMeta refresh / pull-count writes. Mounted at
// a public route (no session) because distribution dials the endpoint
// using a Bearer shared secret — sessions don't apply there.
//
// The /api/internal/registry-events URL is a convention: nginx routes
// anything under /api/ to dockery-api, and distribution talks directly
// to dockery-api via loopback (not through nginx), so "internal" in the
// path is just a marker for humans reading the access log.
type WebhookService struct {
	secret *biz.WebhookSecret
	meta   repoMetaRefresher
}

func NewWebhookService(secret *biz.WebhookSecret, meta *biz.RepoMetaUsecase) *WebhookService {
	return &WebhookService{secret: secret, meta: meta}
}

// --- distribution event shape (see distribution notifications docs) ---

type registryEventEnvelope struct {
	Events []registryEvent `json:"events"`
}

type registryEvent struct {
	ID        string              `json:"id"`
	Timestamp string              `json:"timestamp"`
	Action    string              `json:"action"` // push / pull / delete / mount
	Target    registryEventTarget `json:"target"`
}

type registryEventTarget struct {
	MediaType  string `json:"mediaType"`
	Digest     string `json:"digest"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
}

// Handle is mounted at POST /api/internal/registry-events.
//
// Validates the Bearer token, parses the envelope, and for each event:
//   - push on a manifest mediaType    → enqueue refresh for the repo
//   - delete on a manifest mediaType  → enqueue refresh (RefreshOne
//     detects the empty-tags case and deletes the row)
//   - pull on a manifest mediaType    → bump pull_count + last_pulled_at
//   - anything on a blob mediaType    → ignored (noisy, redundant
//     with the manifest-level event that fires in the same push/pull)
func (s *WebhookService) Handle(ctx *router.Context) error {
	if err := s.authorize(ctx); err != nil {
		return err
	}

	var env registryEventEnvelope
	if err := ctx.Bind(&env); err != nil {
		return err
	}

	// Deduplicate repos we'll enqueue — a single push can emit several
	// manifest events (manifest list + each child) and the queue's own
	// dedup handles it, but pre-filtering keeps the log tidy.
	refreshSet := make(map[string]struct{})
	for _, ev := range env.Events {
		if ev.Target.Repository == "" {
			continue
		}
		if !isManifestMediaType(ev.Target.MediaType) {
			continue
		}
		switch ev.Action {
		case "push", "delete":
			refreshSet[ev.Target.Repository] = struct{}{}
		case "pull":
			// Pull counts update immediately; refresh only if we've
			// never seen this repo before (the increment is a no-op on
			// a missing row, but a follow-up refresh will materialize it).
			s.meta.IncrementPull(ctx.Context(), ev.Target.Repository)
		}
	}
	for repo := range refreshSet {
		s.meta.EnqueueRefresh(repo)
	}
	return ctx.Success(nil)
}

func (s *WebhookService) authorize(ctx *router.Context) error {
	auth := ctx.Header("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return response.ErrUnauthorized.WithMessage("missing bearer token")
	}
	if !s.secret.Verify(auth[len(prefix):]) {
		return response.ErrUnauthorized.WithMessage("invalid webhook secret")
	}
	return nil
}

// isManifestMediaType keeps the webhook focused on tag-visible changes.
// Layer blob mediaTypes fire on every chunk upload and would flood the
// refresh queue; the corresponding manifest event is our cue to refetch.
func isManifestMediaType(mt string) bool {
	// Strip any parameters (e.g. "application/...+json; charset=utf-8").
	if i := strings.Index(mt, ";"); i >= 0 {
		mt = strings.TrimSpace(mt[:i])
	}
	switch mt {
	case
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v1+prettyjws",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.oci.image.index.v1+json":
		return true
	}
	return false
}

