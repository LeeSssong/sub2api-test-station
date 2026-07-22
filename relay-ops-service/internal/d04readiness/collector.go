package d04readiness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"example.invalid/relay-ops-service/internal/sub2api"
)

const AccountAttributedNaturalTraffic = "sub2api_account_attributed_natural_traffic"

type AccountLister interface {
	ListAccounts(context.Context) ([]sub2api.Account, error)
}

type Collector struct {
	Accounts AccountLister
	Clock    func() time.Time
}

type Inputs struct {
	SnapshotID      string
	BalanceEvidence []BalanceEvidence
	QualityEvidence []QualityEvidence
}

type BalanceEvidence struct {
	AccountID   int64     `json:"account_id"`
	BalanceUSD  *float64  `json:"balance_usd"`
	RecordedAt  time.Time `json:"recorded_at"`
	EvidenceRef string    `json:"evidence_ref,omitempty"`
}

type QualityEvidence struct {
	AccountID         int64     `json:"account_id"`
	Source            string    `json:"source"`
	RecordedAt        time.Time `json:"recorded_at"`
	SampleCount       int64     `json:"sample_count"`
	SuccessRate       float64   `json:"success_rate"`
	ErrorRate         float64   `json:"error_rate"`
	TTFTP95MS         float64   `json:"ttft_p95_ms"`
	TotalLatencyP95MS float64   `json:"total_latency_p95_ms"`
}

type Snapshot struct {
	SchemaVersion     int               `json:"schema_version"`
	SnapshotID        string            `json:"snapshot_id"`
	CapturedAt        time.Time         `json:"captured_at"`
	UpstreamDiscovery UpstreamDiscovery `json:"upstream_discovery"`
	ActiveUpstreams   []ActiveUpstream  `json:"active_upstreams"`
}

type UpstreamDiscovery struct {
	Source           string    `json:"source"`
	RecordedAt       time.Time `json:"recorded_at"`
	AccountSetSHA256 string    `json:"account_set_sha256"`
}

type ActiveUpstream struct {
	AccountID           int64      `json:"account_id"`
	DisplayName         string     `json:"display_name"`
	Platform            string     `json:"platform"`
	Status              string     `json:"status"`
	Schedulable         bool       `json:"schedulable"`
	GroupIDs            []int64    `json:"group_ids"`
	RuntimeAvailable    bool       `json:"runtime_available"`
	RuntimeBlockReason  string     `json:"runtime_block_reason,omitempty"`
	BalanceUSD          *float64   `json:"balance_usd"`
	FinancialRecordedAt *time.Time `json:"financial_recorded_at"`
	QualitySource       string     `json:"quality_source"`
	QualityRecordedAt   *time.Time `json:"quality_recorded_at"`
	SampleCount         *int64     `json:"sample_count"`
	SuccessRate         *float64   `json:"success_rate"`
	ErrorRate           *float64   `json:"error_rate"`
	TTFTP95MS           *float64   `json:"ttft_p95_ms"`
	TotalLatencyP95MS   *float64   `json:"total_latency_p95_ms"`
}

