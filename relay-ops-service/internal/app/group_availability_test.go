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

// dedupingSender mimics DeliverySender + notification_deliveries semantics:
// dedup_key = digest(key + "\x00" + evidence) is UNIQUE, and a successfully
// delivered dedup key is never reused — a repeat is silently dropped
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
	dedup := key + "\x00" + evidence
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
// 投递层的 dedup_key 由 incident key + 证据哈希决定且永不复用；单账号分组每
// 次故障的可用比都是 0/1，若证据不含时间维度，第二次故障会撞上第一次的
// dedup_key 被静默丢弃——这正是「告警只会成功发出一次」的根因。两次故障
// 相隔 24 小时，跨小时桶，证据必须不同。
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

	// outage #1: P1 需要 2 个确认窗口，tick2 才 confirmed + Notify
	run(down, day1)
	if len(sender.delivered) != 0 {
		t.Fatalf("确认窗口未满就投递了: %+v", sender.delivered)
	}
	run(down, day1.Add(5*time.Minute))
	// recovery
	run(healthy, day1.Add(30*time.Minute))
	// outage #2（次日）：同样要走满确认窗口
	run(down, day2)
	run(down, day2.Add(5*time.Minute))

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
	// 第二次故障的证据必须与第一次不同，否则投递层 dedup 会把它静默吞掉。
	if second.Evidence == first.Evidence {
		t.Fatalf("跨天故障证据未变，第二次告警会被 dedup 吞掉: %q", first.Evidence)
	}
}

// 已知残留缺陷（不是设计意图）：同一小时桶内「故障→恢复→再故障」的第二次
// 故障红卡会被投递层 dedup 吞掉，静默窗口 <= 1 小时。根因是证据哈希语义
// 冲突——同一个 evidenceHash 既被状态机用来判断「取值是否变化」，又被投递
// 层用作「事件轮次幂等键」。防重复本来由状态机的转移判定和
// ConfirmationWindows: 2 负责，时间桶抑制掉的是全新事件。彻底修法是为投递
// 另行派生带事件轮次判别量的证据（需改 incidents 包暴露轮次）。本测试固化
// 当前真实行为，一旦根治后应改为期望 3 张卡。
func TestRunGroupAvailabilityHasKnownSameBucketSilenceWindow(t *testing.T) {
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

	for i, projection := range []sub2api.AccountMonitorProjection{down, down, healthy, down, down} {
		reader.projection = projection
		if err := runGroupAvailability(ctx, reader, machine, sender, loc, day1.Add(time.Duration(i)*5*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	// 五轮全部落在 10:00–10:20（Asia/Shanghai）同一小时桶内：红、绿之后，
	// 同桶同比值的第二次故障被投递层 dedup 拦下 —— 共 2 张（缺陷现状）。
	if len(sender.delivered) != 2 {
		t.Fatalf("delivered = %d, want 2: %+v", len(sender.delivered), sender.delivered)
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
	if err := runGroupAvailability(ctx, reader, machine, sender, loc, now); err != nil {
		t.Fatal(err)
	}
	err := runGroupAvailability(ctx, reader, machine, sender, loc, now.Add(5*time.Minute))
	if err == nil || !strings.Contains(err.Error(), "GPT-Plus") {
		t.Fatalf("单分组投递失败必须上报错误: %v", err)
	}
	if len(sender.delivered) != 1 || sender.delivered[0].Key != "group:GPT-Pro:availability" {
		t.Fatalf("失败分组不得挡住其余分组: %+v", sender.delivered)
	}
}
