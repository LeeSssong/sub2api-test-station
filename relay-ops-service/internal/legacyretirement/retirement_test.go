package legacyretirement

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRunDefaultsToCountOnly(t *testing.T) {
	t.Parallel()
	database := &fakeDatabase{counts: map[string]int64{
		"agent_analyses": 4,
		"incidents":      2,
	}}
	var output bytes.Buffer
	if err := Run(context.Background(), database, Options{}, &output); err != nil {
		t.Fatal(err)
	}
	if database.retired {
		t.Fatal("count-only run retired tables")
	}
	var report Report
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Mode != "count_only" || len(report.Tables) != len(Tables) {
		t.Fatalf("report=%#v", report)
	}
	if report.Tables[0] != (TableReport{Name: "agent_analyses", Exists: true, Rows: 4}) {
		t.Fatalf("first table=%#v", report.Tables[0])
	}
	if strings.Contains(output.String(), "https://") || strings.Contains(output.String(), "password") {
		t.Fatalf("count report exposed row data: %s", output.String())
	}
}

func TestRunExecuteRequiresExplicitConfirmationAndUsesAuthorizedOrder(t *testing.T) {
	t.Parallel()
	for _, confirmation := range []string{"", "yes", "DELETE"} {
		database := &fakeDatabase{}
		if err := Run(context.Background(), database, Options{Execute: true, Confirmation: confirmation}, &bytes.Buffer{}); err == nil {
			t.Fatalf("confirmation %q was accepted", confirmation)
		}
		if database.retired {
			t.Fatalf("confirmation %q changed database", confirmation)
		}
	}

	database := &fakeDatabase{}
	var output bytes.Buffer
	if err := Run(context.Background(), database, Options{Execute: true, Confirmation: ConfirmationPhrase}, &output); err != nil {
		t.Fatal(err)
	}
	if !database.retired || !reflect.DeepEqual(database.order, Tables) {
		t.Fatalf("retire order=%v", database.order)
	}
	var report Report
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Mode != "executed" {
		t.Fatalf("mode=%q", report.Mode)
	}
}

type fakeDatabase struct {
	counts  map[string]int64
	retired bool
	order   []string
}

func (database *fakeDatabase) Count(_ context.Context, table string) (bool, int64, error) {
	rows, exists := database.counts[table]
	return exists, rows, nil
}

func (database *fakeDatabase) Retire(_ context.Context, tables []string) error {
	database.retired = true
	database.order = append([]string(nil), tables...)
	return nil
}
