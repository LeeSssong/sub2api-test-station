package opsmonitor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/accountquality"
	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

func TestServiceConfirmsErrorRateAfterTwoCompleteWindows(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{
		groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}},
		ops: map[snapshotKey]sub2api.OpsSnapshot{
			{groupID: 2, rangeName: "15m"}: snapshot(20, 18, 0.06, 1200),
			{groupID: 2, rangeName: "24h"}: snapshot(500, 490, 0.02, 1000),
		},
	}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Fatalf("first window sent = %#v", notifier.sent)
	}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 || notifier.sent[0].key != "site:group:2:error_rate" {
		t.Fatalf("sent = %#v", notifier.sent)
	}
	if record := repository.records["site:group:2:error_rate"]; record.Severity != "P1" || record.State != "confirmed" {
		t.Fatalf("record = %#v", record)
	}
	if reader.writeCalls != 0 {
		t.Fatalf("native writes = %d", reader.writeCalls)
	}
}

func TestServiceDoesNotDuplicatePublicRuntimeAlertsForEveryAccount(t *testing.T) {
	now := time.Date(2026, 7, 28, 4, 0, 0, 0, time.UTC)
	reader := &fakeReader{
		groups: []sub2api.Group{{ID: 16, Name: "GPT-PLUS-内测", Status: "active"}},
		ops:    map[snapshotKey]sub2api.OpsSnapshot{},
	}
	reader.ops[snapshotKey{groupID: 16, rangeName: "15m"}] = snapshot(20, 18, 0.06, 1200)
	reader.ops[snapshotKey{groupID: 16, rangeName: "24h"}] = snapshot(500, 490, 0.02, 1000)
	for id := int64(20); id < 28; id++ {
		reader.accounts = append(reader.accounts, sub2api.Account{
			ID: id, Name: "account", Status: "active", Schedulable: true,
		})
		reader.ops[snapshotKey{accountID: id, rangeName: "15m"}] = snapshot(20, 18, 0.06, 1200)
		reader.ops[snapshotKey{accountID: id, rangeName: "24h"}] = snapshot(500, 490, 0.02, 1000)
	}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)

	for range 2 {
		if err := service.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(notifier.sent) != 1 || notifier.sent[0].key != "site:group:16:error_rate" {
		t.Fatalf("sent = %#v", notifier.sent)
	}
	for key := range repository.records {
		if strings.HasPrefix(key, "site:account:") &&
			(strings.HasSuffix(key, ":availability") || strings.HasSuffix(key, ":error_rate") || strings.HasSuffix(key, ":ttft_p95")) {
			t.Fatalf("duplicate account runtime incident created: %s", key)
		}
	}
}

func TestServiceSuppressesUnchangedConfirmedMetricAtANewerEvidenceTime(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{
		groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}},
		ops: map[snapshotKey]sub2api.OpsSnapshot{
			{groupID: 2, rangeName: "15m"}: snapshot(20, 18, 0.06, 1200),
			{groupID: 2, rangeName: "24h"}: snapshot(500, 490, 0.02, 1000),
		},
	}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)
	service.Now = func() time.Time { return now }
	for range 3 {
		if err := service.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		now = now.Add(15 * time.Minute)
	}
	if len(notifier.sent) != 1 || notifier.sent[0].key != "site:group:2:error_rate" {
		t.Fatalf("unchanged metric sent = %#v", notifier.sent)
	}
}

func TestServiceSkipsSampleInsufficientWindow(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{
		groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}},
		ops: map[snapshotKey]sub2api.OpsSnapshot{
			{groupID: 2, rangeName: "15m"}: snapshot(19, 18, 1, 5000),
			{groupID: 2, rangeName: "24h"}: snapshot(500, 490, 0.02, 1000),
		},
	}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 || len(repository.records) != 0 {
		t.Fatalf("sample-insufficient must be silent: sent=%#v records=%#v", notifier.sent, repository.records)
	}
}

func TestServiceDoesNotRecoverAnIncidentFromASampleInsufficientWindow(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{
		groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}},
		ops: map[snapshotKey]sub2api.OpsSnapshot{
			{groupID: 2, rangeName: "15m"}: snapshot(19, 18, 0.01, 1200),
			{groupID: 2, rangeName: "24h"}: snapshot(500, 490, 0.02, 1000),
		},
	}
	repository := newMemoryRepository()
	repository.records["site:group:2:availability"] = incidents.Record{Key: "site:group:2:availability", Severity: "P1", State: "confirmed", SampleCount: 2}
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 || repository.records["site:group:2:availability"].State != "confirmed" {
		t.Fatalf("sample-insufficient must not recover: sent=%#v record=%#v", notifier.sent, repository.records["site:group:2:availability"])
	}
}

