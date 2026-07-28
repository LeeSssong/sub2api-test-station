package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/incidents"
	"example.invalid/relay-ops-service/internal/notify"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type fakeGroupReader struct {
	projection      sub2api.AccountMonitorProjection
	err             error
	histories       map[int64][]sub2api.AccountMonitorHistoryEntry
	historyLimits   []int
	historyAccounts []int64
}

func (f *fakeGroupReader) ListAccountMonitors(context.Context) (sub2api.AccountMonitorProjection, error) {
	return f.projection, f.err
}

func (f *fakeGroupReader) ListAccountMonitorHistory(_ context.Context, accountID int64, limit int) ([]sub2api.AccountMonitorHistoryEntry, error) {
	f.historyLimits = append(f.historyLimits, limit)
	f.historyAccounts = append(f.historyAccounts, accountID)
	return f.histories[accountID], nil
}

type memoryIncidentRepo struct{ records map[string]incidents.Record }

func (r *memoryIncidentRepo) Get(_ context.Context, key string) (incidents.Record, bool, error) {
	record, ok := r.records[key]
	return record, ok, nil
}

func (r *memoryIncidentRepo) Put(_ context.Context, record incidents.Record) error {
	r.records[record.Key] = record
	return nil
}

type groupDelivery struct {
	Key      string
	Evidence string
	Message  notify.FeishuMessage
}

// dedupingSender mimics occurrence-aware DeliverySender semantics. A
// successfully delivered key+occurrence+transition+evidence identity is never
// reused — a repeat is silently dropped
// (ReserveNotification returns reserved=false, SendIncident returns nil).
// Without this behavior the test cannot catch the "alert fires only once"
// bug, because the state machine side would happily keep returning Notify.
type dedupingSender struct {
	seen      map[string]bool
	delivered []groupDelivery
	failKeys  map[string]bool
}

func (s *dedupingSender) SendIncident(_ context.Context, key, evidence string, message notify.FeishuMessage) error {
	if s.failKeys[key] {
		return fmt.Errorf("feishu send failed for %s", key)
	}
	dedup := fmt.Sprintf("%s\x00%d\x00%s\x00%s", key, message.OccurrenceNo, message.Transition, evidence)
	if s.seen[dedup] {
		return nil
	}
	s.seen[dedup] = true
	s.delivered = append(s.delivered, groupDelivery{Key: key, Evidence: evidence, Message: message})
	return nil
}

func groupProjection(accounts ...sub2api.AccountMonitorAccount) sub2api.AccountMonitorProjection {
	return sub2api.AccountMonitorProjection{
		SchemaVersion: 2,
		Settings:      sub2api.AccountMonitorSettings{IntervalSeconds: 300},
		Accounts:      accounts,
	}
}

func downAccount(id int64, name, group string) sub2api.AccountMonitorAccount {
	return sub2api.AccountMonitorAccount{
		AccountID: id, Name: name, GroupIDs: []int64{6}, GroupNames: []string{group},
		SampleCount: 20, SuccessRate: 0, ErrorCode: "balance_exhausted",
	}
}

func healthyAccount(id int64, name, group string) sub2api.AccountMonitorAccount {
	return sub2api.AccountMonitorAccount{
		AccountID: id, Name: name, GroupIDs: []int64{6}, GroupNames: []string{group},
		SampleCount: 20, SuccessRate: 1,
	}
}

