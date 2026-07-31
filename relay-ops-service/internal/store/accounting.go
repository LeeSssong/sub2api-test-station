package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/accounting"
	"example.invalid/relay-ops-service/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

const cashEventAmountScale = 8

func decimalFromText(value string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parse numeric %q: %w", value, err)
	}
	return d, nil
}

func normalizeCashEventForStorage(input accounting.CashEventInput) accounting.CashEventInput {
	// Match NUMERIC(20,8) and pgx's microsecond timestamptz encoding before
	// both insertion and replay comparison.
	input.PaidAt = input.PaidAt.UTC().Truncate(time.Microsecond)
	input.AmountCNY = input.AmountCNY.Round(cashEventAmountScale)
	return input
}

func (s *Store) CreateCashEvent(ctx context.Context, actor domain.AdminActor, input accounting.CashEventInput, idempotencyKey string) (accounting.CashEvent, bool, error) {
	if actor.UserID <= 0 {
		return accounting.CashEvent{}, false, fmt.Errorf("created_by_user_id must be positive")
	}
	if strings.TrimSpace(idempotencyKey) == "" || len([]byte(idempotencyKey)) > 200 {
		return accounting.CashEvent{}, false, fmt.Errorf("idempotency_key is required and must be at most 200 bytes")
	}
	validated, err := accounting.ValidateCashEvent(normalizeCashEventForStorage(input))
	if err != nil {
		return accounting.CashEvent{}, false, err
	}
	var event accounting.CashEvent
	var amountText string
	var inserted bool
	var accountID *int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO relay_ops.accounting_cash_events
			(event_type, paid_at, amount_cny, source_kind, account_id, notes, idempotency_key, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (idempotency_key) DO UPDATE
			SET idempotency_key = EXCLUDED.idempotency_key
		RETURNING id, event_type, paid_at, amount_cny::text, source_kind, account_id, notes,
			idempotency_key, created_by_user_id, created_at, (xmax = 0) AS inserted`,
		validated.EventType, validated.PaidAt, validated.AmountCNY.StringFixed(cashEventAmountScale),
		string(validated.SourceKind), validated.AccountID, validated.Notes, idempotencyKey, actor.UserID,
	).Scan(&event.ID, &event.EventType, &event.PaidAt, &amountText, &event.SourceKind, &accountID,
		&event.Notes, &idempotencyKey, &event.CreatedByUserID, &event.CreatedAt, &inserted)
	if err != nil {
		return accounting.CashEvent{}, false, fmt.Errorf("create accounting cash event: %w", err)
	}
	event.AmountCNY, err = decimalFromText(amountText)
	if err != nil {
		return accounting.CashEvent{}, false, err
	}
	event.AccountID = accountID
	event.PaidAt = event.PaidAt.UTC()
	event.SourceKind = accounting.SourceKind(event.SourceKind)
	if !inserted {
		if err := compareCashEvent(event, validated); err != nil {
			return accounting.CashEvent{}, false, err
		}
	}
	return event, inserted, nil
}

func compareCashEvent(stored accounting.CashEvent, input accounting.CashEventInput) error {
	normalized := normalizeCashEventForStorage(input)
	if stored.EventType != normalized.EventType ||
		!stored.PaidAt.Equal(normalized.PaidAt) ||
		!stored.AmountCNY.Equal(normalized.AmountCNY) ||
		stored.SourceKind != normalized.SourceKind ||
		stored.Notes != normalized.Notes ||
		!sameInt64Ptr(stored.AccountID, normalized.AccountID) {
		return ErrConflict
	}
	return nil
}

func sameInt64Ptr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (s *Store) ReadCashEventTotals(ctx context.Context, window accounting.DayWindow) (accounting.CashEventTotals, error) {
	var outflowText, unlinkedText string
	var count int64
	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(amount_cny), 0)::text,
			COALESCE(SUM(amount_cny) FILTER (WHERE account_id IS NULL), 0)::text,
			COUNT(*)
		FROM relay_ops.accounting_cash_events
		WHERE paid_at >= $1 AND paid_at < $2`,
		window.Start.UTC(), window.End.UTC()).Scan(&outflowText, &unlinkedText, &count)
	if err != nil {
		return accounting.CashEventTotals{}, fmt.Errorf("read accounting cash totals: %w", err)
	}
	outflow, err := decimalFromText(outflowText)
	if err != nil {
		return accounting.CashEventTotals{}, err
	}
	unlinked, err := decimalFromText(unlinkedText)
	if err != nil {
		return accounting.CashEventTotals{}, err
	}
	return accounting.CashEventTotals{OutflowCNY: outflow, UnlinkedOutflowCNY: unlinked, EventCount: count}, nil
}

