package repository

import (
	"context"
	"database/sql"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOctober16PausedNotificationsMigration(t *testing.T) {
	dsn, base := os.Getenv("OCTOBER16_TEST_PG_DSN"), os.Getenv("OCTOBER16_TEST_BASE")
	if dsn == "" || base == "" {
		t.Skip("requires disposable local database and baseline")
	}
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost", "::1"}, u.Hostname())
	require.True(t, strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "sub2api_oct16_"))
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var n int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname='public'").Scan(&n))
	require.Zero(t, n)
	require.NoError(t, applyMigrationsFS(ctx, db, os.DirFS(base)))
	_, err = db.ExecContext(ctx, "CREATE TABLE oct16_receipts AS SELECT * FROM schema_migrations; INSERT INTO accounts(id,name,platform,type,credentials) VALUES(900001,'fixture','openai','oauth','{}'); INSERT INTO account_ops_alerts(account_id,kind,account_name,signal,http_status,state) VALUES(900001,'balance_low','fixture','fixture',0,'sent');")
	require.NoError(t, err)
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "LOCK TABLE groups IN ACCESS SHARE MODE")
	require.NoError(t, err)
	started := time.Now()
	err = ApplyMigrations(ctx, db)
	require.ErrorContains(t, err, "lock timeout")
	require.Less(t, time.Since(started), 2*time.Second)
	require.NoError(t, tx.Rollback())
	require.NoError(t, ApplyMigrations(ctx, db))
	require.NoError(t, VerifyMigrations(ctx, db))
	require.NoError(t, verifyMigrationsFS(ctx, db, os.DirFS(base)))
	require.NoError(t, ApplyMigrations(ctx, db))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM oct16_receipts o LEFT JOIN schema_migrations s USING(filename) WHERE s.checksum IS DISTINCT FROM o.checksum").Scan(&n))
	require.Zero(t, n)
	var group, state string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT group_id,state FROM account_ops_alerts WHERE account_id=900001").Scan(&group, &state))
	require.Equal(t, "account:900001", group)
	require.Equal(t, "sent", state)
	_, err = db.ExecContext(ctx, "INSERT INTO account_ops_alerts(account_id,kind,account_name,signal,http_status) VALUES(900001,'balance_low','fixture','fixture',0) ON CONFLICT(account_id,kind) DO UPDATE SET last_seen=NOW()")
	require.ErrorContains(t, err, "no unique or exclusion constraint")
	_, err = db.ExecContext(ctx, "INSERT INTO account_ops_alerts(account_id,group_id,kind,account_name,signal,http_status) VALUES(900001,'account:900001','balance_low','fixture','fixture',0) ON CONFLICT(group_id,kind) DO UPDATE SET last_seen=NOW()")
	require.NoError(t, err)
}