func cardTitle(t *testing.T, message notify.FeishuMessage) string {
	t.Helper()
	data, err := message.CardJSON()
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// 三阶段回归：故障 → 恢复 → 再次故障，红、绿、红三张卡都必须真正投递出去。
// 两次故障证据应稳定相同，由 occurrence 1/2 区分事件轮次。
func TestRunGroupAvailabilityDeliversAcrossOutageRecoveryOutage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	reader := &fakeGroupReader{}
	machine := incidents.Machine{
		Repository: &memoryIncidentRepo{records: map[string]incidents.Record{}},
		Policy:     incidents.DefaultPolicy(),
	}
	sender := &dedupingSender{seen: map[string]bool{}, failKeys: map[string]bool{}}

	day1 := time.Date(2026, 7, 26, 2, 0, 0, 0, time.UTC)
	day2 := day1.Add(24 * time.Hour)
	down := groupProjection(downAccount(22, "Plus-XN-0.09", "GPT-Plus"))
	healthy := groupProjection(healthyAccount(22, "Plus-XN-0.09", "GPT-Plus"))

	run := func(projection sub2api.AccountMonitorProjection, now time.Time) {
		t.Helper()
		reader.projection = projection
		if err := runGroupAvailability(ctx, reader, machine, sender, loc, now); err != nil {
			t.Fatal(err)
		}
	}

	// outage #1: 0 capacity is P0 and confirms immediately
	run(down, day1)
	// recovery
	run(healthy, day1.Add(30*time.Minute))
	// outage #2 (next day) is a new occurrence with the same evidence
	run(down, day2)

	if len(sender.delivered) != 3 {
		t.Fatalf("delivered = %d, want 3（红、绿、红）: %+v", len(sender.delivered), sender.delivered)
	}
	first, recovery, second := sender.delivered[0], sender.delivered[1], sender.delivered[2]
	if !strings.Contains(cardTitle(t, first.Message), "分组可用账号不足") {
		t.Fatalf("第一张不是告警卡: %s", cardTitle(t, first.Message))
	}
	if !strings.Contains(cardTitle(t, recovery.Message), "分组可用性已恢复") {
		t.Fatalf("第二张不是恢复卡: %s", cardTitle(t, recovery.Message))
	}
	for _, want := range []string{"可用账号", "1 / 1", "可用性正常", "账号监控分组快照"} {
		if !strings.Contains(recovery.Message.RenderedText(), want) {
			t.Fatalf("恢复卡缺少 %q: %s", want, recovery.Message.RenderedText())
		}
	}
	if !strings.Contains(cardTitle(t, second.Message), "分组可用账号不足") {
		t.Fatalf("第三张不是告警卡: %s", cardTitle(t, second.Message))
	}
	if second.Evidence != first.Evidence {
		t.Fatalf("same capacity should keep stable evidence: first=%q second=%q", first.Evidence, second.Evidence)
	}
	if first.Message.OccurrenceNo != 1 || recovery.Message.OccurrenceNo != 1 || second.Message.OccurrenceNo != 2 {
		t.Fatalf("occurrences = %d, %d, %d", first.Message.OccurrenceNo, recovery.Message.OccurrenceNo, second.Message.OccurrenceNo)
	}
}

