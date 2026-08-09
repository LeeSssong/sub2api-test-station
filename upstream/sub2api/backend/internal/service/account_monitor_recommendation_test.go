package service

import (
	"testing"
	"time"
)

func recommendationFloat(value float64) *float64 { return &value }

func recommendationGroups() []AccountMonitorGroup {
	return []AccountMonitorGroup{
		{Name: "GPT-Pro", Status: StatusActive, RateMultiplier: 1},
		{Name: "【专属】GPT-Pro", Status: StatusActive, RateMultiplier: 1},
		{Name: "GPT-Plus", Status: StatusActive, RateMultiplier: 1},
		{Name: "GPT-特惠", Status: StatusActive, RateMultiplier: 1},
		{Name: "GPT-特惠分组", Status: StatusActive, RateMultiplier: 1},
		{Name: "GPT-测试分组", Status: StatusActive, RateMultiplier: 1},
	}
}

func recommendationEvidence(now time.Time, successRate float64) AccountMonitorQualityEvidence {
	return AccountMonitorQualityEvidence{
		Source: "monitor_probe", SampleCount: 20, SuccessSampleCount: 20,
		TTFTSampleCount: 20, LatencySampleCount: 20, SuccessRate: successRate,
		TTFTP50MS: recommendationFloat(500), LatencyP95MS: recommendationFloat(5000),
		ObservedAt: now.Add(-time.Minute),
	}
}

func recommendationLatest(now time.Time) AccountMonitorLatest {
	return AccountMonitorLatest{Status: "success", CheckedAt: now.Add(-time.Minute)}
}

func recommendationAccount(_ ...string) Account {
	rate := 0.1
	return Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive,
		Schedulable: true, RateMultiplier: &rate, Name: "gpt-account"}
}

func TestAccountMonitorGroupRecommendationNormalizesNamesAndUsesStableTargets(t *testing.T) {
	now := time.Now().UTC()
	for _, tt := range []struct {
		current string
		rate    float64
		want    string
	}{
		{"GPT-Pro", .96, AccountMonitorGroupRecommendationTargetPlus},
		{"【专属】GPT-Pro", .96, AccountMonitorGroupRecommendationTargetPlus},
		{"GPT-Plus", 1, AccountMonitorGroupRecommendationTargetPro},
		{"GPT-特惠", 1, AccountMonitorGroupRecommendationTargetPro},
		{"GPT-特惠分组", 1, AccountMonitorGroupRecommendationTargetPro},
		{"GPT-测试分组", 1, AccountMonitorGroupRecommendationTargetPro},
	} {
		got := EvaluateAccountMonitorGroupRecommendation(recommendationAccount(tt.current), []string{tt.current}, recommendationGroups(), recommendationEvidence(now, tt.rate), recommendationLatest(now), now)
		if got == nil || got.Target != tt.want {
			t.Fatalf("current group %q got %#v, want target %q", tt.current, got, tt.want)
		}
	}

	for _, current := range []string{"GPT-Pro", "【专属】GPT-Pro"} {
		got := EvaluateAccountMonitorGroupRecommendation(recommendationAccount("GPT-测试分组"), []string{"GPT-测试分组"}, []AccountMonitorGroup{{Name: current, Status: StatusActive, RateMultiplier: 1}}, recommendationEvidence(now, 1), recommendationLatest(now), now)
		if got == nil || got.Target != AccountMonitorGroupRecommendationTargetPro || got.TargetName != "GPT-Pro" {
			t.Fatalf("pro aliases must resolve to one target: %#v", got)
		}
	}
}

