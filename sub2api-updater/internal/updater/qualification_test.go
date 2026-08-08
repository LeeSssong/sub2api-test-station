package updater

import "testing"

func TestQualificationBlocksDestructiveAndFailedReports(t *testing.T) {
	base := QualificationReport{Tag: "v1", Commit: "abc", ContractVersion: 1, MigrationClass: MigrationExpandOnly, Passed: true}
	if err := Qualify(base); err != nil {
		t.Fatal(err)
	}
	base.MigrationClass = MigrationDestructive
	if err := Qualify(base); err == nil {
		t.Fatal("destructive report accepted")
	}
	base.MigrationClass = MigrationExpandOnly
	base.Passed = false
	if err := Qualify(base); err == nil {
		t.Fatal("failed report accepted")
	}
}