func TestServiceAlertsPausedAccountImmediately(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{accounts: []sub2api.Account{{ID: 7, Name: "Current but paused", Status: "active", Schedulable: false}}}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 || notifier.sent[0].key != "site:account:7:paused" {
		t.Fatalf("sent = %#v", notifier.sent)
	}
	if record := repository.records["site:account:7:paused"]; record.Severity != "P1" || record.State != "confirmed" {
		t.Fatalf("record = %#v", record)
	}
}

func TestPausedAlertDisplaysAccountNameButKeepsIDKey(t *testing.T) {
	now := time.Date(2026, 7, 27, 2, 0, 0, 0, time.UTC)
	reader := &fakeReader{
		accounts: []sub2api.Account{
			{ID: 35, Name: "特惠-SHUAI", Status: "active", Schedulable: false},
		},
	}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(notifier.sent))
	}
	if notifier.sent[0].key != "site:account:35:paused" {
		t.Fatalf("key = %q", notifier.sent[0].key)
	}
	text := notifier.sent[0].message.RenderedText()
	if !strings.Contains(text, "站内运行告警：特惠-SHUAI") ||
		!strings.Contains(text, "对象：特惠-SHUAI") {
		t.Fatalf("account name missing: %s", text)
	}
	if strings.Contains(text, "account #35") || strings.Contains(text, "账号 #35") {
		t.Fatalf("database label leaked: %s", text)
	}
}

func TestObjectDisplayNameDoesNotChangeIdentity(t *testing.T) {
	first := object{kind: "account", id: 35, name: "特惠-SHUAI"}
	renamed := object{kind: "account", id: 35, name: "特惠-SHUAI-新名称"}

	if first.key("paused") != renamed.key("paused") {
		t.Fatal("display name changed incident key")
	}
	firstHash := evidenceHash(first, "paused", "active but paused", "active && schedulable")
	renamedHash := evidenceHash(renamed, "paused", "active but paused", "active && schedulable")
	if firstHash != renamedHash {
		t.Fatal("display name changed evidence hash")
	}
}

func TestRenderAccountWithEmptyNameDoesNotExposeID(t *testing.T) {
	message := render(
		object{kind: "account", id: 35},
		"paused", "active but paused", "active && schedulable",
		1, time.Date(2026, 7, 27, 2, 0, 0, 0, time.UTC), "confirmed",
	)
	text := message.RenderedText()
	if !strings.Contains(text, "账号名称不可用") {
		t.Fatalf("missing empty-name fallback: %s", text)
	}
	if strings.Contains(text, "#35") {
		t.Fatalf("database ID leaked: %s", text)
	}
}

func TestRenderAccountMetricsUseDisplayName(t *testing.T) {
	cases := []struct {
		metric     string
		transition string
		wantTitle  string
	}{
		{"availability", "confirmed", "P1｜站内运行告警：Pro-SHUAI-0.17"},
		{"balance_exhausted", "confirmed", "P1｜上游账号质量告警：Pro-SHUAI-0.17"},
		{"multiplier", "confirmed", "账号计费倍率变更：Pro-SHUAI-0.17"},
		{"paused", "recovered", "站内运行已恢复：Pro-SHUAI-0.17"},
	}
	for _, tc := range cases {
		t.Run(tc.metric+"/"+tc.transition, func(t *testing.T) {
			message := render(
				object{kind: "account", id: 21, name: "Pro-SHUAI-0.17"},
				tc.metric, "current", "baseline", 1,
				time.Date(2026, 7, 27, 2, 0, 0, 0, time.UTC), tc.transition,
			)
			text := message.RenderedText()
			if message.Card.Header.Title.Content != tc.wantTitle {
				t.Fatalf("title=%q, want %q", message.Card.Header.Title.Content, tc.wantTitle)
			}
			if tc.transition != "recovered" && !strings.Contains(text, "对象：Pro-SHUAI-0.17") {
				t.Fatalf("alert body missing object name: %s", text)
			}
		})
	}
}

func TestRenderMultiplierChangeUsesBillingLanguageWithoutUnsupportedOpsLink(t *testing.T) {
	message := render(
		object{kind: "account", id: 21, name: "Plus-TK-极速"},
		"multiplier", "0.07x", "0.08x", 1,
		time.Date(2026, 7, 28, 1, 35, 4, 0, time.UTC), "confirmed",
	)
	text := message.RenderedText()
	for _, want := range []string{
		"账号计费倍率变更：Plus-TK-极速",
		"当前倍率** 0.07x",
		"上次记录** 0.08x",
		"确认是否符合上游价格或账号配置变化",
		"不执行路由或账号写入",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	for _, forbidden := range []string{"上游账号质量告警", "**基线**", "查看上游账号质量证据"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("found %q in %q", forbidden, text)
		}
	}
	if len(message.Card.Elements) != 1 {
		t.Fatalf("multiplier card has unsupported action: %#v", message.Card.Elements)
	}
}