func TestRunGroupAvailabilityDeliversSecondFailureInsideSameHour(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	reader := &fakeGroupReader{}
	machine := incidents.Machine{
		Repository: &memoryIncidentRepo{records: map[string]incidents.Record{}},
		Policy:     incidents.DefaultPolicy(),
	}
	sender := &dedupingSender{seen: map[string]bool{}, failKeys: map[string]bool{}}

	day1 := time.Date(2026, 7, 26, 2, 0, 0, 0, time.UTC)
	down := groupProjection(downAccount(22, "Plus-XN-0.09", "GPT-Plus"))
	healthy := groupProjection(healthyAccount(22, "Plus-XN-0.09", "GPT-Plus"))

	for i, projection := range []sub2api.AccountMonitorProjection{down, healthy, down} {
		reader.projection = projection
		if err := runGroupAvailability(ctx, reader, machine, sender, loc, day1.Add(time.Duration(i)*5*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if len(sender.delivered) != 3 {
		t.Fatalf("delivered = %d, want 3: %+v", len(sender.delivered), sender.delivered)
	}
}

func TestRunGroupAvailabilityConfirmsPartialCapacityAsP1AfterTwoWindows(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 28, 4, 0, 0, 0, time.UTC)
	reader := &fakeGroupReader{projection: groupProjection(
		healthyAccount(20, "Plus-A", "GPT-Plus"),
		downAccount(21, "Plus-B", "GPT-Plus"),
		downAccount(22, "Plus-C", "GPT-Plus"),
	)}
	machine := incidents.Machine{
		Repository: &memoryIncidentRepo{records: map[string]incidents.Record{}},
		Policy:     incidents.DefaultPolicy(),
	}
	sender := &dedupingSender{seen: map[string]bool{}, failKeys: map[string]bool{}}
	for index := 0; index < 2; index++ {
		if err := runGroupAvailability(context.Background(), reader, machine, sender, nil, now.Add(time.Duration(index)*5*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if len(sender.delivered) != 1 {
		t.Fatalf("deliveries = %#v", sender.delivered)
	}
	message := sender.delivered[0].Message
	if message.Severity != "P1" || !strings.Contains(message.Card.Header.Title.Content, "P1｜") {
		t.Fatalf("message severity=%q title=%q", message.Severity, message.Card.Header.Title.Content)
	}
}

// 阻断项回归：告警作业必须按最近 1 小时滚动窗判定，而不是 projection 的
// 7 天累计口径。账号硬挂 1 小时后 7 天成功率仍 ≈98.6% 判健康，修复前
// 5 分钟一轮的作业对 http_error/timeout 类故障的实际响应时间是天级。
// 同时锁定 history 拉取条数与 1 小时窗匹配（300 秒间隔 → 18 条），不得
// 复用 48 小时口径的 HistoryLimitFor（692 条）。
func TestRunGroupAvailabilityJudgesByRollingHourWindow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, 7, 26, 2, 0, 0, 0, time.UTC)
	// projection 口径：7 天成功率 98.6%，修复前会判 Healthy 永不告警
	stale := sub2api.AccountMonitorAccount{
		AccountID: 22, Name: "Plus-XN-0.09", GroupIDs: []int64{6}, GroupNames: []string{"GPT-Plus"},
		SampleCount: 2016, SuccessRate: 0.986, ErrorCode: "http_error",
	}
	entries := make([]sub2api.AccountMonitorHistoryEntry, 0, 12)
	for i := 0; i < 12; i++ {
		entries = append(entries, sub2api.AccountMonitorHistoryEntry{
			AccountID: 22, Status: "failed", ErrorCode: "http_error",
			CheckedAt: now.Add(-time.Duration(i)*5*time.Minute - time.Minute),
		})
	}
	reader := &fakeGroupReader{
		projection: groupProjection(stale),
		histories:  map[int64][]sub2api.AccountMonitorHistoryEntry{22: entries},
	}
	machine := incidents.Machine{
		Repository: &memoryIncidentRepo{records: map[string]incidents.Record{}},
		Policy:     incidents.DefaultPolicy(),
	}
	sender := &dedupingSender{seen: map[string]bool{}, failKeys: map[string]bool{}}

	// P1 需要 2 个确认窗口
	for i := 0; i < 2; i++ {
		reader.histories = map[int64][]sub2api.AccountMonitorHistoryEntry{22: entries}
		if err := runGroupAvailability(ctx, reader, machine, sender, loc, now.Add(time.Duration(i)*5*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if len(sender.delivered) != 1 {
		t.Fatalf("1 小时窗内全失败必须触发告警: %+v", sender.delivered)
	}
	if title := cardTitle(t, sender.delivered[0].Message); !strings.Contains(title, "分组可用账号不足") {
		t.Fatalf("不是告警卡: %s", title)
	}
	if len(reader.historyLimits) == 0 {
		t.Fatal("告警作业必须拉取账号 history")
	}
	for _, limit := range reader.historyLimits {
		if limit != 18 {
			t.Fatalf("history limit = %d, want 18（ceil(3600/300)×1.5，48 小时口径的 692 条是 40 倍浪费）", limit)
		}
	}
}

func TestRunGroupAvailabilitySkipsRoundWhenMonitorReadFails(t *testing.T) {
	t.Parallel()

	reader := &fakeGroupReader{err: fmt.Errorf("monitor endpoint unavailable")}
	machine := incidents.Machine{
		Repository: &memoryIncidentRepo{records: map[string]incidents.Record{}},
		Policy:     incidents.DefaultPolicy(),
	}
	sender := &dedupingSender{seen: map[string]bool{}, failKeys: map[string]bool{}}

	if err := runGroupAvailability(context.Background(), reader, machine, sender, nil, time.Now()); err != nil {
		t.Fatalf("监控读取失败必须静默跳过，err = %v", err)
	}
	if len(sender.delivered) != 0 {
		t.Fatalf("监控读取失败不得产生任何投递: %+v", sender.delivered)
	}
}

func TestRunGroupAvailabilityContinuesPastSingleGroupSendFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	reader := &fakeGroupReader{}
	machine := incidents.Machine{
		Repository: &memoryIncidentRepo{records: map[string]incidents.Record{}},
		Policy:     incidents.DefaultPolicy(),
	}
	sender := &dedupingSender{
		seen:     map[string]bool{},
		failKeys: map[string]bool{"group:GPT-Plus:availability": true},
	}

	// 两个分组同时故障；按分组名排序 GPT-Plus 先投且失败，GPT-Pro 不得被挡住。
	both := groupProjection(
		downAccount(22, "Plus-XN-0.09", "GPT-Plus"),
		sub2api.AccountMonitorAccount{
			AccountID: 26, Name: "Pro-SHEN-0.16", GroupIDs: []int64{7}, GroupNames: []string{"GPT-Pro"},
			SampleCount: 20, SuccessRate: 0, ErrorCode: "balance_exhausted",
		},
	)
	now := time.Date(2026, 7, 26, 2, 0, 0, 0, time.UTC)
	reader.projection = both
	err := runGroupAvailability(ctx, reader, machine, sender, loc, now)
	if err == nil || !strings.Contains(err.Error(), "GPT-Plus") {
		t.Fatalf("单分组投递失败必须上报错误: %v", err)
	}
	if len(sender.delivered) != 1 || sender.delivered[0].Key != "group:GPT-Pro:availability" {
		t.Fatalf("失败分组不得挡住其余分组: %+v", sender.delivered)
	}
}
