package legacyretirement

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresDatabase struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURLFile string) (*PostgresDatabase, error) {
	data, err := os.ReadFile(databaseURLFile)
	if err != nil {
		return nil, errors.New("read retirement database configuration")
	}
	databaseURL := strings.TrimSpace(string(data))
	for index := range data {
		data[index] = 0
	}
	if databaseURL == "" {
		return nil, errors.New("retirement database configuration is empty")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("parse retirement database configuration")
	}
	config.MaxConns = 1
	config.MinConns = 0
	config.MaxConnIdleTime = time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("open retirement database")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("ping retirement database")
	}
	return &PostgresDatabase{pool: pool}, nil
}

func (database *PostgresDatabase) Close() {
	if database != nil && database.pool != nil {
		database.pool.Close()
	}
}

func (database *PostgresDatabase) Count(ctx context.Context, table string) (bool, int64, error) {
	if database == nil || database.pool == nil || !authorizedTable(table) {
		return false, 0, errors.New("invalid retirement table")
	}
	var relation string
	if err := database.pool.QueryRow(ctx, `SELECT COALESCE(to_regclass($1)::text, '')`, "relay_ops."+table).Scan(&relation); err != nil {
		return false, 0, errors.New("inspect retirement table")
	}
	if relation == "" {
		return false, 0, nil
	}
	query := "SELECT COUNT(*) FROM " + pgx.Identifier{"relay_ops", table}.Sanitize()
	var rows int64
	if err := database.pool.QueryRow(ctx, query).Scan(&rows); err != nil {
		return false, 0, errors.New("count retirement table")
	}
	return true, rows, nil
}

func (database *PostgresDatabase) Retire(ctx context.Context, tables []string) error {
	if database == nil || database.pool == nil || !authorizedOrder(tables) {
		return errors.New("invalid retirement order")
	}
	transaction, err := database.pool.Begin(ctx)
	if err != nil {
		return errors.New("begin retirement transaction")
	}
	defer transaction.Rollback(ctx)
	for _, table := range tables {
		statement := "DROP TABLE IF EXISTS " + pgx.Identifier{"relay_ops", table}.Sanitize()
		if _, err := transaction.Exec(ctx, statement); err != nil {
			return fmt.Errorf("drop authorized retirement table: %w", err)
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return errors.New("commit retirement transaction")
	}
	return nil
}

func authorizedTable(table string) bool {
	for _, allowed := range Tables {
		if table == allowed {
			return true
		}
	}
	return false
}