func TestRenderMultiplierStabilityDoesNotClaimHealthRecoveryOrLinkOps(t *testing.T) {
	message := render(
		object{kind: "account", id: 21, name: "Plus-TK-极速"},
		"multiplier", "0.07x", "0.07x", 1,
		time.Date(2026, 7, 28, 1, 40, 4, 0, time.UTC), "recovered",
	)
	text := message.RenderedText()
	for _, want := range []string{
		"账号计费倍率记录已稳定：Plus-TK-极速",
		"当前倍率",
		"0.07x",
		"本次记录与上次记录一致",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	for _, forbidden := range []string{"已恢复", "健康基线", "健康确认"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("found %q in %q", forbidden, text)
		}
	}
	data, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	if contains(string(data), `"tag":"action"`) || contains(string(data), `"/ops"`) {
		t.Fatalf("multiplier stability card has unsupported action: %s", data)
	}
}

func TestServiceAlertsChangedMultiplierAndExplicitBalanceExhaustionImmediately(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{accounts: []sub2api.Account{{ID: 10, Name: "ListAccounts 名称", Status: "active", Schedulable: true}}}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	quality := &fakeQualitySource{result: qualityResult(now, []accountquality.Account{{
		AccountID: 10, ModelID: "gpt", SampleCount: 1, SuccessCount: 1, SuccessRate: 1,
		TTFTP50MS: number(100), TTFTP95MS: number(100), LastResult: "passed", LastObservedAt: now,
	}})}
	multipliers := &stubMultiplierSource{projection: multiplierProjection(10, number(0.05), "ok")}
	service := newService(reader, repository, notifier, now)
	service.Quality = quality
	service.Multipliers = multipliers

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Fatalf("initial multiplier sent = %#v", notifier.sent)
	}
	multipliers.projection = multiplierProjection(10, number(0.10), "ok")
	quality.result = qualityResult(now.Add(time.Minute), []accountquality.Account{{
		AccountID: 10, ModelID: "gpt", SampleCount: 1, SuccessCount: 0, SuccessRate: 0,
		LastResult: "balance_exhausted", LastErrorCode: "balance_exhausted", LastObservedAt: now.Add(time.Minute),
	}})
	service.Now = func() time.Time { return now.Add(time.Minute) }
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 2 || notifier.sent[0].key != "site:account:10:multiplier" || notifier.sent[1].key != "site:account:10:balance_exhausted" {
		t.Fatalf("sent = %#v", notifier.sent)
	}
	for _, sent := range notifier.sent {
		text := sent.message.RenderedText()
		if !strings.Contains(text, "ListAccounts 名称") {
			t.Fatalf("%s did not use ListAccounts name: %s", sent.key, text)
		}
		if strings.Contains(text, "Pro-SHEN-0.16") || strings.Contains(text, "#10") {
			t.Fatalf("%s leaked projection name or account ID: %s", sent.key, text)
		}
	}
	if record := repository.records["site:account:10:multiplier"]; record.Severity != "P2" || record.State != "confirmed" {
		t.Fatalf("multiplier record = %#v", record)
	}
	if record := repository.records["site:account:10:balance_exhausted"]; record.Severity != "P1" || record.State != "confirmed" {
		t.Fatalf("balance record = %#v", record)
	}
}

func TestServiceSuppressesAccountRuntimeAlertCopies(t *testing.T) {
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{
		accounts: []sub2api.Account{{ID: 10, Name: "ListAccounts 运行名称", Status: "active", Schedulable: true}},
		ops: map[snapshotKey]sub2api.OpsSnapshot{
			{accountID: 10, rangeName: "15m"}: snapshot(20, 18, 0.06, 1200),
			{accountID: 10, rangeName: "24h"}: snapshot(500, 490, 0.02, 1000),
		},
	}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)

	for range 2 {
		if err := service.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	reader.ops[snapshotKey{accountID: 10, rangeName: "15m"}] = snapshot(20, 20, 0.01, 1200)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(notifier.sent) != 0 {
		t.Fatalf("account runtime copies sent = %#v", notifier.sent)
	}
	for _, metric := range []string{"availability", "error_rate", "ttft_p95"} {
		if _, found := repository.records["site:account:10:"+metric]; found {
			t.Fatalf("account runtime incident %q was created", metric)
		}
	}
}

func TestServiceSuppressesQualityTransitionsForMismatchedAccountSet(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{accounts: []sub2api.Account{{ID: 10, Status: "active", Schedulable: true}}}
	repository := newMemoryRepository()
	repository.records["site:account:10:multiplier_baseline"] = incidents.Record{
		Key: "site:account:10:multiplier_baseline", CurrentValue: "0.05x", State: "muted",
	}
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)
	service.Quality = &fakeQualitySource{result: accountquality.Result{
		SchemaVersion: 1, SnapshotID: "wrong-set", ObservedAt: now,
		AccountSetSHA256: "95bce61a71a78185f4b6f8f25fc6986108043727fe9d9c19dbe44b0081ef928a",
		Accounts: []accountquality.Account{{
			AccountID: 10, SampleCount: 1, SuccessCount: 0, SuccessRate: 0,
			LastResult: "balance_exhausted", LastErrorCode: "balance_exhausted", LastObservedAt: now,
		}},
	}}

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Fatalf("mismatched quality sent = %#v", notifier.sent)
	}
	if got := repository.records["site:account:10:multiplier_baseline"].CurrentValue; got != "0.05x" {
		t.Fatalf("mismatched quality replaced multiplier baseline with %q", got)
	}
	if _, found := repository.records["site:account:10:balance_exhausted"]; found {
		t.Fatalf("mismatched quality transitioned balance: %#v", repository.records)
	}
}

