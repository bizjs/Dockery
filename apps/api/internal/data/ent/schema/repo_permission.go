package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RepoPermission scopes a non-admin user to repositories matching a
// glob pattern. Available actions on a matched repository are determined
// by the user's role (write → pull+push+delete, view → pull), so this
// table does NOT store actions — it only answers "which repos".
//
// repo_pattern supports:
//
//	*          — any repository
//	alice/*    — any repository under "alice/"
//	alice/app  — exact match
//
// Multiple rows for the same user are OR-ed together. The management API
// accepts a comma-separated list of patterns in a single request and
// splits them into one row per pattern for straightforward indexing.
type RepoPermission struct {
	ent.Schema
}

func (RepoPermission) Fields() []ent.Field {
	return []ent.Field{
		field.String("repo_pattern").NotEmpty(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (RepoPermission) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("permissions").
			Unique().
			Required(),
	}
}

func (RepoPermission) Indexes() []ent.Index {
	return []ent.Index{
		// A given pattern can only appear once per user; overlapping
		// patterns across different rows are allowed.
		index.Edges("user").Fields("repo_pattern").Unique(),
	}
}
