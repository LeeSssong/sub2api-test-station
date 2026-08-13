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

// UsageCostReview stores the immutable administrator confirmation for one
// exceptional usage cost.
type UsageCostReview struct {
	ent.Schema
}

func (UsageCostReview) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "usage_cost_reviews"},
	}
}

func (UsageCostReview) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("usage_log_id").Unique(),
		field.Enum("review_status").Values("reviewed").Default("reviewed"),
		field.Float("manual_cost_cny").Default(0).SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Float("manual_profit_cny").SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Int64("reviewed_by"),
		field.Time("reviewed_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UsageCostReview) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("usage_log", UsageLog.Type).
			Field("usage_log_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (UsageCostReview) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("usage_log_id").
			Unique().
			StorageKey("usage_cost_reviews_usage_log_id_key"),
		index.Fields("usage_log_id").
			StorageKey("idx_usage_cost_reviews_usage_log_id"),
	}
}
