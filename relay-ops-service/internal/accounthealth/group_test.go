package accounthealth

import "testing"

func TestShouldAlertByCapacity(t *testing.T) {
	cases := []struct {
		total, available int
		want             bool
	}{
		{4, 2, false}, {4, 1, true}, {4, 0, true},
		{3, 2, false}, {3, 1, true}, {3, 0, true},
		{2, 2, false}, {2, 1, false}, {2, 0, true},
		{1, 1, false}, {1, 0, true},
		{0, 0, false},
	}
	for _, tc := range cases {
		if got := ShouldAlert(tc.total, tc.available); got != tc.want {
			t.Fatalf("ShouldAlert(%d,%d) = %v, want %v", tc.total, tc.available, got, tc.want)
		}
	}
}

func TestGroupAvailabilitiesAggregates(t *testing.T) {
	verdicts := []AccountVerdict{
		{Name: "Plus-TK-0.08", GroupNames: []string{"GPT-Plus"}, Tier: TierDegraded},
		{Name: "Plus-XN-0.09", GroupNames: []string{"GPT-Plus"}, Tier: TierUnavailable},
		{Name: "Plus-XM-0.1", GroupNames: []string{"GPT-Plus"}, Tier: TierUnavailable},
		{Name: "Pro-SHEN-0.16", GroupNames: []string{"GPT-Pro"}, Tier: TierHealthy},
		{Name: "Pro-TK-0.15", GroupNames: []string{"GPT-Pro"}, Tier: TierHealthy},
		{Name: "Pro-SHUAI-0.17", GroupNames: []string{"GPT-Pro"}, Tier: TierHealthy},
	}
	groups := GroupAvailabilities(verdicts)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].GroupName != "GPT-Plus" || groups[1].GroupName != "GPT-Pro" {
		t.Fatalf("groups not sorted: %+v", groups)
	}
	plus := groups[0]
	if plus.Total != 3 || plus.Available != 1 || !plus.Alerting {
		t.Fatalf("GPT-Plus = %+v, want total 3 available 1 alerting", plus)
	}
	if len(plus.Down) != 2 {
		t.Fatalf("GPT-Plus Down = %+v, want 2", plus.Down)
	}
	pro := groups[1]
	if pro.Total != 3 || pro.Available != 3 || pro.Alerting {
		t.Fatalf("GPT-Pro = %+v", pro)
	}
}

func TestGroupAvailabilitiesSingleAccountGroupNotAlerting(t *testing.T) {
	verdicts := []AccountVerdict{
		{Name: "GPT特惠-TK-0.08", GroupNames: []string{"GPT特惠", "GPT-PLUS-内测"}, Tier: TierDegraded},
	}
	groups := GroupAvailabilities(verdicts)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	for _, group := range groups {
		if group.Total != 1 || group.Available != 1 {
			t.Fatalf("%s = %+v", group.GroupName, group)
		}
		if group.Alerting {
			t.Fatalf("%s 单账号分组可用时不得告警", group.GroupName)
		}
	}
}

func TestGroupAvailabilitiesExcludesUnknown(t *testing.T) {
	verdicts := []AccountVerdict{
		{Name: "A", GroupNames: []string{"G"}, Tier: TierHealthy},
		{Name: "B", GroupNames: []string{"G"}, Tier: TierUnknown},
	}
	groups := GroupAvailabilities(verdicts)
	if groups[0].Total != 1 || groups[0].Available != 1 {
		t.Fatalf("unknown 账号必须排除在容量之外: %+v", groups[0])
	}
	if groups[0].Alerting {
		t.Fatal("单账号分组且可用，不得告警")
	}
}

func TestGroupAvailabilitiesIgnoresAccountsWithoutGroup(t *testing.T) {
	verdicts := []AccountVerdict{{Name: "orphan", GroupNames: nil, Tier: TierHealthy}}
	if groups := GroupAvailabilities(verdicts); len(groups) != 0 {
		t.Fatalf("groups = %+v, want empty", groups)
	}
}
