package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PlatformInfo is one entry in RepoMeta.Platforms — what `docker inspect`
// would show for a child manifest. Stored as JSON rather than modeled as
// a separate table because it's always read as a whole with the repo.
type PlatformInfo struct {
	Os           string `json:"os,omitempty"`
	Architecture string `json:"architecture,omitempty"`
	Variant      string `json:"variant,omitempty"`
}

// RepoMeta is a denormalized per-repository snapshot that powers the
// Catalog page without fanning out to the upstream registry on every
// request. It's a cache — the authoritative data lives in distribution's
// blob store — kept in sync by:
//
//   - Push events (`/api/internal/registry-events`) → refresh one repo
//   - Delete events → refresh one repo (falls back to delete if empty)
//   - Pull events → bump pull_count + last_pulled_at (no meta fetch)
//   - Startup + periodic reconcile → catch missed events by diffing
//     against /v2/_catalog
type RepoMeta struct {
	ent.Schema
}

func (RepoMeta) Fields() []ent.Field {
	return []ent.Field{
		field.String("repo").NotEmpty().Unique().Immutable(),
		// The tag we fetched meta from. Heuristic: `latest` if present,
		// else the lexicographically-last tag. May be empty for an
		// in-flight refresh.
		field.String("latest_tag").Optional(),
		field.Int("tag_count").Default(0),
		// Registry storage bytes (config + layers, summed across
		// platforms for manifest lists).
		field.Int64("size").Default(0),
		// Image build time (ISO 8601, from config.created). String to
		// avoid timezone-parsing drift vs the raw manifest value.
		field.String("created").Optional(),
		// Per-platform descriptors for multi-arch images; empty/nil
		// for single-arch.
		field.JSON("platforms", []PlatformInfo{}).Optional(),
		// Pull counters — maintained from `pull` webhook events so
		// future dashboards can show activity without a separate table.
		field.Int64("pull_count").Default(0),
		field.Time("last_pulled_at").Optional().Nillable(),
		// Wall-clock of the last successful meta refresh. Used by the
		// reconciler to decide which rows are stale.
		field.Time("refreshed_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (RepoMeta) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("repo").Unique(),
	}
}
