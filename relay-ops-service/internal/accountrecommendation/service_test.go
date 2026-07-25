package accountrecommendation

import (
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestAnalyzeRecommendsHigherScoringCompatibleAccount(t *testing.T) {
	t.Parallel()

	result := Analyze(projection(
		account(11, "账号 A", "gpt-5.6-sol", 0.70, 450, 1_600, 0.12, 100, 8, 4),
		account(12, "账号 B", "gpt-5.6-sol", 0.96, 120, 500, 0.08, 40, 2, 0),
	))
	if len(result.Groups) != 1 {
		t.Fatalf("groups = %#v", result.Groups)
	}
	group := result.Groups[0]
	if group.Decision != "candidate_better" || group.CurrentAccountID != 11 || group.CandidateAccountID != 12 {
		t.Fatalf("group = %#v", group)
	}
	if group.ScoreDelta < 0.05 || !strings.Contains(strings.Join(group.Reasons, " "), "稳定性更高") {
		t.Fatalf("recommendation = %#v", group)
	}
}

func TestAnalyzeFailsClosedForRecommendationEvidenceGates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*sub2api.AccountMonitorProjection)
		wantState string
	}{
		{
			name: "stale projection",
			mutate: func(value *sub2api.AccountMonitorProjection) {
				value.Stale = true
			},
			wantState: "stale",
		},
		{
			name: "insufficient samples",
			mutate: func(value *sub2api.AccountMonitorProjection) {
				value.Accounts[1].SampleCount = 2
			},
			wantState: "insufficient_samples",
		},
		{
			name: "incompatible model",
			mutate: func(value *sub2api.AccountMonitorProjection) {
				value.Accounts[1].ModelID = "gpt-5.6-other"
			},
			wantState: "incompatible_model",
		},
		{
			name: "inactive candidate",
			mutate: func(value *sub2api.AccountMonitorProjection) {
				value.Accounts[1].Status = "disabled"
			},
			wantState: "candidate_inactive_or_unschedulable",
		},
		{
			name: "failed multiplier evidence",
			mutate: func(value *sub2api.AccountMonitorProjection) {
				value.Accounts[1].Multiplier = sub2api.AccountMonitorMultiplier{
					Source: "measured",
					Status: "failed",
				}
			},
			wantState: "multiplier_unavailable",
		},
		{
			name: "tie within margin",
			mutate: func(value *sub2api.AccountMonitorProjection) {
				value.Accounts[1] = value.Accounts[0]
				value.Accounts[1].AccountID = 12
				value.Accounts[1].Name = "账号 B"
			},
			wantState: "margin_below_threshold",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			value := projection(
				account(11, "账号 A", "gpt-5.6-sol", 0.70, 450, 1_600, 0.12, 100, 8, 4),
				account(12, "账号 B", "gpt-5.6-sol", 0.96, 120, 500, 0.08, 40, 2, 0),
			)
			test.mutate(&value)
			group := Analyze(value).Groups[0]
			if group.Decision == "candidate_better" || group.EvidenceState != test.wantState {
				t.Fatalf("group = %#v", group)
			}
		})
	}
}

func TestAnalyzeSortsGroupsAndAccountsDeterministically(t *testing.T) {
	t.Parallel()

	value := projection(
		accountWithGroups(22, "账号 22", "gpt", 0.9, 100, 400, 0.1, []int64{9, 3}),
		accountWithGroups(11, "账号 11", "gpt", 0.9, 100, 400, 0.1, []int64{3}),
	)
	result := Analyze(value)
	if len(result.Groups) != 2 || result.Groups[0].GroupID != 3 || result.Groups[1].GroupID != 9 {
		t.Fatalf("groups = %#v", result.Groups)
	}
	if result.Groups[0].CurrentAccountID != 11 || result.Groups[0].CandidateAccountID != 22 {
		t.Fatalf("group 3 = %#v", result.Groups[0])
	}
}

func projection(accounts ...sub2api.AccountMonitorAccount) sub2api.AccountMonitorProjection {
	return sub2api.AccountMonitorProjection{
		SchemaVersion: 2,
		ObservedAt:    time.Date(2026, 7, 25, 7, 0, 0, 0, time.UTC),
		Settings:      sub2api.AccountMonitorSettings{IntervalSeconds: 300},
		Accounts:      accounts,
	}
}

func account(id int64, name, model string, success, ttft, latency, multiplier float64, samples, requests, errors int64) sub2api.AccountMonitorAccount {
	return accountWithGroups(id, name, model, success, ttft, latency, multiplier, []int64{3})
}

func accountWithGroups(id int64, name, model string, success, ttft, latency, multiplier float64, groupIDs []int64) sub2api.AccountMonitorAccount {
	return sub2api.AccountMonitorAccount{
		AccountID: id, Name: name, Platform: "openai", Status: "active", Schedulable: true,
		GroupIDs: groupIDs, GroupNames: []string{"GPT-Pro"}, ModelID: model, LatestStatus: "passed",
		SampleCount: 4, SuccessRate: success, TTFTP95MS: float64ptr(ttft), LatencyP95MS: float64ptr(latency),
		Multiplier: sub2api.AccountMonitorMultiplier{
			Value:      float64ptr(multiplier),
			Source:     "declared",
			Status:     "ok",
			ObservedAt: timePtr(time.Date(2026, 7, 25, 6, 58, 0, 0, time.UTC)),
		},
		RequestCount: 40, ErrorCount: 0,
		CheckedAt:    timePtr(time.Date(2026, 7, 25, 6, 59, 0, 0, time.UTC)),
		UsageWindows: []sub2api.AccountMonitorUsageWindow{{Name: "daily", Utilization: 0.2}},
	}
}

func float64ptr(value float64) *float64  { return &value }
func timePtr(value time.Time) *time.Time { return &value }
