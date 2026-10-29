// Package autoreset 实现「重置卡到期自动重置」：已申领的重置机会（卡）进入
// expire_at − lead 触发窗口后，若对应窗口的剩余占比低于阈值（额度确实被
// 消耗过，重置有收益），自动调 use 消耗掉，避免机会白白作废。
// 不占用平台配额、不写全局冷却、绝不申领新机会（只抢救已有的卡）。
package autoreset

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"zai-zcode-reset/internal/cryptoutil"
	"zai-zcode-reset/internal/model"
	"zai-zcode-reset/internal/store"
	"zai-zcode-reset/internal/upstream"
)

const (
	tickInterval = 30 * time.Second

	// quota/limit 中两类重置卡对应窗口的 unit 编码（见 UsageLimit 契约注释）。
	usageUnitFiveHour = 3
	usageUnitWeek     = 6

	// errLogInterval 账号级错误（解密/status/用量失败）落 reset 日志的限频。
	errLogInterval = time.Hour

	// stateTTL 卡状态条目过期后的保留时长，超时清理防内存膨胀。
	stateTTL = 24 * time.Hour
)

// ClientFactory 从上游账号构造客户端；生产走令牌解密，测试注入 mock。
type ClientFactory func(model.UpstreamAccount) (*upstream.Client, error)

type Runner struct {
	store     *store.Store
	box       *cryptoutil.SecretBox
	mock      bool
	newClient ClientFactory

	mu         sync.Mutex
	cards      map[string]*cardState
	lastErrLog map[string]time.Time // accountID -> 上次账号级错误落日志时间
}

// cardState 是单张卡的自动处理状态，键为 accountID|resetType|expireAt。
// 调度状态只在内存：重启后按 status 实况重建，已消耗的卡不会重复处理。
type cardState struct {
	UseKey    string // 同一张卡的所有重试复用同一幂等键
	ExpireAt  int64
	AccountID string
	Attempts  int
	Done      bool
}

func NewRunner(st *store.Store, box *cryptoutil.SecretBox, mock bool) *Runner {
	r := &Runner{
		store:      st,
		box:        box,
		mock:       mock,
		cards:      map[string]*cardState{},
		lastErrLog: map[string]time.Time{},
	}
	r.newClient = func(a model.UpstreamAccount) (*upstream.Client, error) {
		return upstream.AccountClient(a, box, mock)
	}
	return r
}

// Run 阻塞运行直到 ctx 取消；随进程优雅退出由 main 控制。
func (r *Runner) Run(ctx context.Context) {
	r.tick(ctx) // 启动即扫一次：重启落在触发窗口内的卡尽快补上
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.tick(ctx)
		}
	}
}

func (r *Runner) tick(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("自动重置：tick 异常退出 %v", rec)
		}
	}()
	global := r.store.GetAutoResetConfig()
	nowMs := time.Now().UnixMilli()
	for _, account := range r.store.ListAccounts() {
		if ctx.Err() != nil {
			return
		}
		if !account.Enabled {
			continue
		}
		eff := EffectiveConfig(account.AutoReset, global)
		if !eff.Enabled {
			continue
		}
		r.processAccount(ctx, account, eff)
	}
	r.sweep(nowMs)
}

// EffectiveConfig 账号覆盖（整组）优先于全局默认。
func EffectiveConfig(override *model.AutoResetConfig, global model.AutoResetConfig) model.AutoResetConfig {
	if override != nil {
		return *override
	}
	return global
}

// InTriggerWindow 卡是否处于自动重置评估窗口：expire_at − lead ≤ now < expire_at。
func InTriggerWindow(expireAtMs, nowMs int64, leadMinutes int) bool {
	if expireAtMs <= nowMs {
		return false
	}
	return nowMs >= expireAtMs-int64(leadMinutes)*60_000
}

// ShouldAutoFire 对应窗口是否值得自动用卡：剩余占比（100 − 已用）必须低于阈值，
// 即额度确实被消耗过；剩余充足时重置无收益，让卡自然过期。
func ShouldAutoFire(usedPercent, thresholdRemaining int) bool {
	return 100-usedPercent < thresholdRemaining
}

type pendingCard struct {
	resetType string
	unit      int
	threshold int
	expireAt  int64
}

