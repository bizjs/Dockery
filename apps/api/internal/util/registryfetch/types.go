// Package registryfetch is the shared HTTP client for dockery-api's
// own calls into the upstream Docker Registry V2 API. It is used by:
//
//   - biz/repo_meta_fetch.go — the webhook-driven refresh path
//   - service/registry.go    — the UI Overview enrichment path
//
// Both need the same primitives (list tags, pull manifest, pull config
// blob, detect manifest lists) and the same authentication story (mint
// a pull-scoped JWT per call). Keeping the implementation in one place
// means a change to mediatypes, timeouts, or the auth wire is made
// exactly once.
package registryfetch

// Access is one entry of a Docker Registry token's `access` claim.
// Callers assemble these and hand them to a TokenIssuer when minting
// an upstream JWT. JSON field names are lowercase per the Docker
// token-auth spec — registry rejects tokens with the Go-default
// capitalized keys.
type Access struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

// TokenIssuer abstracts the single method this package needs from
// biz.TokenIssuer so registryfetch can sit strictly below biz in the
// import graph. biz.TokenIssuer satisfies this naturally once
// biz.RegistryAccess is a type alias for Access.
type TokenIssuer interface {
	IssueRegistryToken(subject string, access []Access) (string, error)
}

// Media-type constants as registered with IANA / documented by
// distribution + OCI. Used both for matching inbound manifest
// mediaTypes and for building the Accept header on outbound requests.
const (
	MediaTypeDockerManifestV1       = "application/vnd.docker.distribution.manifest.v1+json"
	MediaTypeDockerManifestV1Signed = "application/vnd.docker.distribution.manifest.v1+prettyjws"
	MediaTypeDockerManifestV2       = "application/vnd.docker.distribution.manifest.v2+json"
	MediaTypeDockerManifestList     = "application/vnd.docker.distribution.manifest.list.v2+json"
	MediaTypeOCIImageManifest       = "application/vnd.oci.image.manifest.v1+json"
	MediaTypeOCIImageIndex          = "application/vnd.oci.image.index.v1+json"
)

// ManifestAcceptHeader lists every manifest mediatype distribution can
// return so the registry picks the richest format the caller understands
// instead of silently downconverting to v1.
const ManifestAcceptHeader = MediaTypeOCIImageManifest + "," +
	MediaTypeDockerManifestV2 + "," +
	MediaTypeDockerManifestList + "," +
	MediaTypeOCIImageIndex

// Manifest is the union shape of single-arch (image manifest) and
// multi-arch (image index / manifest list) responses. Caller checks
// `IsList()` to branch.
type Manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	// Single-arch fields.
	Config *Descriptor  `json:"config"`
	Layers []Descriptor `json:"layers"`
	// Manifest-list fields.
	Manifests []ManifestListEntry `json:"manifests"`
}

// IsList reports whether the manifest is an image index (lists
// per-platform child manifests). Checks the `manifests` slice first —
// that's the authoritative signal; the mediaType is a fallback for
// responses where the slice is absent but the type is set explicitly.
func (m *Manifest) IsList() bool {
	if len(m.Manifests) > 0 {
		return true
	}
	return m.MediaType == MediaTypeDockerManifestList ||
		m.MediaType == MediaTypeOCIImageIndex
}

// Descriptor is one blob reference (either config or layer).
type Descriptor struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

// ManifestListEntry is one row in a manifest list / OCI image index.
type ManifestListEntry struct {
	MediaType string   `json:"mediaType"`
	Digest    string   `json:"digest"`
	Size      int64    `json:"size"`
	Platform  Platform `json:"platform"`
}

// Platform is the {os, architecture, variant} triple that distinguishes
// child manifests in an image index.
type Platform struct {
	Os           string `json:"os"`
	Architecture string `json:"architecture"`
	Variant      string `json:"variant"`
}

// ConfigBlob is the subset of the image config JSON we actually read.
// Registry returns much more (history, rootfs, env, etc.) — callers
// that need those fields use service.RegistryService.GetBlob which
// passes the full blob through.
type ConfigBlob struct {
	Architecture string `json:"architecture"`
	Os           string `json:"os"`
	Created      string `json:"created"`
}

// ChildMeta is the result of walking one manifest-list entry: the
// total storage bytes for that platform and the build timestamp from
// its config. Best-effort — zero values signal "fetch failed, skip".
type ChildMeta struct {
	Size    int64
	Created string
}
