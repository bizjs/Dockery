package service

import (
	"api/internal/data"

	"github.com/bizjs/kratoscarf/router"
)

// RegistryService proxies UI calls to the upstream Docker distribution
// registry at 127.0.0.1:5001 (in-container). Responsibility split:
//
//   - Dockery API verifies the session cookie and consults the user's
//     role + permissions.
//   - Dockery API issues a short-lived (30s) Ed25519-signed JWT and
//     injects it as "Authorization: Bearer" on the upstream call.
//   - The response is forwarded to the UI; lists (catalog/tags) are
//     additionally filtered so users never see repos they cannot access.
type RegistryService struct {
	data *data.Data
}

func NewRegistryService(d *data.Data) *RegistryService { return &RegistryService{data: d} }

// --- DTOs ---

type CatalogView struct {
	Repositories []string `json:"repositories"`
}

type TagsView struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// --- Handlers ---

// Catalog returns the repositories visible to the caller.
//
// admin → everything; write/view → filtered by repo_permissions patterns.
func (s *RegistryService) Catalog(ctx *router.Context) error {
	// TODO(M3): issue internal JWT → GET /v2/_catalog → filter.
	return errNotImplemented()
}

// Tags returns the tag list for a repository the caller can access.
func (s *RegistryService) Tags(ctx *router.Context) error {
	// TODO(M3): authorize pattern → proxy /v2/{name}/tags/list.
	return errNotImplemented()
}

// GetManifest proxies GET /v2/{name}/manifests/{ref}.
// Requires pull on the repository.
func (s *RegistryService) GetManifest(ctx *router.Context) error {
	// TODO(M3)
	return errNotImplemented()
}

// DeleteManifest performs the two-step delete: HEAD for digest, then
// DELETE by digest. Requires delete on the repository (admin / write).
func (s *RegistryService) DeleteManifest(ctx *router.Context) error {
	// TODO(M3)
	return errNotImplemented()
}

// GetBlob proxies GET /v2/{name}/blobs/{digest} — used by UI to fetch
// the image config blob for detail views (cmd/env/labels/ports).
func (s *RegistryService) GetBlob(ctx *router.Context) error {
	// TODO(M3)
	return errNotImplemented()
}