func (r *Runner) processAccount(ctx context.Context, account model.UpstreamAccount, eff model.AutoResetConfig) {
	client, err := r.newClient(account)
	if err != nil {
		r.logAccountError(account, "auto_client_error", "构造上游客户端失败: "+err.Error())
		return
	}
	st, err := client.Status(ctx)
	if err != nil {
		r.logAccountError(account, "auto_status_error", "查询重置状态失败: "+err.Error())
		return
	}
	nowMs := time.Now().UnixMilli()

	var pendings []pendingCard
	for _, t := range []struct {
		resetType string
		cards     []upstream.Opportunity
		unit      int
		threshold int
	}{
		{model.ResetFiveHour, st.AvailableFiveHour, usageUnitFiveHour, eff.FiveHourThreshold},
		{model.ResetWeek, st.AvailableWeek, usageUnitWeek, eff.WeekThreshold},
	} {
		for _, card := range t.cards {
			if !InTriggerWindow(card.ExpireAt, nowMs, eff.LeadMinutes) {
				continue
			}
			if r.isDone(cardKey(account.ID, t.resetType, card.ExpireAt)) {
				continue
			}
			pendings = append(pendings, pendingCard{resetType: t.resetType, unit: t.unit, threshold: t.threshold, expireAt: card.ExpireAt})
		}
	}
	if len(pendings) == 0 {
		return
	}

	// 用量只查一次，两类卡共用；拿不到用量时保守不动（下个 tick 重试）。
	ql, err := client.QuotaLimit(ctx)
	if err != nil {
		r.logAccountError(account, "auto_usage_error", "查询用量失败: "+err.Error())
		return
	}
	for _, p := range pendings {
		limit := upstream.FindUsageLimit(ql.Limits, "TOKENS_LIMIT", p.unit)
		if limit == nil {
			log.Printf("自动重置：账号 %s 缺少 unit=%d 用量桶，跳过 %s 卡", account.Name, p.unit, p.resetType)
			continue
		}
		if !ShouldAutoFire(limit.Percentage, p.threshold) {
			continue
		}
		r.fire(ctx, client, account, p.resetType, p.expireAt)
	}
}

// fire 消耗一张卡；失败在窗口内下个 tick 复用同一幂等键重试。
func (r *Runner) fire(ctx context.Context, client *upstream.Client, account model.UpstreamAccount, resetType string, expireAt int64) {
	state := r.state(cardKey(account.ID, resetType, expireAt), account.ID, expireAt)
	if state.Done {
		return
	}
	state.Attempts++
	err := client.Use(ctx, state.UseKey, resetType)
	if err == nil {
		state.Done = true
		r.logCard(account, resetType, true, "", "", 0)
		log.Printf("自动重置成功：账号 %s %s 卡（expire_at=%d）", account.Name, resetType, expireAt)
		return
	}
	var biz *upstream.BusinessError
	code := 0
	if errors.As(err, &biz) {
		code = biz.Code
	}
	if time.Now().UnixMilli() >= expireAt {
		state.Done = true
		r.logCard(account, resetType, false, "auto_expired", "卡已到期，自动重置最终失败: "+err.Error(), code)
		return
	}
	log.Printf("自动重置失败（窗口内将重试）：账号 %s %s 卡: %v", account.Name, resetType, err)
}

// sweep 收尾与清理：已过期且尝试过但未成功的卡记一条最终失败日志；
// 过期超过 stateTTL 的状态条目删除。
func (r *Runner) sweep(nowMs int64) {
	type finalizeJob struct {
		account   model.UpstreamAccount
		resetType string
	}
	var jobs []finalizeJob
	r.mu.Lock()
	for key, st := range r.cards {
		if st.ExpireAt+stateTTL.Milliseconds() < nowMs {
			delete(r.cards, key)
			continue
		}
		if !st.Done && st.Attempts > 0 && st.ExpireAt < nowMs {
			st.Done = true
			account, ok := r.store.GetAccount(st.AccountID)
			if !ok {
				continue
			}
			parts := strings.Split(key, "|")
			if len(parts) != 3 {
				continue
			}
			jobs = append(jobs, finalizeJob{account: account, resetType: parts[1]})
		}
	}
	r.mu.Unlock()
	for _, j := range jobs {
		r.logCard(j.account, j.resetType, false, "auto_expired", "卡已到期，自动重置未成功", 0)
	}
}

// ---------- 内部状态 ----------

func cardKey(accountID, resetType string, expireAt int64) string {
	return accountID + "|" + resetType + "|" + strconv.FormatInt(expireAt, 10)
}

func (r *Runner) state(key, accountID string, expireAt int64) *cardState {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.cards[key]
	if st == nil {
		st = &cardState{UseKey: upstream.NewIdempotencyKey(), ExpireAt: expireAt, AccountID: accountID}
		r.cards[key] = st
	}
	return st
}

func (r *Runner) isDone(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.cards[key]
	return st != nil && st.Done
}

// logCard 写一条自动重置日志；归属该账号绑定的用户（无绑定则留空）。
func (r *Runner) logCard(account model.UpstreamAccount, resetType string, success bool, kind, message string, upstreamCode int) {
	user, _ := r.store.FindBoundUser(account.ID)
	_ = r.store.AppendResetLog(model.ResetLog{
		At:              time.Now(),
		UserID:          user.ID,
		Username:        user.Username,
		AccountID:       account.ID,
		AccountName:     account.Name,
		ResetType:       resetType,
		Success:         success,
		Trigger:         model.TriggerAuto,
		UpstreamCode:    upstreamCode,
		UpstreamMessage: message,
		ErrorKind:       kind,
		IP:              "system",
	})
}

// logAccountError 账号级错误限频落日志（1 小时/账号），并始终写服务端日志。
func (r *Runner) logAccountError(account model.UpstreamAccount, kind, message string) {
	log.Printf("自动重置：%s（账号 %s）", message, account.Name)
	r.mu.Lock()
	if last, ok := r.lastErrLog[account.ID]; ok && time.Since(last) < errLogInterval {
		r.mu.Unlock()
		return
	}
	r.lastErrLog[account.ID] = time.Now()
	r.mu.Unlock()
	r.logCard(account, model.ResetAll, false, kind, message, 0)
}
