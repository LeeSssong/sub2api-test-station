package accounthealth

import "testing"

func float64Ptr(v float64) *float64 { return &v }

func TestClassifyAccount(t *testing.T) {
	cases := []struct {
		name     string
		sample   AccountSample
		wantTier Tier
		wantSlow bool
	}{
		{"健康下边界", AccountSample{SuccessRate: 0.95, SampleCount: 10}, TierHealthy, false},
		{"健康上方", AccountSample{SuccessRate: 0.951, SampleCount: 10}, TierHealthy, false},
		{"降级上边界", AccountSample{SuccessRate: 0.949, SampleCount: 10}, TierDegraded, false},
		{"降级下边界", AccountSample{SuccessRate: 0.50, SampleCount: 10}, TierDegraded, false},
		{"不可用上边界", AccountSample{SuccessRate: 0.499, SampleCount: 10}, TierUnavailable, false},
		{"余额耗尽直判不可用", AccountSample{SuccessRate: 1.0, SampleCount: 10, ErrorCode: ErrorCodeBalanceExhausted}, TierUnavailable, false},
		{"最新成功覆盖窗口旧失败", AccountSample{SuccessRate: 0.01, SampleCount: 100, LatestStatus: statusSuccess}, TierHealthy, false},
		{"最新失败覆盖窗口旧成功", AccountSample{SuccessRate: 0.99, SampleCount: 100, LatestStatus: "failed"}, TierUnavailable, false},
		{"活跃但未参与调度", AccountSample{SuccessRate: 1.0, SampleCount: 10, Unschedulable: true}, TierUnavailable, false},
		{"无样本判未知", AccountSample{SuccessRate: 0, SampleCount: 0}, TierUnknown, false},
		// 窗口口径下零样本是常态（新增账号、跨零点后第一小时）。余额耗尽的
		// 短路必须先于零样本判定：判成 Unknown 会被 GroupAvailabilities 剔出
		// Total，3 账号组缩成 2 账号组，告警阈值从「<=1」悄悄放宽到「==0」。
		{"零样本余额耗尽仍判不可用", AccountSample{SuccessRate: 0, SampleCount: 0, ErrorCode: ErrorCodeBalanceExhausted}, TierUnavailable, false},
		{"零样本其他错误码判未知", AccountSample{SuccessRate: 0, SampleCount: 0, ErrorCode: "http_error"}, TierUnknown, false},
		{"延时边界内不标记", AccountSample{SuccessRate: 1.0, SampleCount: 10, TTFTP95MS: float64Ptr(3000)}, TierHealthy, false},
		{"延时超阈值标记", AccountSample{SuccessRate: 1.0, SampleCount: 10, TTFTP95MS: float64Ptr(3000.1)}, TierHealthy, true},
		{"偏慢不降档", AccountSample{SuccessRate: 0.99, SampleCount: 10, TTFTP95MS: float64Ptr(9000)}, TierHealthy, true},
		{"延时缺失不标记", AccountSample{SuccessRate: 1.0, SampleCount: 10, TTFTP95MS: nil}, TierHealthy, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyAccount(tc.sample)
			if got.Tier != tc.wantTier {
				t.Fatalf("Tier = %q, want %q", got.Tier, tc.wantTier)
			}
			if got.Slow != tc.wantSlow {
				t.Fatalf("Slow = %v, want %v", got.Slow, tc.wantSlow)
			}
		})
	}
}

func TestClassifyAccountCarriesIdentity(t *testing.T) {
	got := ClassifyAccount(AccountSample{
		AccountID: 21, Name: "Pro-SHUAI-0.17", GroupNames: []string{"GPT-Pro"},
		SuccessRate: 0.98, SampleCount: 100,
	})
	if got.AccountID != 21 || got.Name != "Pro-SHUAI-0.17" {
		t.Fatalf("identity lost: %+v", got)
	}
	if len(got.GroupNames) != 1 || got.GroupNames[0] != "GPT-Pro" {
		t.Fatalf("GroupNames = %v", got.GroupNames)
	}
}