func TestAccountMonitorGroupRecommendationProAliasesShareEveryCostAndQualityConstraint(t *testing.T) {
	now := time.Now().UTC()
	publicPro := AccountMonitorGroup{
		Name: "GPT-Pro", Status: StatusActive, RateMultiplier: 1,
		ScoreWeights: AccountMonitorScoreWeights{TTFTTargetMS: 1000, TTFTLimitMS: 4000, LatencyTargetMS: 10000, LatencyLimitMS: 60000},
	}
	exclusivePro := AccountMonitorGroup{
		Name: "【专属】GPT-Pro", Status: StatusActive, RateMultiplier: 1.1,
		ScoreWeights: AccountMonitorScoreWeights{TTFTTargetMS: 1000, TTFTLimitMS: 6000, LatencyTargetMS: 10000, LatencyLimitMS: 40000},
	}

	t.Run("exclusive Pro uses its 0.02 margin", func(t *testing.T) {
		account := recommendationAccount()
		account.RateMultiplier = recommendationFloat(.97)
		got := EvaluateAccountMonitorGroupRecommendation(account, []string{"GPT-测试分组"}, []AccountMonitorGroup{{Name: "【专属】GPT-Pro", Status: StatusActive, RateMultiplier: 1}}, recommendationEvidence(now, 1), recommendationLatest(now), now)
		if got == nil || got.Target != AccountMonitorGroupRecommendationTargetPro {
			t.Fatalf("exclusive Pro recommendation = %#v, want Pro at the 0.02 margin", got)
		}
	})

	for _, groups := range [][]AccountMonitorGroup{{publicPro, exclusivePro}, {exclusivePro, publicPro}} {
		t.Run("all alias cost ceilings survive group order", func(t *testing.T) {
			account := recommendationAccount()
			account.RateMultiplier = recommendationFloat(.96)
			got := EvaluateAccountMonitorGroupRecommendation(account, []string{"GPT-测试分组"}, groups, recommendationEvidence(now, 1), recommendationLatest(now), now)
			if got == nil || got.Status != AccountMonitorGroupRecommendationStatusBlocked {
				t.Fatalf("recommendation = %#v, want public Pro's 0.05 margin to reject cost", got)
			}
		})

		t.Run("all alias quality limits survive group order", func(t *testing.T) {
			account := recommendationAccount()
			account.RateMultiplier = recommendationFloat(.9)
			evidence := recommendationEvidence(now, 1)
			evidence.TTFTP50MS = recommendationFloat(5000)
			evidence.LatencyP95MS = recommendationFloat(50000)
			got := EvaluateAccountMonitorGroupRecommendation(account, []string{"GPT-测试分组"}, groups, evidence, recommendationLatest(now), now)
			if got == nil || got.Status != AccountMonitorGroupRecommendationStatusNotRecommended || len(got.ReasonCodes) != 1 || got.ReasonCodes[0] != "ttft_exceeds_target" {
				t.Fatalf("recommendation = %#v, want order-independent TTFT-first quality rejection", got)
			}
		})
	}
}