func TestServiceDoesNotCreateOrReplaceMultiplierBaselineWhenUnavailable(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{accounts: []sub2api.Account{{ID: 10, Status: "active", Schedulable: true}}}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)
	service.Quality = &fakeQualitySource{result: qualityResult(now, []accountquality.Account{{
		AccountID: 10, SampleCount: 1, SuccessCount: 0, SuccessRate: 0,
		LastResult: "account_test_error", LastErrorCode: "account_test_error", LastObservedAt: now,
	}})}

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Fatalf("unavailable multiplier sent = %#v", notifier.sent)
	}
	if _, found := repository.records["site:account:10:multiplier_baseline"]; found {
		t.Fatalf("unavailable multiplier created baseline: %#v", repository.records)
	}

	repository.records["site:account:10:multiplier_baseline"] = incidents.Record{
		Key: "site:account:10:multiplier_baseline", CurrentValue: "0.05x", State: "muted",
	}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := repository.records["site:account:10:multiplier_baseline"].CurrentValue; got != "0.05x" {
		t.Fatalf("unavailable multiplier replaced baseline with %q", got)
	}
}

func TestServiceDoesNotTreatOrdinaryAccountTestErrorAsBalanceExhaustion(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{accounts: []sub2api.Account{{ID: 10, Name: "Current", Status: "active", Schedulable: true}}}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)
	service.Quality = &fakeQualitySource{result: qualityResult(now, []accountquality.Account{{
		AccountID: 10, SampleCount: 1, SuccessCount: 0, SuccessRate: 0,
		LastResult: "account_test_error", LastErrorCode: "account_test_error", LastObservedAt: now,
	}})}

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 {
		t.Fatalf("ordinary account test error sent = %#v", notifier.sent)
	}
	if _, found := repository.records["site:account:10:balance_exhausted"]; found {
		t.Fatalf("ordinary account test error must not create balance incident: %#v", repository.records)
	}
}

func TestServiceDoesNotRecoverConfirmedBalanceIncidentFromAnAccountTestError(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{accounts: []sub2api.Account{{ID: 10, Name: "Current", Status: "active", Schedulable: true}}}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	quality := &fakeQualitySource{result: qualityResult(now, []accountquality.Account{{
		AccountID: 10, ModelID: "gpt", SampleCount: 1, SuccessCount: 0, SuccessRate: 0,
		LastResult: "balance_exhausted", LastErrorCode: "balance_exhausted", LastObservedAt: now,
	}})}
	service := newService(reader, repository, notifier, now)
	service.Quality = quality

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 || notifier.sent[0].key != "site:account:10:balance_exhausted" {
		t.Fatalf("balance incident = %#v", notifier.sent)
	}
	quality.result = qualityResult(now.Add(time.Minute), []accountquality.Account{{
		AccountID: 10, ModelID: "gpt", SampleCount: 1, SuccessCount: 0, SuccessRate: 0,
		LastResult: "account_test_error", LastErrorCode: "account_test_error", LastObservedAt: now.Add(time.Minute),
	}})
	service.Now = func() time.Time { return now.Add(time.Minute) }
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 {
		t.Fatalf("account test error must be silent, sent = %#v", notifier.sent)
	}
	if record := repository.records["site:account:10:balance_exhausted"]; record.State != "confirmed" {
		t.Fatalf("account test error must not recover balance incident: %#v", record)
	}
}

func TestServiceSkipsIndeterminateAccountQualityResults(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	for _, result := range []string{"account_test_error", "timeout", "http_error", "malformed_stream", "model_unavailable", "unknown"} {
		result := result
		t.Run(result, func(t *testing.T) {
			reader := &fakeReader{accounts: []sub2api.Account{{ID: 10, Name: "Current", Status: "active", Schedulable: true}}}
			repository := newMemoryRepository()
			repository.records["site:account:10:balance_exhausted"] = incidents.Record{Key: "site:account:10:balance_exhausted", Severity: "P1", State: "confirmed", SampleCount: 1}
			notifier := &fakeNotifier{}
			service := newService(reader, repository, notifier, now)
			service.Quality = &fakeQualitySource{result: qualityResult(now, []accountquality.Account{{
				AccountID: 10, SampleCount: 1, SuccessCount: 0, SuccessRate: 0,
				LastResult: result, LastErrorCode: result, LastObservedAt: now,
			}})}

			if err := service.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(notifier.sent) != 0 || repository.records["site:account:10:balance_exhausted"].State != "confirmed" {
				t.Fatalf("indeterminate quality must be silent and preserve incident: sent=%#v record=%#v", notifier.sent, repository.records["site:account:10:balance_exhausted"])
			}
		})
	}
}

