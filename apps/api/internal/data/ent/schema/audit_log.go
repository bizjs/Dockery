package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AuditLog records authentication and administrative events for
// post-hoc review. Written append-only; never updated.
//
// Canonical action values:
//
//	token.issued         — /token endpoint granted a JWT
//	token.denied         — /token endpoint rejected credentials
//	user.created / updated / deleted / password_changed
//	permission.granted / revoked
//	image.deleted        — UI-triggered manifest deletion
//	gc.started / completed
//	key.rotated
type AuditLog struct {
	ent.Schema
}

func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.Time("ts").
			Default(time.Now).
			Immutable(),
		field.String("actor").NotEmpty(), // username (or "anonymous")
		field.String("action").NotEmpty(),
		field.String("target").Optional(),    // e.g. "repository:alice/app"
		field.String("scope").Optional(),     // granted actions, CSV
		field.String("client_ip").Optional(),
		field.Bool("success").Default(true),
		field.JSON("detail", map[string]any{}).Optional(),
	}
}

func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ts"),
		index.Fields("actor"),
		index.Fields("action"),
	}
}
