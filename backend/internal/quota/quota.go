// Package quota 实现重置配额：规则匹配（用户+类型 > 用户 > 全局+类型 > 全局）、
// 周期计数（预占/回滚）与上游冷却记忆。
package quota

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"zai-zcode-reset/internal/model"
	"zai-zcode-reset/internal/store"
)

type Manager struct {
	store *store.Store

	mu        sync.Mutex
	cooldowns map[string]int64 // accountID|resetType -> 冷却到期毫秒
}

func NewManager(s *store.Store) *Manager {
	return &Manager{store: s, cooldowns: map[string]int64{}}
}

// PeriodKey 返回计数周期键：DAY=YYYY-MM-DD；WEEK=ISO 周 YYYY-Www。
func PeriodKey(period string, now time.Time) string {
	switch period {
	case model.PeriodWeek:
		year, week := now.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", year, week)
	default:
		return now.Format("2006-01-02")
	}
}

func counterKey(userID, resetType, period string, now time.Time) string {
	return strings.Join([]string{userID, resetType, period, PeriodKey(period, now)}, "|")
}

type ruleScore int

// specificity 越具体的规则分数越高：用户+类型(3) > 用户+ALL(2) > 全局+类型(1) > 全局+ALL(0)。
func specificity(r model.Rule) ruleScore {
	s := 0
	if r.UserID != "" {
		s += 2
	}
	if r.ResetType != model.ResetAll {
		s++
	}
	return ruleScore(s)
}

// EffectiveLimit 计算某用户某重置类型在给定周期的生效上限。
// 没有任何匹配规则时上限为 0（默认拒绝，管理员显式配置才放行）。
func (m *Manager) EffectiveLimit(userID, resetType, period string) int {
	rules := m.store.ListRules()
	best := -1
	bestScore := ruleScore(-1)
	for _, r := range rules {
		if !r.Enabled || r.Period != period {
			continue
		}
		if r.ResetType != model.ResetAll && r.ResetType != resetType {
			continue
		}
		if r.UserID != "" && r.UserID != userID {
			continue
		}
		if s := specificity(r); s > bestScore || (s == bestScore && r.MaxCount > best) {
			bestScore = s
			best = r.MaxCount
		}
	}
	if best < 0 {
		return 0
	}
	return best
}

// Usage 返回某用户某类型在 DAY/WEEK 两个周期的（已用, 上限）。
type PeriodUsage struct {
	DayUsed, DayLimit   int
	WeekUsed, WeekLimit int
}

func (m *Manager) Usage(userID, resetType string) PeriodUsage {
	return PeriodUsage{
		DayUsed:   m.store.GetCounter(counterKey(userID, resetType, model.PeriodDay, time.Now())),
		DayLimit:  m.EffectiveLimit(userID, resetType, model.PeriodDay),
		WeekUsed:  m.store.GetCounter(counterKey(userID, resetType, model.PeriodWeek, time.Now())),
		WeekLimit: m.EffectiveLimit(userID, resetType, model.PeriodWeek),
	}
}

// Reserve 预占一次配额：两个周期任一超额即拒绝（ErrLimitReached）。
// 成功后立即落盘计数；上游失败时调用方必须 Rollback。
func (m *Manager) Reserve(userID, resetType string) error {
	now := time.Now()
	dayKey := counterKey(userID, resetType, model.PeriodDay, now)
	weekKey := counterKey(userID, resetType, model.PeriodWeek, now)
	if d := m.store.GetCounter(dayKey); d >= m.EffectiveLimit(userID, resetType, model.PeriodDay) {
		return &LimitError{Period: model.PeriodDay}
	}
	if w := m.store.GetCounter(weekKey); w >= m.EffectiveLimit(userID, resetType, model.PeriodWeek) {
		return &LimitError{Period: model.PeriodWeek}
	}
	if err := m.store.AdjustCounter(dayKey, 1); err != nil {
		return err
	}
	if err := m.store.AdjustCounter(weekKey, 1); err != nil {
		_ = m.store.AdjustCounter(dayKey, -1)
		return err
	}
	return nil
}

// Rollback 回滚一次预占（上游执行失败时调用）。
func (m *Manager) Rollback(userID, resetType string) {
	now := time.Now()
	_ = m.store.AdjustCounter(counterKey(userID, resetType, model.PeriodDay, now), -1)
	_ = m.store.AdjustCounter(counterKey(userID, resetType, model.PeriodWeek, now), -1)
}

// LimitError 表示配额已用尽。
type LimitError struct{ Period string }

func (e *LimitError) Error() string {
	if e.Period == model.PeriodWeek {
		return "本周重置次数已用完"
	}
	return "今日重置次数已用完"
}

// ---------- 上游冷却 ----------

func cooldownKey(accountID, resetType string) string {
	return accountID + "|" + resetType
}

// CooldownUntil 返回该账号+类型的冷却到期毫秒；0 表示无冷却。
func (m *Manager) CooldownUntil(accountID, resetType string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cooldowns[cooldownKey(accountID, resetType)]
}

// SetCooldown 记录上游给出的冷却边界（3301 next_try_at 或 429 退避）。
func (m *Manager) SetCooldown(accountID, resetType string, untilMs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if untilMs > m.cooldowns[cooldownKey(accountID, resetType)] {
		m.cooldowns[cooldownKey(accountID, resetType)] = untilMs
	}
}

// SortRules 是给管理后台的稳定排序：全局规则在前、类型字母序。
func SortRules(rules []model.Rule) {
	sort.SliceStable(rules, func(i, j int) bool {
		if (rules[i].UserID == "") != (rules[j].UserID == "") {
			return rules[i].UserID == ""
		}
		return rules[i].ResetType < rules[j].ResetType
	})
}
