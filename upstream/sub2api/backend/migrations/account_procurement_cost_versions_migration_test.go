package migrations

import (
 "strings"
 "testing"
)
func TestAccountProcurementCostVersionsMigrationIsExpandOnly(t *testing.T) {
 b, err := FS.ReadFile("226_account_procurement_cost_versions.sql"); if err != nil { t.Fatal(err) }
 s := string(b)
 for _, token := range []string{"CREATE TABLE IF NOT EXISTS account_procurement_cost_versions", "UNIQUE INDEX", "ended_at", "loss_cny", "request_id"} { if !strings.Contains(s, token) { t.Fatalf("missing %s", token) } }
 for _, forbidden := range []string{"DROP TABLE", "UPDATE accounts", "INSERT INTO accounts"} { if strings.Contains(strings.ToUpper(s), forbidden) { t.Fatalf("forbidden %s", forbidden) } }
}
