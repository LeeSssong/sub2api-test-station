package schema

import (
	"fmt"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserQuotaLedgerEntry records an auditable change to a user's split quota.
type UserQuotaLedgerEntry struct {
	ent.Schema
}

func (UserQuotaLedgerEntry) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "user_quota_ledger_entries"}}
}

func (UserQuotaLedgerEntry) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("record_type").MaxLen(40).Validate(func(value string) error {
			switch value {
			case "recharge", "refund", "usage_consumption", "legacy_balance_adjustment", "payment_fulfillment", "redeem_credit", "affiliate_credit", "migration_projection":
				return nil
			default:
				return fmt.Errorf("unsupported quota ledger record type %q", value)
			}
		}),
		field.Float("cash_delta_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).Default(0),
		field.Float("paid_quota_delta_usd").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).Default(0),
		field.Float("gift_quota_delta_usd").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).Default(0),
		field.Float("cash_before_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).Default(0),
		field.Float("cash_after_cny").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).Default(0),
		field.Float("paid_before_usd").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).Default(0),
		field.Float("paid_after_usd").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).Default(0),
		field.Float("gift_before_usd").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).Default(0),
		field.Float("gift_after_usd").SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).Default(0),
		field.String("reference_type").MaxLen(64).Optional().Nillable(),
		field.String("reference_id").MaxLen(255).Optional().Nillable(),
		field.String("idempotency_key").MaxLen(255).Optional().Nillable(),
		field.Text("note").Default(""),
		field.Int64("operator_id").Optional().Nillable(),
		field.String("status").MaxLen(24).Default("confirmed").Validate(func(value string) error {
			if value != "confirmed" {
				return fmt.Errorf("unsupported quota ledger status %q", value)
			}
			return nil
		}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UserQuotaLedgerEntry) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("quota_ledger_entries").
			Field("user_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.From("operator", User.Type).
			Ref("operated_quota_ledger_entries").
			Field("operator_id").
			Unique(),
		edge.To("idempotency_record", QuotaIdempotencyRecord.Type).Unique(),
	}
}

func (UserQuotaLedgerEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at").StorageKey("idx_user_quota_ledger_entries_user_created_at"),
		index.Fields("reference_type", "reference_id").StorageKey("idx_user_quota_ledger_entries_reference"),
		index.Fields("user_id", "idempotency_key").Unique(),
	}
}
