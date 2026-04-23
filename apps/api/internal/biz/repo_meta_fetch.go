package biz

import (
	"context"
	"sync"

	"api/internal/data/ent/schema"
	"api/internal/util/registryfetch"
)

// Orchestration for RepoMetaUsecase.RefreshOne: pull tags, pick a
// representative one, fetch its manifest + config blob (or walk
// children for manifest lists), and assemble a RepoMeta. All HTTP
// primitives are in internal/util/registryfetch so this file stays
// pure domain logic.

// fetchTags lists tags for a repo. Wrapper around registryfetch.Client
// so the call site stays local to the file doing the work.
func (u *RepoMetaUsecase) fetchTags(ctx context.Context, repo string) ([]string, error) {
	return u.registry.Tags(ctx, repo)
}

// fetchRepoMeta returns the RepoMeta populated from a single tag's
// manifest (plus its config blob, or child manifests for multi-arch).
// Caller fills in Repo/LatestTag/TagCount/RefreshedAt afterwards.
func (u *RepoMetaUsecase) fetchRepoMeta(ctx context.Context, repo, tag string) (*RepoMeta, error) {
	manifest, err := u.registry.Manifest(ctx, repo, tag)
	if err != nil {
		return nil, err
	}
	out := &RepoMeta{}

	if manifest.IsList() {
		// Children are fetched in parallel — concurrent round-trips
		// against the loopback registry are cheap and give a linear
		// speed-up on 3+ platform images. Results are written into
		// per-index slots so the order matches `manifest.Manifests`
		// without a shared mutex.
		entries := manifest.Manifests
		platforms := make([]schema.PlatformInfo, len(entries))
		metas := make([]registryfetch.ChildMeta, len(entries))

		var wg sync.WaitGroup
		for i := range entries {
			entry := entries[i]
			platforms[i] = schema.PlatformInfo{
				Os:           entry.Platform.Os,
				Architecture: entry.Platform.Architecture,
				Variant:      entry.Platform.Variant,
			}
			if entry.Digest == "" {
				continue
			}
			wg.Add(1)
			go func(idx int, digest string) {
				defer wg.Done()
				metas[idx] = u.registry.ChildMeta(ctx, repo, digest)
			}(i, entry.Digest)
		}
		wg.Wait()

		var total int64
		var latestCreated string
		for i, m := range metas {
			total += m.Size
			// Skip attestation manifests (unknown/unknown) from the
			// `created` max — their config timestamp is the generator's
			// time, not a real image build.
			if entries[i].Platform.Os == "unknown" {
				continue
			}
			if m.Created != "" && m.Created > latestCreated {
				latestCreated = m.Created
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
		cfg, err := u.registry.ConfigBlob(ctx, repo, manifest.Config.Digest)
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

