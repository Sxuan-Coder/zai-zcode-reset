package autoreset

import (
	"context"
	"testing"
	"time"

	"zai-zcode-reset/internal/model"
	"zai-zcode-reset/internal/store"
	"zai-zcode-reset/internal/upstream"
)

func TestEffectiveConfig(t *testing.T) {
	global := model.AutoResetConfig{Enabled: true, LeadMinutes: 10, FiveHourThreshold: 90, WeekThreshold: 80}
	if got := EffectiveConfig(nil, global); got != global {
		t.Fatalf("无覆盖时应使用全局配置: %+v", got)
	}
	override := &model.AutoResetConfig{Enabled: false, LeadMinutes: 5, FiveHourThreshold: 70, WeekThreshold: 60}
	got := EffectiveConfig(override, global)
	if *override != got {
		t.Fatalf("有覆盖时应整组使用覆盖配置: %+v", got)
	}
}

func TestShouldAutoFire(t *testing.T) {
	cases := []struct {
		used      int
		threshold int
		want      bool
	}{
		{used: 0, threshold: 90, want: false},  // 没用过：剩余 100，不动
		{used: 10, threshold: 90, want: false}, // 剩余 90 不低于 90，边界不动
		{used: 11, threshold: 90, want: true},  // 剩余 89 < 90，触发
		{used: 30, threshold: 50, want: false}, // 剩余 70 ≥ 50，不动
		{used: 50, threshold: 50, want: false},
		{used: 51, threshold: 50, want: true},
		{used: 99, threshold: 99, want: true},
	}
	for _, c := range cases {
		if got := ShouldAutoFire(c.used, c.threshold); got != c.want {
			t.Fatalf("已用 %d%%、阈值 %d%%：期望 %v，得到 %v", c.used, c.threshold, c.want, got)
		}
	}
}

func TestInTriggerWindow(t *testing.T) {
	now := time.Now().UnixMilli()
	min := int64(60_000)
	if InTriggerWindow(now, now, 10) {
		t.Fatalf("已过期的卡不应进窗口")
	}
	if InTriggerWindow(now+5*min, now, 10) == false {
		t.Fatalf("到期前 5 分钟、提前量 10 分钟应进窗口")
	}
	if InTriggerWindow(now+5*min, now, 1) {
		t.Fatalf("到期前 5 分钟、提前量 1 分钟不应进窗口")
	}
	if InTriggerWindow(now+10*min, now, 10) == false {
		t.Fatalf("恰好等于提前量边界应进窗口")
	}
}

// mockCreds 与 mock 上游交互统一使用，保证 runner 与测试共享同一 mock 状态。
var mockCreds = upstream.Credentials{ZCodeJWT: "mock-jwt", PlanToken: "mock-token"}

func newTestRunner(t *testing.T) (*Runner, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("打开存储失败: %v", err)
	}
	t.Cleanup(func() { st.Close() }) // Windows 下必须先关句柄，TempDir 才能清理
	r := NewRunner(st, nil, true)
	r.newClient = func(model.UpstreamAccount) (*upstream.Client, error) {
		return upstream.NewClient(mockCreds, true), nil
	}
	return r, st
}

func readAutoLogs(t *testing.T, st *store.Store) []model.ResetLog {
	t.Helper()
	logs, err := st.ReadResetLogs(1000)
	if err != nil {
		t.Fatalf("读取重置日志失败: %v", err)
	}
	return logs
}

// mock 上游契约：申领一次同时发两类卡各一张（30 分钟后过期）；
// 用量固定 five_hour 已用 30%、week 已用 12%。
func TestRunnerMockFire(t *testing.T) {
	r, st := newTestRunner(t)
	ctx := context.Background()
	acc, err := st.CreateAccount(model.UpstreamAccount{Name: "acc", Family: "bigmodel", TargetType: "PERSONAL", Enabled: true, IsDefault: true})
	if err != nil {
		t.Fatalf("创建账号失败: %v", err)
	}
	if err := st.SetAutoResetConfig(model.AutoResetConfig{Enabled: true, LeadMinutes: 60, FiveHourThreshold: 90, WeekThreshold: 90}); err != nil {
		t.Fatalf("写入配置失败: %v", err)
	}

	client := upstream.NewClient(mockCreds, true)
	if _, err := client.RequestOpportunity(ctx, upstream.NewIdempotencyKey()); err != nil {
		t.Fatalf("申领 mock 机会失败: %v", err)
	}

	r.tick(ctx)

	logs := readAutoLogs(t, st)
	if len(logs) != 2 {
		t.Fatalf("期望两类卡各自动重置一次（共 2 条日志），得到 %d 条", len(logs))
	}
	kinds := map[string]bool{}
	for _, l := range logs {
		if !l.Success || l.Trigger != model.TriggerAuto || l.AccountID != acc.ID {
			t.Fatalf("日志字段不符: %+v", l)
		}
		kinds[l.ResetType] = true
	}
	if !kinds[model.ResetFiveHour] || !kinds[model.ResetWeek] {
		t.Fatalf("期望 FIVE_HOUR 与 WEEK 各一条: %v", kinds)
	}

	// 再次申领；five_hour 阈值收紧到 50（剩余 70 ≥ 50 不触发），week 仍触发。
	if _, err := client.RequestOpportunity(ctx, upstream.NewIdempotencyKey()); err != nil {
		t.Fatalf("申领 mock 机会失败: %v", err)
	}
	if err := st.SetAutoResetConfig(model.AutoResetConfig{Enabled: true, LeadMinutes: 60, FiveHourThreshold: 50, WeekThreshold: 90}); err != nil {
		t.Fatalf("更新配置失败: %v", err)
	}
	r.tick(ctx)

	logs = readAutoLogs(t, st)
	if len(logs) != 3 {
		t.Fatalf("收紧阈值后只应再触发一次，共 3 条日志，得到 %d 条", len(logs))
	}
	if last := logs[len(logs)-1]; last.ResetType != model.ResetWeek {
		t.Fatalf("最后一条应为 WEEK 自动重置: %+v", last)
	}

	// 幂等：已消耗的卡不再处理，重复 tick 不产生新日志。
	r.tick(ctx)
	if logs = readAutoLogs(t, st); len(logs) != 3 {
		t.Fatalf("重复 tick 不应产生新日志，得到 %d 条", len(logs))
	}
}

func TestRunnerSweepExpired(t *testing.T) {
	r, st := newTestRunner(t)
	acc, err := st.CreateAccount(model.UpstreamAccount{Name: "acc", Family: "bigmodel", TargetType: "PERSONAL", Enabled: true})
	if err != nil {
		t.Fatalf("创建账号失败: %v", err)
	}
	expired := time.Now().Add(-time.Minute).UnixMilli()
	r.cards[cardKey(acc.ID, model.ResetFiveHour, expired)] = &cardState{
		UseKey: "k", ExpireAt: expired, AccountID: acc.ID, Attempts: 3,
	}
	r.tick(context.Background()) // 全局默认关闭，仅执行 sweep 收尾

	logs := readAutoLogs(t, st)
	if len(logs) != 1 || logs[0].ErrorKind != "auto_expired" || logs[0].Success || logs[0].Trigger != model.TriggerAuto {
		t.Fatalf("期望一条 auto_expired 失败日志: %+v", logs)
	}
	// 收尾后条目置 Done，重复 tick 不再落日志。
	r.tick(context.Background())
	if logs = readAutoLogs(t, st); len(logs) != 1 {
		t.Fatalf("收尾后的卡不应重复落日志，得到 %d 条", len(logs))
	}
}
