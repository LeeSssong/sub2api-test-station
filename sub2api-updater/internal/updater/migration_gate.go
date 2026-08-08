package updater

import "strings"

func ClassifyMigration(sqlText string) MigrationClass {
	upper := strings.ToUpper(sqlText)
	if strings.Contains(upper, "DROP TABLE") || strings.Contains(upper, "TRUNCATE") || strings.Contains(upper, "DROP COLUMN") || strings.Contains(upper, "ALTER TABLE") && strings.Contains(upper, " DROP ") {
		return MigrationDestructive
	}
	if strings.Contains(upper, "ALTER TABLE") || strings.Contains(upper, "CREATE INDEX") || strings.Contains(upper, "CREATE TABLE") {
		return MigrationPotentiallyBreaking
	}
	return MigrationExpandOnly
}
func MigrationAllowed(class MigrationClass) bool { return class == MigrationExpandOnly }
