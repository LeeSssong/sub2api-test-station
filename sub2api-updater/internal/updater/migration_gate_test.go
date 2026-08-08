package updater

import "testing"

func TestMigrationClassification(t *testing.T) {
	if ClassifyMigration("CREATE TABLE x (id int)") != MigrationPotentiallyBreaking {
		t.Fatal("create not classified")
	}
	if ClassifyMigration("ALTER TABLE x ADD COLUMN y text") != MigrationPotentiallyBreaking {
		t.Fatal("alter not classified")
	}
	if ClassifyMigration("ALTER TABLE x DROP COLUMN y") != MigrationDestructive {
		t.Fatal("drop not blocked")
	}
	if !MigrationAllowed(MigrationExpandOnly) {
		t.Fatal("expand-only should be allowed")
	}
}