func (c Collector) Collect(ctx context.Context, input Inputs) (Snapshot, error) {
	if c.Accounts == nil {
		return Snapshot{}, fmt.Errorf("account reader is required")
	}
	if strings.TrimSpace(input.SnapshotID) == "" {
		return Snapshot{}, fmt.Errorf("snapshot ID is required")
	}
	now := time.Now().UTC()
	if c.Clock != nil {
		now = c.Clock().UTC()
	}
	balances, err := indexBalances(input.BalanceEvidence, now)
	if err != nil {
		return Snapshot{}, err
	}
	qualities, err := indexQualities(input.QualityEvidence, now)
	if err != nil {
		return Snapshot{}, err
	}
	accounts, err := c.Accounts.ListAccounts(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list Sub2API accounts: %w", err)
	}
	seen := make(map[int64]struct{}, len(accounts))
	active := make([]sub2api.Account, 0, len(accounts))
	for _, account := range accounts {
		if account.ID <= 0 || strings.TrimSpace(account.Status) == "" {
			return Snapshot{}, fmt.Errorf("Sub2API account metadata is malformed")
		}
		if _, exists := seen[account.ID]; exists {
			return Snapshot{}, fmt.Errorf("duplicate Sub2API account ID %d", account.ID)
		}
		seen[account.ID] = struct{}{}
		if account.Status == "active" && account.Schedulable {
			active = append(active, account)
		}
	}
	if len(active) == 0 {
		return Snapshot{}, fmt.Errorf("no active schedulable Sub2API accounts")
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })

	canonical := make([]canonicalAccount, 0, len(active))
	upstreams := make([]ActiveUpstream, 0, len(active))
	for _, account := range active {
		groups := append([]int64(nil), account.GroupIDs...)
		sort.Slice(groups, func(i, j int) bool { return groups[i] < groups[j] })
		available, blockReason := runtimeAvailability(account, now)
		upstream := ActiveUpstream{
			AccountID: account.ID, DisplayName: account.Name, Platform: account.Platform,
			Status: account.Status, Schedulable: account.Schedulable, GroupIDs: groups,
			RuntimeAvailable: available, RuntimeBlockReason: blockReason,
		}
		if evidence, ok := balances[account.ID]; ok {
			upstream.BalanceUSD = evidence.BalanceUSD
			recordedAt := evidence.RecordedAt.UTC()
			upstream.FinancialRecordedAt = &recordedAt
		}
		if evidence, ok := qualities[account.ID]; ok {
			recordedAt := evidence.RecordedAt.UTC()
			sampleCount := evidence.SampleCount
			successRate := evidence.SuccessRate
			errorRate := evidence.ErrorRate
			ttft := evidence.TTFTP95MS
			total := evidence.TotalLatencyP95MS
			upstream.QualitySource = evidence.Source
			upstream.QualityRecordedAt = &recordedAt
			upstream.SampleCount = &sampleCount
			upstream.SuccessRate = &successRate
			upstream.ErrorRate = &errorRate
			upstream.TTFTP95MS = &ttft
			upstream.TotalLatencyP95MS = &total
		}
		upstreams = append(upstreams, upstream)
		canonical = append(canonical, canonicalAccount{
			AccountID: account.ID, Status: account.Status, Schedulable: account.Schedulable,
			GroupIDs: groups,
		})
	}
	hash, err := canonicalHash(canonical)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		SchemaVersion: 3, SnapshotID: input.SnapshotID, CapturedAt: now,
		UpstreamDiscovery: UpstreamDiscovery{
			Source: "sub2api_admin_accounts", RecordedAt: now, AccountSetSHA256: hash,
		},
		ActiveUpstreams: upstreams,
	}, nil
}

type canonicalAccount struct {
	AccountID   int64   `json:"account_id"`
	Status      string  `json:"status"`
	Schedulable bool    `json:"schedulable"`
	GroupIDs    []int64 `json:"group_ids"`
}

func canonicalHash(accounts []canonicalAccount) (string, error) {
	data, err := json.Marshal(accounts)
	if err != nil {
		return "", fmt.Errorf("encode canonical account set: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func indexBalances(records []BalanceEvidence, now time.Time) (map[int64]BalanceEvidence, error) {
	indexed := make(map[int64]BalanceEvidence, len(records))
	for _, record := range records {
		if record.AccountID <= 0 || record.RecordedAt.IsZero() || record.RecordedAt.After(now) {
			return nil, fmt.Errorf("balance evidence is malformed")
		}
		if record.BalanceUSD != nil && (math.IsNaN(*record.BalanceUSD) || math.IsInf(*record.BalanceUSD, 0)) {
			return nil, fmt.Errorf("balance evidence is malformed")
		}
		if _, duplicate := indexed[record.AccountID]; duplicate {
			return nil, fmt.Errorf("duplicate balance evidence for account %d", record.AccountID)
		}
		indexed[record.AccountID] = record
	}
	return indexed, nil
}

func indexQualities(records []QualityEvidence, now time.Time) (map[int64]QualityEvidence, error) {
	indexed := make(map[int64]QualityEvidence, len(records))
	for _, record := range records {
		if record.AccountID <= 0 || record.Source != AccountAttributedNaturalTraffic ||
			record.RecordedAt.IsZero() || record.RecordedAt.After(now) || record.SampleCount < 0 ||
			record.SuccessRate < 0 || record.SuccessRate > 1 || record.ErrorRate < 0 || record.ErrorRate > 1 ||
			record.TTFTP95MS < 0 || record.TotalLatencyP95MS < 0 {
			return nil, fmt.Errorf("quality evidence is malformed")
		}
		if _, duplicate := indexed[record.AccountID]; duplicate {
			return nil, fmt.Errorf("duplicate quality evidence for account %d", record.AccountID)
		}
		indexed[record.AccountID] = record
	}
	return indexed, nil
}

func runtimeAvailability(account sub2api.Account, now time.Time) (bool, string) {
	switch {
	case account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(now):
		return false, "temporary_unschedulable"
	case account.OverloadUntil != nil && account.OverloadUntil.After(now):
		return false, "overloaded"
	case account.RateLimitResetAt != nil && account.RateLimitResetAt.After(now):
		return false, "rate_limited"
	case account.AutoPauseOnExpired && account.ExpiresAt != nil && *account.ExpiresAt <= now.Unix():
		return false, "expired"
	default:
		return true, ""
	}
}
