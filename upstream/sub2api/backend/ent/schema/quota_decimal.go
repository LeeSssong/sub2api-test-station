package schema

import (
	"github.com/shopspring/decimal"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
)

func quotaDecimal(name string) ent.Field {
	return field.Other(name, decimal.Decimal{}).
		SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)", dialect.SQLite: "decimal"}).
		Default(decimal.Zero)
}
