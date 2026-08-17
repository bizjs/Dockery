package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// SystemSetting stores system-wide, dynamically editable settings.
// Business usecases own the type and validation rules for individual keys;
// the table provides one shared persistence and concurrency model.
type SystemSetting struct {
	ent.Schema
}

func (SystemSetting) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").
			NotEmpty().
			Immutable().
			Unique(),
		field.JSON("value", json.RawMessage{}),
		field.Int64("version").
			Default(1).
			Positive(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
		field.String("updated_by").
			NotEmpty().
			Default("system"),
	}
}

func (SystemSetting) Annotations() []entschema.Annotation {
	return []entschema.Annotation{
		&entsql.Annotation{
			Table: "system_settings",
			Checks: map[string]string{
				"system_settings_key_nonempty":     "length(key) > 0",
				"system_settings_version_positive": "version > 0",
			},
		},
	}
}
