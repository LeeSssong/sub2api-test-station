package nativeopssilence

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Scope struct {
	RuleID   int64
	Platform string
	GroupID  *int64
	Region   *string
}

type Reader interface {
	IsSilenced(context.Context, Scope, time.Time) (bool, error)
	Close()
}

type PostgresReader struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURLFile string) (*PostgresReader, error) {
	data, err := os.ReadFile(databaseURLFile)
	if err != nil {
		return nil, fmt.Errorf("read alert silence database URL: %w", err)
	}
	databaseURL := strings.TrimSpace(string(data))
	if databaseURL == "" {
		return nil, fmt.Errorf("alert silence database URL is empty")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse alert silence database URL: %w", err)
	}
	config.MaxConns = 2
	config.MinConns = 0
	config.MaxConnIdleTime = 2 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open alert silence database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping alert silence database: %w", err)
	}
	return &PostgresReader{pool: pool}, nil
}

func (r *PostgresReader) IsSilenced(ctx context.Context, scope Scope, now time.Time) (bool, error) {
	if r == nil || r.pool == nil {
		return false, fmt.Errorf("alert silence reader is not open")
	}
	if scope.RuleID <= 0 {
		return false, fmt.Errorf("alert silence rule ID must be positive")
	}
	if strings.TrimSpace(scope.Platform) == "" {
		return false, fmt.Errorf("alert silence platform is required")
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var matched bool
	err := r.pool.QueryRow(queryCtx, `
		SELECT EXISTS (
			SELECT 1
			FROM public.ops_alert_silences
			WHERE rule_id=$1
			  AND platform=$2
			  AND group_id IS NOT DISTINCT FROM $3
			  AND region IS NOT DISTINCT FROM $4
			  AND until>$5
		)`, scope.RuleID, scope.Platform, scope.GroupID, scope.Region, now).Scan(&matched)
	if err != nil {
		return false, fmt.Errorf("query alert silence state: %w", err)
	}
	return matched, nil
}

func (r *PostgresReader) Close() {
	if r != nil && r.pool != nil {
		r.pool.Close()
	}
}

var _ Reader = (*PostgresReader)(nil)
