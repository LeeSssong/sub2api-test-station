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

// UserQuotaGrant records one positive quota issuance. Monetary values use the
// numeric database type; service code must adapt generated float fields through
// the decimal boundary contract before doing accounting arithmetic.
type UserQuotaGrant struct{ ent.Schema }

func (UserQuotaGrant) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "user_quota_grants"}}
}

func (UserQuotaGrant) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"), field.String("grant_type").MaxLen(32),
		field.Int64("payment_order_id").Optional().Nillable(), field.Int64("redeem_code_id").Optional().Nillable(),
		field.Int64("promo_code_usage_id").Optional().Nillable(), field.Int64("affiliate_ledger_id").Optional().Nillable(),
		quotaDecimal("paid_quota_usd"), quotaDecimal("gift_quota_usd"), quotaDecimal("total_quota_usd"),
		quotaDecimal("consumed_paid_quota_usd"), quotaDecimal("consumed_gift_quota_usd"), quotaDecimal("refunded_paid_quota_usd"),
		quotaDecimal("deducted_gift_quota_usd"), quotaDecimal("reserved_paid_quota_usd"), quotaDecimal("legacy_debt_offset_paid_quota_usd"),
		field.Int64("operator_user_id").Optional().Nillable(), field.String("idempotency_key").MaxLen(128).Optional().Nillable(),
		field.JSON("rule_snapshot", map[string]any{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("note").SchemaType(map[string]string{dialect.Postgres: "text"}).Default(""),
		field.Time("granted_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UserQuotaGrant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("quota_grants").Field("user_id").Unique().Required(),
		edge.From("payment_order", PaymentOrder.Type).Ref("quota_grants").Field("payment_order_id").Unique(),
		edge.From("redeem_code", RedeemCode.Type).Ref("quota_grants").Field("redeem_code_id").Unique(),
		edge.From("promo_code_usage", PromoCodeUsage.Type).Ref("quota_grants").Field("promo_code_usage_id").Unique(),
	}
}

func (UserQuotaGrant) Indexes() []ent.Index {
	return []ent.Index{index.Fields("user_id", "granted_at", "id"), index.Fields("payment_order_id").Unique(), index.Fields("redeem_code_id").Unique(), index.Fields("promo_code_usage_id").Unique(), index.Fields("affiliate_ledger_id").Unique(), index.Fields("user_id", "idempotency_key").Unique()}
}
