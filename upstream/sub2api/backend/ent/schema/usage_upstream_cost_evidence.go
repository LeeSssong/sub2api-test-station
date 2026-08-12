package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UsageUpstreamCostEvidence stores the single terminal upstream billing result
// associated with an official usage log.
type UsageUpstreamCostEvidence struct {
	ent.Schema
}

func (UsageUpstreamCostEvidence) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "usage_upstream_cost_evidence"},
	}
}

func (UsageUpstreamCostEvidence) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("usage_log_id").Unique(),
		field.Enum("source").Values("sub", "newapi"),
		field.String("upstream_request_id").Optional().Nillable().MaxLen(255),
		field.Time("upstream_billing_time").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("upstream_model").Optional().Nillable().MaxLen(255),
		field.Float("sub_actual_cost").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Float("newapi_quota").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Float("newapi_quota_per_unit").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Float("normalized_cost_cny").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Float("profit_cny").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Enum("evidence_status").Values("confirmed", "confirmed_zero", "unavailable"),
		field.String("reason_code").Optional().Nillable().MaxLen(64),
		field.Time("recorded_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UsageUpstreamCostEvidence) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("usage_log", UsageLog.Type).
			Field("usage_log_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (UsageUpstreamCostEvidence) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("evidence_status", "usage_log_id").
			StorageKey("idx_usage_upstream_cost_evidence_status_usage_log_id"),
	}
}
