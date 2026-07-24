// Package opsmetrics projects native Sub2API aggregates into safe site-runtime rows.
package opsmetrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"example.invalid/relay-ops-service/internal/accountquality"
	"example.invalid/relay-ops-service/internal/sub2api"
)

const (
	StatusOK                 = "ok"
	StatusSampleInsufficient = "sample_insufficient"
	StatusReadFailed         = "read_failed"

	ErrorCodeOpsSnapshotUnavailable = "ops_snapshot_unavailable"
	minimumRequestCount             = 20
)

var (
	ErrGroupsSourceUnavailable   = errors.New("ops metrics groups source unavailable")
	ErrAccountsSourceUnavailable = errors.New("ops metrics accounts source unavailable")
)

type Reader interface {
	ListGroups(context.Context) ([]sub2api.Group, error)
	ListAccounts(context.Context) ([]sub2api.Account, error)
	GetOpsSnapshot(context.Context, sub2api.OpsQuery) (sub2api.OpsSnapshot, error)
}

type GroupRuntime struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	RequestCount  int64   `json:"request_count"`
	SuccessCount  int64   `json:"success_count"`
	ErrorRate     float64 `json:"error_rate"`
	SLA           float64 `json:"sla"`
	TTFTP95MS     float64 `json:"ttft_p95_ms"`
	DurationP95MS float64 `json:"duration_p95_ms"`
	Status        string  `json:"status"`
	ErrorCode     string  `json:"error_code,omitempty"`
	EvidenceHash  string  `json:"evidence_hash"`
}

type AccountRuntime struct {
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	GroupIDs         []int64  `json:"group_ids"`
	PublicGroupNames []string `json:"public_group_names"`
	RequestCount     int64    `json:"request_count"`
	SuccessCount     int64    `json:"success_count"`
	ErrorRate        float64  `json:"error_rate"`
	SLA              float64  `json:"sla"`
	TTFTP95MS        float64  `json:"ttft_p95_ms"`
	DurationP95MS    float64  `json:"duration_p95_ms"`
	Status           string   `json:"status"`
	ErrorCode        string   `json:"error_code,omitempty"`
	EvidenceHash     string   `json:"evidence_hash"`
}

type Snapshot struct {
	CapturedAt       time.Time        `json:"captured_at"`
	AccountSetSHA256 string           `json:"account_set_sha256"`
	Groups           []GroupRuntime   `json:"groups"`
	Accounts         []AccountRuntime `json:"accounts"`
}

func Collect(ctx context.Context, reader Reader, now time.Time) (Snapshot, error) {
	if reader == nil {
		return Snapshot{}, fmt.Errorf("ops metrics reader is required")
	}
	groups, err := reader.ListGroups(ctx)
	if err != nil {
		return Snapshot{}, ErrGroupsSourceUnavailable
	}
	accounts, err := reader.ListAccounts(ctx)
	if err != nil {
		return Snapshot{}, ErrAccountsSourceUnavailable
	}

	snapshot := Snapshot{CapturedAt: now.UTC()}
	publicGroupNames := make(map[int64]string, len(groups))
	for _, group := range groups {
		if group.CustomerVisible() {
			publicGroupNames[group.ID] = group.Name
		}
	}
	for _, group := range groups {
		if !group.CustomerVisible() {
			continue
		}
		row := GroupRuntime{ID: group.ID, Name: group.Name}
		ops, readErr := reader.GetOpsSnapshot(ctx, sub2api.OpsQuery{TimeRange: "15m", GroupID: group.ID})
		if readErr != nil {
			row.Status = StatusReadFailed
			row.ErrorCode = ErrorCodeOpsSnapshotUnavailable
		} else {
			applyGroupMetrics(&row, ops)
		}
		row.EvidenceHash = evidenceHash("group", row.ID, row.Name, nil, nil, row.RequestCount, row.SuccessCount, row.ErrorRate, row.SLA, row.TTFTP95MS, row.DurationP95MS, row.Status, row.ErrorCode)
		snapshot.Groups = append(snapshot.Groups, row)
	}

	active := make([]sub2api.Account, 0, len(accounts))
	for _, account := range accounts {
		if account.Status == "active" && account.Schedulable {
			active = append(active, account)
		}
	}
	sort.SliceStable(active, func(i, j int) bool { return active[i].ID < active[j].ID })
	accountIDs := make([]int64, 0, len(active))
	for _, account := range active {
		accountIDs = append(accountIDs, account.ID)
	}
	snapshot.AccountSetSHA256, err = accountquality.CanonicalAccountSetSHA256(accountIDs)
	if err != nil {
		return Snapshot{}, ErrAccountsSourceUnavailable
	}
	for _, account := range active {
		groupIDs := append([]int64(nil), account.GroupIDs...)
		sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
		row := AccountRuntime{
			ID: account.ID, Name: account.Name, GroupIDs: groupIDs,
			PublicGroupNames: labelsForPublicGroups(groupIDs, publicGroupNames),
		}
		ops, readErr := reader.GetOpsSnapshot(ctx, sub2api.OpsQuery{TimeRange: "15m", AccountID: account.ID})
		if readErr != nil {
			row.Status = StatusReadFailed
			row.ErrorCode = ErrorCodeOpsSnapshotUnavailable
		} else {
			applyAccountMetrics(&row, ops)
		}
		row.EvidenceHash = evidenceHash("account", row.ID, row.Name, row.GroupIDs, row.PublicGroupNames, row.RequestCount, row.SuccessCount, row.ErrorRate, row.SLA, row.TTFTP95MS, row.DurationP95MS, row.Status, row.ErrorCode)
		snapshot.Accounts = append(snapshot.Accounts, row)
	}
	return snapshot, nil
}

