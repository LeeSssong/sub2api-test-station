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

// UserWallet is the native split-balance source of truth for a user.
type UserWallet struct {
	ent.Schema
}

func (UserWallet) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "user_wallets"}}
}

func (UserWallet) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Float("cash_balance_cny").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("paid_quota_balance_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("gift_quota_balance_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Int64("version").Default(1),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UserWallet) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("wallet").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (UserWallet) Indexes() []ent.Index {
	return []ent.Index{index.Fields("user_id").Unique()}
}