func (s *Store) ReadUsageTotals(ctx context.Context, window accounting.DayWindow, exclusions accounting.ExclusionPolicy) (accounting.UsageTotals, error) {
	userIDs := exclusions.InternalUserIDs
	if userIDs == nil {
		userIDs = []int64{}
	}
	apiKeyIDs := exclusions.InternalAPIKeyIDs
	if apiKeyIDs == nil {
		apiKeyIDs = []int64{}
	}
	var revenueText, customerText, internalText, oauthText, apiKeyText string
	var externalRequests, internalRequests int64
	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(ul.actual_cost) FILTER (
				WHERE u.role <> 'admin'
					AND NOT (ul.user_id = ANY($3::BIGINT[]))
					AND NOT (ul.api_key_id = ANY($4::BIGINT[]))
			), 0)::text,
			COUNT(*) FILTER (
				WHERE u.role <> 'admin'
					AND NOT (ul.user_id = ANY($3::BIGINT[]))
					AND NOT (ul.api_key_id = ANY($4::BIGINT[]))
			),
			COUNT(*) FILTER (
				WHERE u.role = 'admin'
					OR ul.user_id = ANY($3::BIGINT[])
					OR ul.api_key_id = ANY($4::BIGINT[])
			),
			COALESCE(SUM(
				COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)
			) FILTER (
				WHERE u.role <> 'admin'
					AND NOT (ul.user_id = ANY($3::BIGINT[]))
					AND NOT (ul.api_key_id = ANY($4::BIGINT[]))
			), 0)::text,
			COALESCE(SUM(
				COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)
			) FILTER (
				WHERE u.role = 'admin'
					OR ul.user_id = ANY($3::BIGINT[])
					OR ul.api_key_id = ANY($4::BIGINT[])
			), 0)::text,
			COALESCE(SUM(
				COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)
			) FILTER (WHERE a.type = 'oauth'), 0)::text,
			COALESCE(SUM(
				COALESCE(ul.account_stats_cost, ul.total_cost) * COALESCE(ul.account_rate_multiplier, 1)
			) FILTER (WHERE a.type = 'apikey'), 0)::text
		FROM public.usage_logs ul
		JOIN public.users u ON u.id = ul.user_id
		LEFT JOIN public.accounts a ON a.id = ul.account_id
		WHERE ul.created_at >= $1 AND ul.created_at < $2`,
		window.Start.UTC(), window.End.UTC(), userIDs, apiKeyIDs,
	).Scan(&revenueText, &externalRequests, &internalRequests, &customerText, &internalText, &oauthText, &apiKeyText)
	if err != nil {
		return accounting.UsageTotals{}, fmt.Errorf("read accounting usage totals: %w", err)
	}
	revenue, err := decimalFromText(revenueText)
	if err != nil {
		return accounting.UsageTotals{}, err
	}
	customer, err := decimalFromText(customerText)
	if err != nil {
		return accounting.UsageTotals{}, err
	}
	internal, err := decimalFromText(internalText)
	if err != nil {
		return accounting.UsageTotals{}, err
	}
	oauth, err := decimalFromText(oauthText)
	if err != nil {
		return accounting.UsageTotals{}, err
	}
	apiKey, err := decimalFromText(apiKeyText)
	if err != nil {
		return accounting.UsageTotals{}, err
	}
	return accounting.UsageTotals{
		ExternalRevenueCNY: revenue, ExternalRequests: externalRequests, InternalRequests: internalRequests,
		CustomerCostCNY: customer, InternalCostCNY: internal,
		OwnedOAuthCostCNY: oauth, UpstreamAPIKeyCostCNY: apiKey,
	}, nil
}

func (s *Store) UpsertDailySnapshot(ctx context.Context, snapshot accounting.DailySnapshot) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO relay_ops.accounting_daily_snapshots (
			report_date, external_revenue_cny, external_requests, internal_requests,
			customer_resource_cost_cny, internal_resource_cost_cny, resource_cost_cny,
			operating_gross_profit_cny, cash_outflow_cny, cash_net_result_cny,
			unlinked_cash_outflow_cny, cash_event_count, owned_oauth_cost_cny,
			upstream_apikey_cost_cny, computed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NOW())
		ON CONFLICT (report_date) DO UPDATE SET
			external_revenue_cny=EXCLUDED.external_revenue_cny,
			external_requests=EXCLUDED.external_requests,
			internal_requests=EXCLUDED.internal_requests,
			customer_resource_cost_cny=EXCLUDED.customer_resource_cost_cny,
			internal_resource_cost_cny=EXCLUDED.internal_resource_cost_cny,
			resource_cost_cny=EXCLUDED.resource_cost_cny,
			operating_gross_profit_cny=EXCLUDED.operating_gross_profit_cny,
			cash_outflow_cny=EXCLUDED.cash_outflow_cny,
			cash_net_result_cny=EXCLUDED.cash_net_result_cny,
			unlinked_cash_outflow_cny=EXCLUDED.unlinked_cash_outflow_cny,
			cash_event_count=EXCLUDED.cash_event_count,
			owned_oauth_cost_cny=EXCLUDED.owned_oauth_cost_cny,
			upstream_apikey_cost_cny=EXCLUDED.upstream_apikey_cost_cny,
			computed_at=NOW()`,
		accounting.LocalDay(snapshot.ReportDate).Format("2006-01-02"),
		snapshot.ExternalRevenueCNY.String(), snapshot.ExternalRequests, snapshot.InternalRequests,
		snapshot.CustomerResourceCostCNY.String(), snapshot.InternalResourceCostCNY.String(),
		snapshot.ResourceCostCNY.String(), snapshot.OperatingGrossProfitCNY.String(),
		snapshot.CashOutflowCNY.String(), snapshot.CashNetResultCNY.String(),
		snapshot.UnlinkedCashOutflowCNY.String(), snapshot.CashEventCount,
		snapshot.OwnedOAuthCostCNY.String(), snapshot.UpstreamAPIKeyCostCNY.String())
	if err != nil {
		return fmt.Errorf("upsert accounting daily snapshot: %w", err)
	}
	return nil
}