func TestAccountMonitorGroupRecommendationHardGates(t *testing.T) {
	now := time.Now().UTC()
	base := recommendationAccount("GPT-测试分组")
	tests := []struct {
		name   string
		mutate func(*Account, *AccountMonitorQualityEvidence, *AccountMonitorLatest)
		want   string
	}{
		{"sample_insufficient", func(_ *Account, e *AccountMonitorQualityEvidence, _ *AccountMonitorLatest) { e.SampleCount = 19 }, AccountMonitorGroupRecommendationStatusObserve},
		{"stale_evidence", func(_ *Account, e *AccountMonitorQualityEvidence, _ *AccountMonitorLatest) {
			e.ObservedAt = now.Add(-8 * 24 * time.Hour)
		}, AccountMonitorGroupRecommendationStatusObserve},
		{"unavailable_multiplier", func(a *Account, _ *AccountMonitorQualityEvidence, _ *AccountMonitorLatest) { a.RateMultiplier = nil }, AccountMonitorGroupRecommendationStatusBlocked},
		{"cost_above_margin", func(a *Account, _ *AccountMonitorQualityEvidence, _ *AccountMonitorLatest) {
			a.RateMultiplier = recommendationFloat(0.99)
		}, AccountMonitorGroupRecommendationStatusBlocked},
		{"fatal_401", func(_ *Account, _ *AccountMonitorQualityEvidence, l *AccountMonitorLatest) {
			code := 401
			l.HTTPStatus = &code
		}, AccountMonitorGroupRecommendationStatusBlocked},
		{"fatal_403", func(_ *Account, _ *AccountMonitorQualityEvidence, l *AccountMonitorLatest) {
			code := 403
			l.HTTPStatus = &code
		}, AccountMonitorGroupRecommendationStatusBlocked},
		{"fatal_402", func(_ *Account, _ *AccountMonitorQualityEvidence, l *AccountMonitorLatest) {
			code := 402
			l.HTTPStatus = &code
		}, AccountMonitorGroupRecommendationStatusBlocked},
		{"fatal_auth", func(_ *Account, _ *AccountMonitorQualityEvidence, l *AccountMonitorLatest) {
			l.ErrorCode = "auth_failed"
		}, AccountMonitorGroupRecommendationStatusBlocked},
		{"fatal_balance", func(_ *Account, _ *AccountMonitorQualityEvidence, l *AccountMonitorLatest) {
			l.ErrorCode = "balance_exhausted"
		}, AccountMonitorGroupRecommendationStatusBlocked},
		{"fatal_quota", func(_ *Account, _ *AccountMonitorQualityEvidence, l *AccountMonitorLatest) {
			l.ErrorCode = "quota"
		}, AccountMonitorGroupRecommendationStatusBlocked},
		{"fatal_billing", func(_ *Account, _ *AccountMonitorQualityEvidence, l *AccountMonitorLatest) {
			l.ErrorCode = "billing"
		}, AccountMonitorGroupRecommendationStatusBlocked},
		{"missing_latency", func(_ *Account, e *AccountMonitorQualityEvidence, _ *AccountMonitorLatest) {
			e.LatencySampleCount = 0
			e.LatencyP95MS = nil
		}, AccountMonitorGroupRecommendationStatusBlocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := base
			e := recommendationEvidence(now, 1)
			l := recommendationLatest(now)
			tt.mutate(&a, &e, &l)
			got := EvaluateAccountMonitorGroupRecommendation(a, []string{"GPT-测试分组"}, recommendationGroups(), e, l, now)
			if got == nil || got.Status != tt.want {
				t.Fatalf("got %#v, want status %q", got, tt.want)
			}
		})
	}

	formal := recommendationAccount("GPT-Pro")
	if got := EvaluateAccountMonitorGroupRecommendation(formal, []string{"GPT-Pro"}, []AccountMonitorGroup{{Name: "GPT-Pro", RateMultiplier: 1}}, recommendationEvidence(now, 1), recommendationLatest(now), now); got != nil {
		t.Fatalf("formal account without alternate target must return nil: %#v", got)
	}
}

func TestAccountMonitorGroupRecommendationTreatsUnavailableModelsAsHardGates(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name               string
		latest             AccountMonitorLatest
		insufficientSample bool
	}{
		{name: "classified model unavailable overrides insufficient samples", latest: AccountMonitorLatest{Status: "failed", ErrorCode: "model_unavailable", CheckedAt: now}, insufficientSample: true},
		{name: "model not found message", latest: AccountMonitorLatest{Status: "failed", ErrorCode: "model not found", CheckedAt: now}},
		{name: "HTTP 404", latest: AccountMonitorLatest{Status: "failed", HTTPStatus: func() *int { value := 404; return &value }(), CheckedAt: now}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := recommendationEvidence(now, 1)
			if tt.insufficientSample {
				evidence.SampleCount = accountMonitorGroupRecommendationMinSamples - 1
			}
			got := EvaluateAccountMonitorGroupRecommendation(recommendationAccount(), []string{"GPT-测试分组"}, recommendationGroups(), evidence, tt.latest, now)
			if got == nil || got.Status != AccountMonitorGroupRecommendationStatusBlocked || len(got.ReasonCodes) == 0 || got.ReasonCodes[0] != "model_unavailable" {
				t.Fatalf("test-group recommendation = %#v, want model_unavailable block", got)
			}

			got = EvaluateAccountMonitorGroupRecommendation(recommendationAccount(), []string{"GPT-Plus"}, recommendationGroups(), recommendationEvidence(now, 1), tt.latest, now)
			if got != nil {
				t.Fatalf("formal-group unavailable model must stay silent: %#v", got)
			}
		})
	}
}

