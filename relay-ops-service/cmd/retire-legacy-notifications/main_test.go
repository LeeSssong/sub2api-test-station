package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"example.invalid/relay-ops-service/internal/legacyretirement"
)

func TestRunCommandRequiresBothExecuteFlagAndConfirmation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name         string
		args         []string
		confirmation string
		wantRetire   bool
		wantError    bool
	}{
		{name: "default count only"},
		{name: "confirmation without flag", confirmation: legacyretirement.ConfirmationPhrase},
		{name: "flag without confirmation", args: []string{"--execute"}, wantError: true},
		{name: "both gates", args: []string{"--execute"}, confirmation: legacyretirement.ConfirmationPhrase, wantRetire: true},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			database := &fakeCommandDatabase{}
			var output bytes.Buffer
			err := run(context.Background(), test.args, func(key string) string {
				if key == confirmationEnvironment {
					return test.confirmation
				}
				return ""
			}, &output, func(context.Context, string) (retirementDatabase, error) {
				return database, nil
			})
			if (err != nil) != test.wantError {
				t.Fatalf("error=%v", err)
			}
			if database.retired != test.wantRetire {
				t.Fatalf("retired=%v", database.retired)
			}
			if strings.Contains(output.String(), "database-url") {
				t.Fatalf("output exposed protected path: %q", output.String())
			}
		})
	}
}

type fakeCommandDatabase struct {
	retired bool
}

func (*fakeCommandDatabase) Count(context.Context, string) (bool, int64, error) {
	return false, 0, nil
}

func (database *fakeCommandDatabase) Retire(context.Context, []string) error {
	database.retired = true
	return nil
}

func (*fakeCommandDatabase) Close() {}
