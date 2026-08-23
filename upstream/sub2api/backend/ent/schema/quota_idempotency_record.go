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

// QuotaIdempotencyRecord stores the durable replay result for quota mutations.
type QuotaIdempotencyRecord struct {
	ent.Schema
}

func (QuotaIdempotencyRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "quota_idempotency_records"}}
}

func (QuotaIdempotencyRecord) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("idempotency_key").MaxLen(255),
		field.String("request_fingerprint").MaxLen(64),
		field.String("status").MaxLen(24).Default("processing"),
		field.Int64("ledger_entry_id").Optional().Nillable(),
		field.Int("response_status").Optional().Nillable(),
		field.Text("response_body").Optional().Nillable(),
		field.Time("expires_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (QuotaIdempotencyRecord) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("quota_idempotency_records").
			Field("user_id").
			Unique().
			Required(),
		edge.From("ledger_entry", UserQuotaLedgerEntry.Type).
			Ref("idempotency_record").
			Field("ledger_entry_id").
			Unique(),
	}
}

func (QuotaIdempotencyRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "idempotency_key").Unique(),
		index.Fields("expires_at"),
	}
}