func TestAccountMonitorGroupRecommendationQualityTiersFallThrough(t *testing.T) {
	now := time.Now().UTC()
	for _, tt := range []struct {
		name string
		rate float64
		want string
	}{
		{"pro", .98, AccountMonitorGroupRecommendationTargetPro},
		{"plus", .95, AccountMonitorGroupRecommendationTargetPlus},
		{"special", .70, AccountMonitorGroupRecommendationTargetSpecial},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateAccountMonitorGroupRecommendation(recommendationAccount("GPT-测试分组"), []string{"GPT-测试分组"}, recommendationGroups(), recommendationEvidence(now, tt.rate), recommendationLatest(now), now)
			if got == nil || got.Target != tt.want || got.Status != AccountMonitorGroupRecommendationStatusRecommended {
				t.Fatalf("got %#v, want target %q", got, tt.want)
			}
		})
	}

	e := recommendationEvidence(now, .97)
	got := EvaluateAccountMonitorGroupRecommendation(recommendationAccount("GPT-测试分组"), []string{"GPT-测试分组"}, recommendationGroups(), e, recommendationLatest(now), now)
	if got == nil || got.Target != AccountMonitorGroupRecommendationTargetPlus {
		t.Fatalf("failed Pro must fall through to Plus: %#v", got)
	}

	e = recommendationEvidence(now, .69)
	got = EvaluateAccountMonitorGroupRecommendation(recommendationAccount("GPT-测试分组"), []string{"GPT-测试分组"}, recommendationGroups(), e, recommendationLatest(now), now)
	if got == nil || got.Status != AccountMonitorGroupRecommendationStatusNotRecommended {
		t.Fatalf("failed all tiers must be not_recommended for test group: %#v", got)
	}

	for _, tt := range []struct {
		name   string
		mutate func(*AccountMonitorQualityEvidence)
	}{
		{"ttft_limit", func(e *AccountMonitorQualityEvidence) { *e.TTFTP50MS = float64(AccountMonitorDefaultTTFTLimitMS + 1) }},
		{"latency_limit", func(e *AccountMonitorQualityEvidence) {
			*e.LatencyP95MS = float64(AccountMonitorDefaultLatencyLimitMS + 1)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := recommendationEvidence(now, 1)
			tt.mutate(&e)
			got := EvaluateAccountMonitorGroupRecommendation(recommendationAccount("GPT-测试分组"), []string{"GPT-测试分组"}, recommendationGroups(), e, recommendationLatest(now), now)
			if got == nil || got.Status != AccountMonitorGroupRecommendationStatusNotRecommended {
				t.Fatalf("got %#v, want not_recommended", got)
			}
		})
	}
}