func TestServiceEvaluatesMultiplierWhenPatrolResultIsIndeterminate(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	for _, result := range []string{"account_test_error", "timeout"} {
		result := result
		t.Run(result, func(t *testing.T) {
			reader := &fakeReader{accounts: []sub2api.Account{{ID: 10, Name: "Current", Status: "active", Schedulable: true}}}
			repository := newMemoryRepository()
			notifier := &fakeNotifier{}
			quality := &fakeQualitySource{result: qualityResult(now, []accountquality.Account{{
				AccountID: 10, ModelID: "gpt", SampleCount: 1, SuccessCount: 1, SuccessRate: 1,
				TTFTP50MS: number(100), TTFTP95MS: number(100), LastResult: "passed", LastObservedAt: now,
			}})}
			multipliers := &stubMultiplierSource{projection: multiplierProjection(10, number(0.05), "ok")}
			service := newService(reader, repository, notifier, now)
			service.Quality = quality
			service.Multipliers = multipliers
			if err := service.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			multipliers.projection = multiplierProjection(10, number(0.10), "ok")
			quality.result = qualityResult(now.Add(time.Minute), []accountquality.Account{{
				AccountID: 10, SampleCount: 1, SuccessCount: 0, SuccessRate: 0,
				LastResult: result, LastErrorCode: result, LastObservedAt: now.Add(time.Minute),
			}})
			service.Now = func() time.Time { return now.Add(time.Minute) }
			if err := service.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(notifier.sent) != 1 || notifier.sent[0].key != "site:account:10:multiplier" {
				t.Fatalf("multiplier change must alert independently: %#v", notifier.sent)
			}
			if _, found := repository.records["site:account:10:balance_exhausted"]; found {
				t.Fatalf("indeterminate patrol must not transition balance: %#v", repository.records)
			}
		})
	}
}

func TestServiceConfirmsTTFTP95RegressionAfterTwoQualifyingWindows(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{
		groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}},
		ops: map[snapshotKey]sub2api.OpsSnapshot{
			{groupID: 2, rangeName: "15m"}: snapshot(20, 20, 0.01, 3900),
			{groupID: 2, rangeName: "24h"}: snapshot(500, 490, 0.02, 3000),
		},
	}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)

	for range 2 {
		if err := service.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if len(notifier.sent) != 1 || notifier.sent[0].key != "site:group:2:ttft_p95" {
		t.Fatalf("TTFT regression notification = %#v", notifier.sent)
	}
}

func TestServiceRequiresBothTTFTP95Thresholds(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name        string
		windowP95   float64
		baselineP95 float64
		wantAlerts  int
	}{
		{name: "absolute_only", windowP95: 3500, baselineP95: 3000, wantAlerts: 0},
		{name: "relative_only", windowP95: 3000, baselineP95: 2000, wantAlerts: 0},
		{name: "both_thresholds", windowP95: 3900, baselineP95: 3000, wantAlerts: 1},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			reader := &fakeReader{
				groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}},
				ops: map[snapshotKey]sub2api.OpsSnapshot{
					{groupID: 2, rangeName: "15m"}: snapshot(20, 20, 0.01, test.windowP95),
					{groupID: 2, rangeName: "24h"}: snapshot(500, 490, 0.02, test.baselineP95),
				},
			}
			notifier := &fakeNotifier{}
			service := newService(reader, newMemoryRepository(), notifier, now)
			for range 2 {
				if err := service.Run(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if len(notifier.sent) != test.wantAlerts {
				t.Fatalf("alerts=%#v", notifier.sent)
			}
		})
	}
}

func TestServiceAlertsCompleteFailureImmediatelyBelowMinimumSample(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{
		groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}},
		ops: map[snapshotKey]sub2api.OpsSnapshot{
			{groupID: 2, rangeName: "15m"}: snapshot(3, 0, 1, 0),
			{groupID: 2, rangeName: "24h"}: snapshot(500, 490, 0.02, 1000),
		},
	}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 1 || notifier.sent[0].key != "site:group:2:availability" {
		t.Fatalf("complete failure must emit only its parent P0: %#v", notifier.sent)
	}
	message := notifier.sent[0].message
	if message.Severity != "P0" || !strings.HasPrefix(message.Card.Header.Title.Content, "P0｜") {
		t.Fatalf("complete failure message severity=%q title=%q", message.Severity, message.Card.Header.Title.Content)
	}
}

