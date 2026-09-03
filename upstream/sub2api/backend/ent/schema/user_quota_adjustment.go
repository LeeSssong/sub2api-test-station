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

// UserQuotaAdjustment records refunds and administrative gift deductions.
type UserQuotaAdjustment struct{ ent.Schema }

func (UserQuotaAdjustment) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "user_quota_adjustments"}}
}

func (UserQuotaAdjustment) Fields() []ent.Field {
	json := func(name string) ent.Field {
		return field.JSON(name, []map[string]any{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"})
	}
	return []ent.Field{
		field.Int64("user_id"), field.String("adjustment_type").MaxLen(32), field.Int64("payment_order_id").Optional().Nillable(),
		json("reserved_allocations"), json("applied_allocations"), quotaDecimal("refund_amount"), field.String("refund_currency").MaxLen(8).Optional().Nillable(),
		field.String("refund_method").MaxLen(32).Optional().Nillable(), field.String("refund_trade_no").MaxLen(128).Optional().Nillable(),
		field.String("refund_provider_instance_id").MaxLen(128).Optional().Nillable(), field.String("provider_refund_id").MaxLen(128).Optional().Nillable(), field.String("provider_request_key").MaxLen(128).Optional().Nillable(),
		field.String("provider_state").MaxLen(20).Default("not_started"), field.JSON("provider_response_snapshot", map[string]any{}).Optional().SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("provider_error_code").MaxLen(64).Optional().Nillable(), field.String("provider_error_message").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int("attempt_count").Default(0), field.Time("last_attempt_at").Optional().Nillable(), field.Time("next_retry_at").Optional().Nillable(),
		field.String("reconciliation_note").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}), field.Int64("reconciled_by").Optional().Nillable(), field.Time("reconciled_at").Optional().Nillable(),
		quotaDecimal("requested_paid_quota_usd"), quotaDecimal("applied_paid_quota_usd"), quotaDecimal("applied_gift_quota_usd"), quotaDecimal("shortfall_paid_quota_usd"),
		field.Bool("force_refund").Default(false), field.String("approval_reason").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}), field.Int64("approved_by").Optional().Nillable(), field.Time("approved_at").Optional().Nillable(), field.String("financial_exception_ref").MaxLen(128).Optional().Nillable(), field.Int64("operator_user_id").Optional().Nillable(),
		field.String("actor_type").MaxLen(16), field.String("reason").SchemaType(map[string]string{dialect.Postgres: "text"}), field.String("status").MaxLen(16).Default("pending"), field.String("idempotency_key").MaxLen(128),
		field.Time("adjusted_at").Optional().Nillable(), field.Time("created_at").Immutable().Default(time.Now), field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (UserQuotaAdjustment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("quota_adjustments").Field("user_id").Unique().Required(),
		edge.From("payment_order", PaymentOrder.Type).Ref("quota_adjustments").Field("payment_order_id").Unique(),
	}
}

func (UserQuotaAdjustment) Indexes() []ent.Index {
	return []ent.Index{index.Fields("user_id", "created_at"), index.Fields("user_id", "idempotency_key").Unique(), index.Fields("refund_provider_instance_id", "provider_request_key").Unique(), index.Fields("refund_provider_instance_id", "provider_refund_id").Unique(), index.Fields("refund_method", "refund_trade_no").Unique()}
}