func (s *Store) ReadDailySnapshot(ctx context.Context, date time.Time) (accounting.DailySnapshot, bool, error) {
	var snapshot accounting.DailySnapshot
	var reportDate time.Time
	var values [10]string
	var eventCount int64
	err := s.pool.QueryRow(ctx, `
		SELECT report_date, external_revenue_cny::text, external_requests, internal_requests,
			customer_resource_cost_cny::text, internal_resource_cost_cny::text, resource_cost_cny::text,
			operating_gross_profit_cny::text, cash_outflow_cny::text, cash_net_result_cny::text,
			unlinked_cash_outflow_cny::text, cash_event_count, owned_oauth_cost_cny::text,
			upstream_apikey_cost_cny::text
		FROM relay_ops.accounting_daily_snapshots
		WHERE report_date=$1`,
		accounting.LocalDay(date).Format("2006-01-02")).Scan(
		&reportDate, &values[0], &snapshot.ExternalRequests, &snapshot.InternalRequests,
		&values[1], &values[2], &values[3], &values[4], &values[5], &values[6], &values[7],
		&eventCount, &values[8], &values[9])
	if errors.Is(err, pgx.ErrNoRows) {
		return accounting.DailySnapshot{}, false, nil
	}
	if err != nil {
		return accounting.DailySnapshot{}, false, fmt.Errorf("read accounting daily snapshot: %w", err)
	}
	parsed := make([]decimal.Decimal, len(values))
	for i, value := range values {
		parsed[i], err = decimalFromText(value)
		if err != nil {
			return accounting.DailySnapshot{}, false, err
		}
	}
	snapshot.ReportDate = accounting.LocalDay(reportDate)
	snapshot.ExternalRevenueCNY = parsed[0]
	snapshot.CustomerResourceCostCNY = parsed[1]
	snapshot.InternalResourceCostCNY = parsed[2]
	snapshot.ResourceCostCNY = parsed[3]
	snapshot.OperatingGrossProfitCNY = parsed[4]
	snapshot.CashOutflowCNY = parsed[5]
	snapshot.CashNetResultCNY = parsed[6]
	snapshot.UnlinkedCashOutflowCNY = parsed[7]
	snapshot.CashEventCount = eventCount
	snapshot.OwnedOAuthCostCNY = parsed[8]
	snapshot.UpstreamAPIKeyCostCNY = parsed[9]
	return snapshot, true, nil
}

func (s *Store) ListCashEvents(ctx context.Context, from, to time.Time, limit int) ([]accounting.CashEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, event_type, paid_at, amount_cny::text, source_kind, account_id, notes,
			created_by_user_id, created_at
		FROM relay_ops.accounting_cash_events
		WHERE paid_at >= $1 AND paid_at < $2
		ORDER BY paid_at DESC, id DESC
		LIMIT $3`, from.UTC(), to.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list accounting cash events: %w", err)
	}
	defer rows.Close()
	result := make([]accounting.CashEvent, 0)
	for rows.Next() {
		var event accounting.CashEvent
		var amountText string
		if err := rows.Scan(&event.ID, &event.EventType, &event.PaidAt, &amountText,
			&event.SourceKind, &event.AccountID, &event.Notes, &event.CreatedByUserID, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan accounting cash event: %w", err)
		}
		event.AmountCNY, err = decimalFromText(amountText)
		if err != nil {
			return nil, err
		}
		event.PaidAt = event.PaidAt.UTC()
		event.SourceKind = accounting.SourceKind(event.SourceKind)
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounting cash events: %w", err)
	}
	return result, nil
}
