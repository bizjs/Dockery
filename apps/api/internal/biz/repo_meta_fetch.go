package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"api/internal/data/ent/schema"
)

// Fetch helpers for RepoMetaUsecase. Isolated here so repo_meta.go stays
// focused on orchestration (queue, worker, upsert). These duplicate some
// of service/registry.go's logic (manifest list enrichment, child meta
// fetch) — the duplication is deliberate: service does it on-demand
// against each UI request, we do it ambient on webhook events, and
// coupling both paths through a shared helper made wire + testability
// noisier than the redundancy is worth. If both grow more logic, merge
// into a single internal/pkg/registryfetch package.

const manifestListMediaTypes = "application/vnd.docker.distribution.manifest.list.v2+json," +
	"application/vnd.oci.image.index.v1+json"

const manifestAccept = "application/vnd.oci.image.manifest.v1+json," +
	"application/vnd.docker.distribution.manifest.v2+json," +
	manifestListMediaTypes

type tagsListResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type manifestEnvelope struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	// Single-arch
	Config *struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"layers"`
	// Manifest list
	Manifests []manifestListEntry `json:"manifests"`
}

type manifestListEntry struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	Platform  struct {
		Os           string `json:"os"`
		Architecture string `json:"architecture"`
		Variant      string `json:"variant"`
	} `json:"platform"`
}

type configBlob struct {
	Architecture string `json:"architecture"`
	Os           string `json:"os"`
	Created      string `json:"created"`
}

// fetchTags lists tags for a repo via the upstream /v2/tags/list. Nil
// tags (distribution returns `{"tags": null}` after the last tag is
// removed) flatten to an empty slice.
func (u *RepoMetaUsecase) fetchTags(ctx context.Context, repo string) ([]string, error) {
	url := fmt.Sprintf("%s/v2/%s/tags/list", u.upstreamURL, repo)
	body, err := u.getUpstream(ctx, repo, url, "")
	if err != nil {
		return nil, fmt.Errorf("fetch tags: %w", err)
	}
	var resp tagsListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse tags: %w", err)
	}
	if resp.Tags == nil {
		return []string{}, nil
	}
	return resp.Tags, nil
}

// fetchRepoMeta returns the RepoMeta populated from a single tag's
// manifest (plus its config blob, or child manifests for multi-arch).
// Caller fills in Repo/LatestTag/TagCount/RefreshedAt afterwards.
func (u *RepoMetaUsecase) fetchRepoMeta(ctx context.Context, repo, tag string) (*RepoMeta, error) {
	manifest, err := u.fetchManifest(ctx, repo, tag)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	out := &RepoMeta{}

	if isManifestList(manifest) {
		// Walk each child manifest in parallel-friendly sequential mode
		// (goroutines would require yet another sync layer for 2-3
		// platforms — not worth it). The total size + latest created
		// across runnable children become the repo-level values.
		var total int64
		var latestCreated string
		platforms := make([]schema.PlatformInfo, 0, len(manifest.Manifests))
		for _, entry := range manifest.Manifests {
			platforms = append(platforms, schema.PlatformInfo{
				Os:           entry.Platform.Os,
				Architecture: entry.Platform.Architecture,
				Variant:      entry.Platform.Variant,
			})
			if entry.Digest == "" {
				continue
			}
			size, created := u.fetchChildMeta(ctx, repo, entry.Digest)
			total += size
			// Skip attestation manifests (unknown/unknown) from the
			// `created` max — their config timestamp is the generator's
			// time, not a real image build.
			if entry.Platform.Os == "unknown" {
				continue
			}
			if created != "" && created > latestCreated {
				latestCreated = created
			}
		}
		out.Size = total
		out.Created = latestCreated
		out.Platforms = platforms
		return out, nil
	}

	// Single-arch: config.size + Σ layers[].size. Config blob has
	// `created`.
	if manifest.Config != nil {
		out.Size += manifest.Config.Size
	}
	for _, l := range manifest.Layers {
		out.Size += l.Size
	}
	if manifest.Config != nil && manifest.Config.Digest != "" {
		cfg, err := u.fetchConfigBlob(ctx, repo, manifest.Config.Digest)
		if err == nil {
			out.Created = cfg.Created
			if cfg.Os != "" || cfg.Architecture != "" {
				out.Platforms = []schema.PlatformInfo{{
					Os:           cfg.Os,
					Architecture: cfg.Architecture,
				}}
			}
		}
	}
	return out, nil
}

// fetchChildMeta pulls a single child manifest inside a manifest list
// and returns (size, created). Best-effort — returns zero values on any
// failure so the caller keeps processing other children.
func (u *RepoMetaUsecase) fetchChildMeta(ctx context.Context, repo, digest string) (int64, string) {
	m, err := u.fetchManifest(ctx, repo, digest)
	if err != nil {
		return 0, ""
	}
	var size int64
	if m.Config != nil {
		size += m.Config.Size
	}
	for _, l := range m.Layers {
		size += l.Size
	}
	var created string
	if m.Config != nil && m.Config.Digest != "" {
		if cfg, err := u.fetchConfigBlob(ctx, repo, m.Config.Digest); err == nil {
			created = cfg.Created
		}
	}
	return size, created
}

func (u *RepoMetaUsecase) fetchManifest(ctx context.Context, repo, ref string) (*manifestEnvelope, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", u.upstreamURL, repo, ref)
	body, err := u.getUpstream(ctx, repo, url, manifestAccept)
	if err != nil {
		return nil, err
	}
	var out manifestEnvelope
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &out, nil
}

func (u *RepoMetaUsecase) fetchConfigBlob(ctx context.Context, repo, digest string) (*configBlob, error) {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", u.upstreamURL, repo, digest)
	body, err := u.getUpstream(ctx, repo, url, "")
	if err != nil {
		return nil, err
	}
	var out configBlob
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse config blob: %w", err)
	}
	return &out, nil
}

// getUpstream mints an admin-scoped JWT for `repo` and issues the GET.
// Returns the response body; treats non-200 as an error.
func (u *RepoMetaUsecase) getUpstream(ctx context.Context, repo, url, accept string) ([]byte, error) {
	access := []RegistryAccess{
		{Type: "repository", Name: repo, Actions: []string{"pull"}},
	}
	token, err := u.tokens.IssueRegistryToken("dockery-repo-meta", access)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %s → %d: %s", url, resp.StatusCode, truncate(body))
	}
	return body, nil
}

func isManifestList(m *manifestEnvelope) bool {
	if len(m.Manifests) > 0 {
		return true
	}
	switch m.MediaType {
	case "application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.index.v1+json":
		return true
	}
	return false
}

func truncate(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}