func TestAccountMonitorGroupRecommendationCodexAuth(t *testing.T) {
	now := time.Now().UTC()
	a := Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"auth_mode": OpenAIAuthModeAgentIdentity}}
	got := EvaluateAccountMonitorGroupRecommendation(a, []string{"GPT-测试分组"}, recommendationGroups(), recommendationEvidence(now, .98), recommendationLatest(now), now)
	if got == nil || got.Target != AccountMonitorGroupRecommendationTargetPro || got.Status != AccountMonitorGroupRecommendationStatusRecommended {
		t.Fatalf("healthy Codex Auth must default to Pro: %#v", got)
	}

	for _, tt := range []struct {
		name   string
		mutate func(*AccountMonitorQualityEvidence)
		reason string
	}{
		{name: "success rate", mutate: func(e *AccountMonitorQualityEvidence) { e.SuccessRate = .97 }, reason: "success_rate_below_pro"},
		{name: "TTFT", mutate: func(e *AccountMonitorQualityEvidence) {
			e.TTFTP50MS = recommendationFloat(AccountMonitorDefaultTTFTLimitMS + 1)
		}, reason: "ttft_exceeds_target"},
		{name: "latency", mutate: func(e *AccountMonitorQualityEvidence) {
			e.LatencyP95MS = recommendationFloat(AccountMonitorDefaultLatencyLimitMS + 1)
		}, reason: "latency_exceeds_limit"},
	} {
		t.Run("quality failure holds Pro target: "+tt.name, func(t *testing.T) {
			evidence := recommendationEvidence(now, 1)
			tt.mutate(&evidence)
			got := EvaluateAccountMonitorGroupRecommendation(a, []string{"GPT-测试分组"}, recommendationGroups(), evidence, recommendationLatest(now), now)
			if got == nil || got.Status != AccountMonitorGroupRecommendationStatusBlocked || got.Target != AccountMonitorGroupRecommendationTargetPro || got.Action != AccountMonitorGroupRecommendationActionHold || len(got.ReasonCodes) == 0 || got.ReasonCodes[0] != tt.reason {
				t.Fatalf("test-group recommendation = %#v, want Pro hold for %s", got, tt.reason)
			}

			got = EvaluateAccountMonitorGroupRecommendation(a, []string{"GPT-Plus"}, recommendationGroups(), evidence, recommendationLatest(now), now)
			if got != nil {
				t.Fatalf("formal-group quality failure must stay silent: %#v", got)
			}
		})
	}

	a.RateMultiplier = recommendationFloat(.96)
	got = EvaluateAccountMonitorGroupRecommendation(a, []string{"GPT-测试分组"}, recommendationGroups(), recommendationEvidence(now, 1), recommendationLatest(now), now)
	if got == nil || got.Target != AccountMonitorGroupRecommendationTargetPro || got.Action != AccountMonitorGroupRecommendationActionHold || len(got.ReasonCodes) == 0 || got.ReasonCodes[0] != "profit_below_minimum" {
		t.Fatalf("configured Codex Auth multiplier must meet Pro cost limits: %#v", got)
	}

	e := recommendationEvidence(now, .99)
	e.Source = "stale"
	got = EvaluateAccountMonitorGroupRecommendation(a, []string{"GPT-测试分组"}, recommendationGroups(), e, recommendationLatest(now), now)
	if got == nil || got.Target != AccountMonitorGroupRecommendationTargetPro || got.Action != AccountMonitorGroupRecommendationActionHold {
		t.Fatalf("fatal/stale Codex Auth in test group must hold Pro target: %#v", got)
	}
}

func TestAccountMonitorGroupRecommendationExcludesImageAndClaudeWorkloads(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name   string
		groups []string
		model  string
	}{
		{name: "image group", groups: []string{"GPT-测试分组", "OpenAI 生图"}, model: "gpt-5.4"},
		{name: "Claude group", groups: []string{"GPT-测试分组", "CLAUDE 专组"}, model: "gpt-5.4"},
		{name: "gpt-image model", groups: []string{"GPT-测试分组"}, model: "gpt-image-2"},
		{name: "image model", groups: []string{"GPT-测试分组"}, model: "image-1"},
		{name: "Claude model", groups: []string{"GPT-测试分组"}, model: "claude-sonnet-4-5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			latest := recommendationLatest(now)
			latest.ModelID = tt.model
			got := EvaluateAccountMonitorGroupRecommendation(recommendationAccount(), tt.groups, recommendationGroups(), recommendationEvidence(now, 1), latest, now)
			if got != nil {
				t.Fatalf("excluded workload recommendation = %#v, want nil", got)
			}
		})
	}
}
