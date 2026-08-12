package repository

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsErrorListSelectsCanonicalBusinessLimitFlag(t *testing.T) {
	src, err := os.ReadFile("ops_repo.go")
	require.NoError(t, err)
	text := string(src)
	listStart := strings.Index(text, "func (r *opsRepository) ListErrorLogs")
	detailStart := strings.Index(text, "func (r *opsRepository) GetErrorLogByID")
	require.Greater(t, listStart, -1)
	require.Greater(t, detailStart, listStart)
	listSource := text[listStart:detailStart]
	require.Contains(t, listSource, "e.is_business_limited")
	require.Contains(t, listSource, "&item.IsBusinessLimited")
}

func TestOpsErrorRepositoryListAndDetailPreserveOverloadClassificationEvidence(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	createdAt := time.Date(2026, 8, 12, 3, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ops_error_logs`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT.*e\.upstream_error_message.*e\.upstream_error_detail.*FROM ops_error_logs e`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "error_phase", "error_type", "error_owner", "error_source", "severity",
			"is_business_limited", "status_code", "platform", "model", "resolved", "resolved_at",
			"resolved_by_user_id", "resolved_by_name", "client_request_id", "request_id", "error_message",
			"upstream_error_message", "upstream_error_detail", "user_id", "user_email", "api_key_id",
			"account_id", "account_name", "group_id", "group_name", "client_ip", "request_path", "stream",
			"inbound_endpoint", "upstream_endpoint", "requested_model", "upstream_model", "user_agent",
			"request_type", "api_key_name", "api_key_deleted_at",
		}).AddRow(
			int64(44), createdAt, "upstream", "upstream_error", "provider", "upstream_http", "P1",
			false, 503, "openai", "gpt-5", false, nil,
			nil, "", "client-44", "request-44", "upstream request failed",
			"provider overloaded due to capacity", `{"message":"capacity exhausted"}`, nil, "", nil,
			int64(81), "plus-Auv", int64(6), "GPT-Plus", nil, "/v1/responses", true,
			"/v1/responses", "https://provider.test/v1/responses", "gpt-5", "gpt-5", "codex", nil, "key", nil,
		))

	repo := NewOpsRepository(db).(*opsRepository)
	list, err := repo.ListErrorLogs(context.Background(), &service.OpsErrorLogFilter{Page: 1, PageSize: 20, View: "all"})
	require.NoError(t, err)
	require.Len(t, list.Errors, 1)
	listDiagnosis := service.ProjectNativeErrorDiagnosis(&service.OpsErrorLogDetail{OpsErrorLog: *list.Errors[0]})
	require.NotNil(t, listDiagnosis)
	require.Equal(t, service.NativeErrorClassUpstreamOverloaded, listDiagnosis.Class)

	mock.ExpectQuery(`(?s)SELECT.*e\.upstream_error_message.*e\.upstream_error_detail.*WHERE e\.id = \$1`).
		WithArgs(int64(44)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "error_phase", "error_type", "error_owner", "error_source", "severity", "status_code",
			"platform", "model", "resolved", "resolved_at", "resolved_by_user_id", "client_request_id", "request_id",
			"error_message", "error_body", "upstream_status_code", "upstream_error_message", "upstream_error_detail",
			"upstream_errors", "is_business_limited", "user_id", "user_email", "api_key_id", "account_id", "account_name",
			"group_id", "group_name", "client_ip", "request_path", "stream", "inbound_endpoint", "upstream_endpoint",
			"requested_model", "upstream_model", "request_type", "user_agent", "auth_latency_ms", "routing_latency_ms",
			"upstream_latency_ms", "response_latency_ms", "time_to_first_token_ms", "api_key_prefix", "api_key_name",
			"api_key_deleted_at",
		}).AddRow(
			int64(44), createdAt, "upstream", "upstream_error", "provider", "upstream_http", "P1", 503,
			"openai", "gpt-5", false, nil, nil, "client-44", "request-44",
			"upstream request failed", "", 503, "provider overloaded due to capacity", `{"message":"capacity exhausted"}`,
			"", false, nil, "", nil, int64(81), "plus-Auv", int64(6), "GPT-Plus", nil, "/v1/responses", true,
			"/v1/responses", "https://provider.test/v1/responses", "gpt-5", "gpt-5", nil, "codex", nil, nil, nil, nil, nil,
			"sk-safe", "key", nil,
		))

	detail, err := repo.GetErrorLogByID(context.Background(), 44)
	require.NoError(t, err)
	detailDiagnosis := service.ProjectNativeErrorDiagnosis(detail)
	require.NotNil(t, detailDiagnosis)
	require.Equal(t, listDiagnosis.Class, detailDiagnosis.Class)
	require.Equal(t, service.NativeErrorClassUpstreamOverloaded, detailDiagnosis.Class)
	require.NoError(t, mock.ExpectationsWereMet())
}