func TestServiceOnlyUsesActiveSchedulableAccountsForQualityEvaluation(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{accounts: []sub2api.Account{
		{ID: 10, Name: "Current", Status: "active", Schedulable: true},
		{ID: 11, Name: "Inactive", Status: "inactive", Schedulable: true},
		{ID: 12, Name: "Paused", Status: "active", Schedulable: false},
	}}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)
	service.Quality = &fakeQualitySource{result: qualityResult(now, []accountquality.Account{
		{AccountID: 10, ModelID: "gpt", SampleCount: 1, SuccessCount: 0, SuccessRate: 0, LastResult: "balance_exhausted", LastErrorCode: "balance_exhausted", LastObservedAt: now},
	})}

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, found := repository.records["site:account:10:balance_exhausted"]; !found {
		t.Fatal("active schedulable account must be evaluated")
	}
	for _, id := range []int64{11, 12} {
		if _, found := repository.records[object{kind: "account", id: id}.key("balance_exhausted")]; found {
			t.Fatalf("non-schedulable account %d must not be evaluated for account quality", id)
		}
	}
}

func TestRenderSeparatesRuntimeAndMultiplierEvidenceInCardJSON(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	for name, message := range map[string]notify.FeishuMessage{
		"runtime":    render(object{kind: "group", id: 2}, "ttft_p95", "3900ms", "3000ms", 2, now, "confirmed"),
		"multiplier": render(object{kind: "account", id: 10}, "multiplier", "0.1x", "0.05x", 1, now, "confirmed"),
	} {
		data, err := message.CardJSON()
		if err != nil {
			t.Fatal(err)
		}
		card := string(data)
		if name == "runtime" {
			if !contains(card, "站内运行告警") || !contains(card, "数据来源：Sub2API 原生站内运行快照") || contains(card, "账号质量定时巡检") {
				t.Fatalf("runtime card = %s", card)
			}
			continue
		}
		if !contains(card, "账号计费倍率变更") || !contains(card, "数据来源：Sub2API 原生账号监控倍率投影") || contains(card, "账号质量定时巡检结果") {
			t.Fatalf("multiplier card = %s", card)
		}
	}
}

func TestRenderRecoveryUsesBusinessLanguageAndStructuredMetrics(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		message notify.FeishuMessage
		wants   []string
		forbids []string
	}{
		{
			name: "runtime scheduling",
			message: render(object{kind: "account", id: 10, name: "特惠-SHUAI"},
				"paused", "active && schedulable", "active && schedulable", 1, now, "recovered"),
			wants:   []string{"已恢复正常调度", "当前状态", "正常调度", "1 个完整窗口", "Sub2API 原生账号调度状态"},
			forbids: []string{"**恢复结果**", "指标：paused", "active && schedulable", "15 分钟 / 24 小时"},
		},
		{
			name: "account balance",
			message: render(object{kind: "account", id: 10, name: "特惠-SHUAI"},
				"balance_exhausted", "passed", "not_balance_exhausted", 1, now, "recovered"),
			wants:   []string{"余额状态已恢复", "余额可用", "余额耗尽", "账号质量定时巡检结果"},
			forbids: []string{"balance_exhausted", "not_balance_exhausted"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := tt.message.RenderedText()
			for _, want := range tt.wants {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %q in %q", want, text)
				}
			}
			for _, forbidden := range tt.forbids {
				if strings.Contains(text, forbidden) {
					t.Fatalf("found %q in %q", forbidden, text)
				}
			}
		})
	}
}

func TestServiceSkipsStaleAccountQualityEvidence(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	stale := now.Add(-21 * time.Minute)
	reader := &fakeReader{accounts: []sub2api.Account{{ID: 10, Name: "Current", Status: "active", Schedulable: true}}}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)
	service.Quality = &fakeQualitySource{result: qualityResult(stale, []accountquality.Account{{
		AccountID: 10, SampleCount: 1, SuccessCount: 0, SuccessRate: 0,
		LastResult: "balance_exhausted", LastErrorCode: "balance_exhausted", LastObservedAt: stale,
	}})}

	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 0 || len(repository.records) != 0 {
		t.Fatalf("stale quality must be silent: sent=%#v records=%#v", notifier.sent, repository.records)
	}
}

func TestServiceRecoversAfterOneHealthyCompleteWindow(t *testing.T) {
	now := time.Date(2026, 7, 23, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{
		groups: []sub2api.Group{{ID: 2, Name: "Public", Status: "active"}},
		ops: map[snapshotKey]sub2api.OpsSnapshot{
			{groupID: 2, rangeName: "15m"}: snapshot(20, 18, 0.06, 1200),
			{groupID: 2, rangeName: "24h"}: snapshot(500, 490, 0.02, 1000),
		},
	}
	repository := newMemoryRepository()
	notifier := &fakeNotifier{}
	service := newService(reader, repository, notifier, now)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	reader.ops[snapshotKey{groupID: 2, rangeName: "15m"}] = snapshot(20, 20, 0.01, 1200)
	if err := service.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.sent) != 2 || notifier.sent[1].key != "site:group:2:error_rate" || !contains(notifier.sent[1].message.RenderedText(), "已恢复") {
		t.Fatalf("sent = %#v", notifier.sent)
	}
	if record := repository.records["site:group:2:error_rate"]; record.State != "recovered" {
		t.Fatalf("record = %#v", record)
	}
}

