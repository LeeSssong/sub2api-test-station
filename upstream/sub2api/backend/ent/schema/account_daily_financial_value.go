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

// AccountDailyFinancialValue stores Beijing-day OAuth cost and independent
// revenue/cost override cutoffs for an account.
type AccountDailyFinancialValue struct {
	ent.Schema
}

func (AccountDailyFinancialValue) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "account_daily_financial_values"},
	}
}

func (AccountDailyFinancialValue) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		field.Time("business_date").SchemaType(map[string]string{dialect.Postgres: "date"}),
		field.Float("oauth_cost_cny").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Float("revenue_override_cny").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Time("revenue_override_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("revenue_evidence_cutoff_id").Optional().Nillable(),
		field.Int64("revenue_review_cutoff_id").Optional().Nillable(),
		field.Float("cost_override_cny").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,10)"}),
		field.Time("cost_override_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("cost_evidence_cutoff_id").Optional().Nillable(),
		field.Int64("cost_review_cutoff_id").Optional().Nillable(),
		field.Int64("updated_by"),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (AccountDailyFinancialValue) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("account", Account.Type).
			Field("account_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (AccountDailyFinancialValue) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "business_date").
			Unique().
			StorageKey("account_daily_financial_values_account_date_key"),
		index.Fields("account_id", "business_date").
			StorageKey("idx_account_daily_financial_values_account_id_business_date"),
	}
}
