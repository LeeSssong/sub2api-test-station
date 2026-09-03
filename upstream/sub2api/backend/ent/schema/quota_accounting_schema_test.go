package schema

import (
	"reflect"
	"testing"

	"entgo.io/ent/dialect"
)

func TestQuotaAccountingGrantDecimalFieldsUseDecimalAndNumeric(t *testing.T) {
	fields := (UserQuotaGrant{}).Fields()
	decimalFields := 0
	for _, f := range fields {
		if f.Descriptor().Info.Type.String() == "other" && f.Descriptor().Info.RType != nil {
			if f.Descriptor().Info.RType.Kind != reflect.Struct || f.Descriptor().Info.Ident != "decimal.Decimal" {
				continue
			}
			decimalFields++
			if got := f.Descriptor().SchemaType[dialect.Postgres]; got != "numeric(20,8)" {
				t.Fatalf("field %s has postgres type %q", f.Descriptor().Name, got)
			}
		}
	}
	if decimalFields != 9 {
		t.Fatalf("expected nine quota grant decimal fields, got %d", decimalFields)
	}
	if got := (UserQuotaGrant{}).Annotations(); len(got) == 0 {
		t.Fatal("missing table annotation")
	}
}

func TestQuotaAccountingAdjustmentHasDecimalFields(t *testing.T) {
	decimalFields := 0
	for _, f := range (UserQuotaAdjustment{}).Fields() {
		if f.Descriptor().Info.Ident == "decimal.Decimal" {
			decimalFields++
			if f.Descriptor().SchemaType[dialect.Postgres] != "numeric(20,8)" {
				t.Fatalf("field %s is not numeric(20,8)", f.Descriptor().Name)
			}
		}
	}
	if decimalFields != 5 {
		t.Fatalf("expected five adjustment decimal fields, got %d", decimalFields)
	}
}