type stubMultiplierSource struct {
	projection sub2api.AccountMonitorProjection
	err        error
}

func (s stubMultiplierSource) ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error) {
	return s.projection, s.err
}

func multiplierProjection(accountID int64, value *float64, status string) sub2api.AccountMonitorProjection {
	return sub2api.AccountMonitorProjection{
		SchemaVersion: 2,
		Accounts: []sub2api.AccountMonitorAccount{{
			AccountID: accountID, Name: "Pro-SHEN-0.16",
			Multiplier: sub2api.AccountMonitorMultiplier{Value: value, Source: "declared", Status: status},
		}},
	}
}

func newTestSiteMonitor(t *testing.T, source MultiplierSource) (Service, *memoryRepository) {
	t.Helper()
	now := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	reader := &fakeReader{accounts: []sub2api.Account{{ID: 26, Name: "Pro-SHEN-0.16", Status: "active", Schedulable: true}}}
	repository := newMemoryRepository()
	service := newService(reader, repository, &fakeNotifier{}, now)
	service.Multipliers = source
	return service, repository
}

func TestEvaluateMultiplierUsesTrustworthyValue(t *testing.T) {
	value := 0.16
	source := stubMultiplierSource{projection: multiplierProjection(26, &value, "ok")}
	service, repo := newTestSiteMonitor(t, source)

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	record, found, err := repo.Get(context.Background(), "site:account:26:multiplier_baseline")
	if err != nil || !found {
		t.Fatalf("baseline not created: found=%v err=%v", found, err)
	}
	if record.CurrentValue != "0.16x" {
		t.Fatalf("CurrentValue = %q, want 0.16x（必须是可信倍率而非废弃的 1x）", record.CurrentValue)
	}
}

func TestEvaluateMultiplierSkipsUntrustworthyStatus(t *testing.T) {
	for _, status := range []string{"failed", "stale", "unsupported", "unavailable"} {
		t.Run(status, func(t *testing.T) {
			source := stubMultiplierSource{projection: multiplierProjection(26, nil, status)}
			service, repo := newTestSiteMonitor(t, source)

			if err := service.Run(context.Background()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if _, found, _ := repo.Get(context.Background(), "site:account:26:multiplier_baseline"); found {
				t.Fatalf("status=%s 时不得建立基线（拿不到值不是变更事件）", status)
			}
		})
	}
}

func TestEvaluateMultiplierSkipsNonPositiveValue(t *testing.T) {
	zero := 0.0
	source := stubMultiplierSource{projection: multiplierProjection(26, &zero, "ok")}
	service, repo := newTestSiteMonitor(t, source)

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, found, _ := repo.Get(context.Background(), "site:account:26:multiplier_baseline"); found {
		t.Fatal("倍率 0 是坏数据，不得建立基线")
	}
}

func TestEvaluateMultiplierUsesUpstreamPricingFallbackWhenUnmeasurable(t *testing.T) {
	// measured/failed：Sub2API 自己测不出，靠上游定价兜底建立基线，
	// 这样上游调价同样会触发倍率变化告警。
	source := stubMultiplierSource{projection: multiplierProjection(26, nil, "failed")}
	service, repo := newTestSiteMonitor(t, source)
	service.Fallback = func(_ context.Context, name string) *float64 {
		if name != "Pro-SHEN-0.16" {
			return nil
		}
		value := 0.17
		return &value
	}

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	record, found, err := repo.Get(context.Background(), "site:account:26:multiplier_baseline")
	if err != nil || !found {
		t.Fatalf("baseline not created from fallback: found=%v err=%v", found, err)
	}
	if record.CurrentValue != "0.17x" {
		t.Fatalf("CurrentValue = %q, want 0.17x（上游定价兜底值）", record.CurrentValue)
	}
}

func TestEvaluateMultiplierRecoveryNamesUpstreamPricingFallbackSource(t *testing.T) {
	source := &stubMultiplierSource{projection: multiplierProjection(26, nil, "failed")}
	service, _ := newTestSiteMonitor(t, source)
	notifier := service.Notifier.(*fakeNotifier)
	fallbackValue := 0.17
	service.Fallback = func(context.Context, string) *float64 { return &fallbackValue }

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("baseline run: %v", err)
	}
	fallbackValue = 0.19
	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("alert run: %v", err)
	}
	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("recovery run: %v", err)
	}
	if len(notifier.sent) != 2 {
		t.Fatalf("sent=%#v", notifier.sent)
	}
	recoveryText := notifier.sent[1].message.RenderedText()
	if !strings.Contains(recoveryText, "上游公开定价只读结果") || strings.Contains(recoveryText, "账号监控倍率投影") {
		t.Fatalf("recovery source=%q", recoveryText)
	}
}

