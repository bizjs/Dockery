package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// User represents a Dockery account usable from both docker CLI and Web UI.
//
// Role determines the set of actions a user can perform on any repository
// their permissions match:
//
//	admin — full access to everything; bypasses repo_permissions (registry:catalog:*)
//	write — pull + push + delete on matched repositories
//	view  — pull only on matched repositories
//
// For admin users, repo_permissions rows are ignored. For write/view users,
// at least one RepoPermission row must match the requested repository name
// for any action to be granted.
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("username").
			NotEmpty().
			Unique().
			Immutable(),
		field.String("password_hash").
			NotEmpty().
			Sensitive(),
		field.Enum("role").
			Values("admin", "write", "view").
			Default("view"),
		field.Bool("disabled").
			Default(false),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("permissions", RepoPermission.Type),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("username").Unique(),
	}
}
