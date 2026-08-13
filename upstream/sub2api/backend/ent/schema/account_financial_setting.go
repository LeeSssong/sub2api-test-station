package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// AccountFinancialSetting stores the feature enablement boundary. The setting
// is created before activation, so enabled_at remains nullable until enabled.
type AccountFinancialSetting struct {
	ent.Schema
}

func (AccountFinancialSetting) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "account_financial_settings"},
	}
}

func (AccountFinancialSetting) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").MaxLen(100).Unique().DefaultFunc(func() string { return "t03_r1_account_financial" }),
		field.Time("enabled_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}