func TestEvaluateMultiplierFallbackDoesNotOverrideTrustworthy(t *testing.T) {
	value := 0.16
	source := stubMultiplierSource{projection: multiplierProjection(26, &value, "ok")}
	service, repo := newTestSiteMonitor(t, source)
	service.Fallback = func(context.Context, string) *float64 {
		wrong := 9.99
		return &wrong
	}

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	record, found, err := repo.Get(context.Background(), "site:account:26:multiplier_baseline")
	if err != nil || !found {
		t.Fatalf("baseline not created: found=%v err=%v", found, err)
	}
	if record.CurrentValue != "0.16x" {
		t.Fatalf("CurrentValue = %q, want 0.16x（可信值优先于兜底）", record.CurrentValue)
	}
}

func TestEvaluateMultiplierFallbackFailureStaysSilent(t *testing.T) {
	source := stubMultiplierSource{projection: multiplierProjection(26, nil, "failed")}
	service, repo := newTestSiteMonitor(t, source)
	service.Fallback = func(context.Context, string) *float64 { return nil }

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, found, _ := repo.Get(context.Background(), "site:account:26:multiplier_baseline"); found {
		t.Fatal("兜底也拿不到值时不得建立基线")
	}
}

func TestEvaluateMultiplierSilentWhenSourceFails(t *testing.T) {
	source := stubMultiplierSource{err: errors.New("unavailable")}
	service, repo := newTestSiteMonitor(t, source)

	if err := service.Run(context.Background()); err != nil {
		t.Fatalf("倍率源故障不得让整个巡检失败: %v", err)
	}
	if _, found, _ := repo.Get(context.Background(), "site:account:26:multiplier_baseline"); found {
		t.Fatal("读取失败时不得建立基线")
	}
}

func newService(reader *fakeReader, repository *memoryRepository, notifier *fakeNotifier, now time.Time) Service {
	return Service{
		Reader:    reader,
		Incidents: &incidents.Machine{Repository: repository, Policy: incidents.DefaultPolicy()},
		Notifier:  notifier,
		Now:       func() time.Time { return now },
	}
}

type snapshotKey struct {
	groupID, accountID int64
	rangeName          string
}

type fakeReader struct {
	groups      []sub2api.Group
	accounts    []sub2api.Account
	accountRows []sub2api.Account
	ops         map[snapshotKey]sub2api.OpsSnapshot
	writeCalls  int
}

func (f *fakeReader) ListGroups(context.Context) ([]sub2api.Group, error) { return f.groups, nil }
func (f *fakeReader) ListAccounts(context.Context) ([]sub2api.Account, error) {
	if f.accounts != nil {
		return f.accounts, nil
	}
	return f.accountRows, nil
}
func (f *fakeReader) GetOpsSnapshot(_ context.Context, query sub2api.OpsQuery) (sub2api.OpsSnapshot, error) {
	return f.ops[snapshotKey{groupID: query.GroupID, accountID: query.AccountID, rangeName: query.TimeRange}], nil
}

type fakeQualitySource struct{ result accountquality.Result }

func (f *fakeQualitySource) Read(time.Time) (accountquality.Result, error) { return f.result, nil }

type memoryRepository struct{ records map[string]incidents.Record }

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{records: map[string]incidents.Record{}}
}
func (m *memoryRepository) Get(_ context.Context, key string) (incidents.Record, bool, error) {
	value, ok := m.records[key]
	return value, ok, nil
}
func (m *memoryRepository) Put(_ context.Context, value incidents.Record) error {
	m.records[value.Key] = value
	return nil
}

type sentMessage struct {
	key     string
	message notify.FeishuMessage
}
type fakeNotifier struct{ sent []sentMessage }

func (f *fakeNotifier) SendIncident(_ context.Context, key, _ string, message notify.FeishuMessage) error {
	f.sent = append(f.sent, sentMessage{key: key, message: message})
	return nil
}

func snapshot(requests, successes int64, errorRate, ttftP95 float64) sub2api.OpsSnapshot {
	return sub2api.OpsSnapshot{Overview: sub2api.OpsOverview{RequestCountTotal: requests, SuccessCount: successes, ErrorRate: errorRate, TTFT: sub2api.Percentiles{P95MS: ttftP95}}}
}
func qualityResult(now time.Time, accounts []accountquality.Account) accountquality.Result {
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.AccountID)
	}
	hash, err := accountquality.CanonicalAccountSetSHA256(ids)
	if err != nil {
		panic(err)
	}
	return accountquality.Result{SchemaVersion: 1, SnapshotID: "quality", ObservedAt: now, AccountSetSHA256: hash, Accounts: accounts}
}
func number(value float64) *float64 { return &value }
func contains(value, fragment string) bool {
	return len(fragment) == 0 || (len(value) >= len(fragment) && stringContains(value, fragment))
}
func stringContains(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