func labelsForPublicGroups(groupIDs []int64, names map[int64]string) []string {
	labels := make([]string, 0, len(groupIDs))
	var previousID int64
	hasPreviousID := false
	for _, groupID := range groupIDs {
		if hasPreviousID && groupID == previousID {
			continue
		}
		previousID, hasPreviousID = groupID, true
		if name, ok := names[groupID]; ok {
			labels = append(labels, name)
		}
	}
	return labels
}

func (row GroupRuntime) ErrorRateDisplay() string {
	return errorRateDisplay(row.RequestCount, row.ErrorRate)
}

func (row GroupRuntime) SLADisplay() string { return slaDisplay(row.RequestCount, row.SLA) }

func (row AccountRuntime) ErrorRateDisplay() string {
	return errorRateDisplay(row.RequestCount, row.ErrorRate)
}

func (row AccountRuntime) SLADisplay() string { return slaDisplay(row.RequestCount, row.SLA) }

func (row GroupRuntime) TTFTP95Display() string {
	return ttftDisplay(row.SuccessCount, row.TTFTP95MS)
}

func (row AccountRuntime) TTFTP95Display() string {
	return ttftDisplay(row.SuccessCount, row.TTFTP95MS)
}

func (row GroupRuntime) DurationP95Display() string {
	return durationDisplay(row.RequestCount, row.DurationP95MS)
}

func (row AccountRuntime) DurationP95Display() string {
	return durationDisplay(row.RequestCount, row.DurationP95MS)
}

func errorRateDisplay(requestCount int64, value float64) string {
	if requestCount == 0 {
		return "未知"
	}
	return formatRatioPercent(value)
}

func slaDisplay(requestCount int64, value float64) string {
	if requestCount == 0 {
		return "未知"
	}
	return formatPercent(value)
}

func ttftDisplay(successCount int64, value float64) string {
	if successCount == 0 {
		return "无成功样本"
	}
	return fmt.Sprintf("%.0fms", value)
}

func durationDisplay(requestCount int64, value float64) string {
	if requestCount == 0 {
		return "未知"
	}
	return fmt.Sprintf("%.0fms", value)
}

func formatRatioPercent(value float64) string { return fmt.Sprintf("%.2f%%", value*100) }

func formatPercent(value float64) string { return fmt.Sprintf("%.2f%%", value) }

func applyGroupMetrics(row *GroupRuntime, snapshot sub2api.OpsSnapshot) {
	overview := snapshot.Overview
	row.RequestCount = overview.RequestCountTotal
	row.SuccessCount = overview.SuccessCount
	row.ErrorRate = overview.ErrorRate
	row.SLA = overview.SLA
	row.TTFTP95MS = overview.TTFT.P95MS
	row.DurationP95MS = overview.Duration.P95MS
	row.Status = statusFor(overview.RequestCountTotal)
}

func applyAccountMetrics(row *AccountRuntime, snapshot sub2api.OpsSnapshot) {
	overview := snapshot.Overview
	row.RequestCount = overview.RequestCountTotal
	row.SuccessCount = overview.SuccessCount
	row.ErrorRate = overview.ErrorRate
	row.SLA = overview.SLA
	row.TTFTP95MS = overview.TTFT.P95MS
	row.DurationP95MS = overview.Duration.P95MS
	row.Status = statusFor(overview.RequestCountTotal)
}

func statusFor(requestCount int64) string {
	if requestCount < minimumRequestCount {
		return StatusSampleInsufficient
	}
	return StatusOK
}

type evidence struct {
	Scope            string   `json:"scope"`
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	GroupIDs         []int64  `json:"group_ids,omitempty"`
	PublicGroupNames []string `json:"public_group_names,omitempty"`
	RequestCount     int64    `json:"request_count"`
	SuccessCount     int64    `json:"success_count"`
	ErrorRate        float64  `json:"error_rate"`
	SLA              float64  `json:"sla"`
	TTFTP95MS        float64  `json:"ttft_p95_ms"`
	DurationP95MS    float64  `json:"duration_p95_ms"`
	Status           string   `json:"status"`
	ErrorCode        string   `json:"error_code,omitempty"`
}

func evidenceHash(scope string, id int64, name string, groupIDs []int64, publicGroupNames []string, requestCount int64, successCount int64, errorRate, sla, ttftP95MS, durationP95MS float64, status, errorCode string) string {
	data, err := json.Marshal(evidence{
		Scope: scope, ID: id, Name: name, GroupIDs: groupIDs, PublicGroupNames: publicGroupNames, RequestCount: requestCount, SuccessCount: successCount,
		ErrorRate: errorRate, SLA: sla, TTFTP95MS: ttftP95MS, DurationP95MS: durationP95MS,
		Status: status, ErrorCode: errorCode,
	})
	if err != nil {
		panic(fmt.Sprintf("marshal ops evidence: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
