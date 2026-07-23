package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"example.invalid/relay-ops-service/internal/qualityreports"
	"github.com/jackc/pgx/v5"
)

//go:embed migrations/003_quality_reports.sql
var qualityReportsMigration string

func init() {
	initialMigration += "\n" + qualityReportsMigration
}

func (s *Store) PutQualityReport(ctx context.Context, report qualityreports.Report) error {
	if report.ReportID == "" || len(report.ReportHash) != 64 || report.UpstreamID <= 0 || report.UpstreamName == "" ||
		report.JobKind == "" || report.Status == "" || !json.Valid(report.Record) || report.RecordedAt.IsZero() || report.ExpiresAt.IsZero() {
		return fmt.Errorf("quality report is incomplete")
	}
	command, err := s.pool.Exec(ctx, `
		INSERT INTO relay_ops.quality_reports
			(report_id, report_hash, upstream_id, upstream_name, job_kind, status, quality_score, total_score,
			 direct_summary, gateway_summary, models_summary, pricing_summary, capacity_summary, record, recorded_at, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (report_id) DO NOTHING`,
		report.ReportID, report.ReportHash, report.UpstreamID, report.UpstreamName, report.JobKind, report.Status,
		report.QualityScore, report.TotalScore, report.Direct, report.Gateway, report.Models, report.Pricing,
		report.Capacity, report.Record, report.RecordedAt.UTC(), report.ExpiresAt.UTC())
	if err != nil {
		return fmt.Errorf("append quality report: %w", err)
	}
	if command.RowsAffected() == 1 {
		return nil
	}
	var existingHash string
	if err := s.pool.QueryRow(ctx, `SELECT report_hash FROM relay_ops.quality_reports WHERE report_id=$1`, report.ReportID).Scan(&existingHash); err != nil {
		return fmt.Errorf("read existing quality report: %w", err)
	}
	if existingHash != report.ReportHash {
		return ErrConflict
	}
	return nil
}

func (s *Store) GetQualityReport(ctx context.Context, reportID string) (qualityreports.Report, bool, error) {
	var report qualityreports.Report
	report.ReportID = strings.TrimSpace(reportID)
	if report.ReportID == "" {
		return qualityreports.Report{}, false, nil
	}
	err := s.pool.QueryRow(ctx, `
		SELECT report_hash, upstream_id, upstream_name, job_kind, status, quality_score, total_score,
		       direct_summary, gateway_summary, models_summary, pricing_summary, capacity_summary,
		       record, recorded_at, expires_at
		FROM relay_ops.quality_reports WHERE report_id=$1`, report.ReportID).Scan(
		&report.ReportHash, &report.UpstreamID, &report.UpstreamName, &report.JobKind, &report.Status,
		&report.QualityScore, &report.TotalScore, &report.Direct, &report.Gateway, &report.Models,
		&report.Pricing, &report.Capacity, &report.Record, &report.RecordedAt, &report.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return qualityreports.Report{}, false, nil
	}
	if err != nil {
		return qualityreports.Report{}, false, fmt.Errorf("get quality report: %w", err)
	}
	return report, true, nil
}

func (s *Store) ListQualityReports(ctx context.Context, limit int) ([]qualityreports.Report, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT report_id, report_hash, upstream_id, upstream_name, job_kind, status, quality_score, total_score,
		       direct_summary, gateway_summary, models_summary, pricing_summary, capacity_summary,
		       record, recorded_at, expires_at
		FROM relay_ops.quality_reports ORDER BY recorded_at DESC, report_id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list quality reports: %w", err)
	}
	defer rows.Close()
	items := make([]qualityreports.Report, 0)
	for rows.Next() {
		var report qualityreports.Report
		if err := rows.Scan(
			&report.ReportID, &report.ReportHash, &report.UpstreamID, &report.UpstreamName, &report.JobKind,
			&report.Status, &report.QualityScore, &report.TotalScore, &report.Direct, &report.Gateway,
			&report.Models, &report.Pricing, &report.Capacity, &report.Record, &report.RecordedAt, &report.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan quality report: %w", err)
		}
		items = append(items, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quality reports: %w", err)
	}
	return items, nil
}
