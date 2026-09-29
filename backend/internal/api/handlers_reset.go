package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"zai-zcode-reset/internal/model"
	"zai-zcode-reset/internal/quota"
	"zai-zcode-reset/internal/upstream"
)

type availableItem struct {
	ExpireAt int64 `json:"expire_at"`
}

type resetTypeSnapshot struct {
	Available  []availableItem `json:"available"`
	LatestUsed *int64          `json:"latest_used_at"`
}

type periodQuota struct {
	DayUsed   int `json:"day_used"`
	DayLimit  int `json:"day_limit"`
	WeekUsed  int `json:"week_used"`
	WeekLimit int `json:"week_limit"`
}

type statusResponse struct {
	Account       map[string]interface{} `json:"account"`
	FiveHour      resetTypeSnapshot      `json:"five_hour"`
	Week          resetTypeSnapshot      `json:"week"`
	Quota         map[string]periodQuota `json:"quota"`
	CooldownUntil map[string]int64       `json:"cooldown_until"`
	ServerTime    int64                  `json:"server_time"`
	LastResult    *executeOutcome        `json:"last_result,omitempty"`
}

// resolveClient 解析当前用户绑定的上游账号并构造客户端。
func (s *Server) resolveClient(user model.User) (model.UpstreamAccount, *upstream.Client, error) {
	account, ok := s.store.ResolveAccount(user.ID)
	if !ok {
		return account, nil, errors.New("no_account")
	}
	jwt, err1 := s.box.Decrypt(account.ZCodeJWT)
	token, err2 := s.box.Decrypt(account.PlanToken)
	if err1 != nil || err2 != nil {
		return account, nil, errors.New("token_decrypt_failed")
	}
	baseURL := account.BaseURL
	if baseURL == "" {
		baseURL = upstream.DefaultBaseURL(account.Family)
	}
	client := upstream.NewClient(upstream.Credentials{
		ZCodeJWT:   jwt,
		PlanToken:  token,
		Family:     account.Family,
		TargetType: account.TargetType,
		OrgID:      account.OrgID,
		ProjectID:  account.ProjectID,
		BaseURL:    baseURL,
	}, s.cfg.MockUpstream)
	return account, client, nil
}

func validOpportunities(list []upstream.Opportunity, nowMs int64) []availableItem {
	out := make([]availableItem, 0, len(list))
	for _, o := range list {
		if o.ExpireAt > nowMs {
			out = append(out, availableItem{ExpireAt: o.ExpireAt})
		}
	}
	return out
}

func snapshotType(st *upstream.Status, resetType string, nowMs int64) resetTypeSnapshot {
	var ops []upstream.Opportunity
	var hist *upstream.History
	if resetType == model.ResetWeek {
		ops, hist = st.AvailableWeek, st.LatestWeek
	} else {
		ops, hist = st.AvailableFiveHour, st.LatestFiveHour
	}
	snap := resetTypeSnapshot{Available: validOpportunities(ops, nowMs)}
	if hist != nil {
		used := hist.UsedAt
		snap.LatestUsed = &used
	}
	return snap
}

func accountInfo(a model.UpstreamAccount) map[string]interface{} {
	return map[string]interface{}{
		"id":          a.ID,
		"name":        a.Name,
		"family":      a.Family,
		"target_type": a.TargetType,
	}
}

func quotaSnapshot(q *quota.Manager, userID string) map[string]periodQuota {
	fh := q.Usage(userID, model.ResetFiveHour)
	wk := q.Usage(userID, model.ResetWeek)
	return map[string]periodQuota{
		"five_hour": {DayUsed: fh.DayUsed, DayLimit: fh.DayLimit, WeekUsed: fh.WeekUsed, WeekLimit: fh.WeekLimit},
		"week":      {DayUsed: wk.DayUsed, DayLimit: wk.DayLimit, WeekUsed: wk.WeekUsed, WeekLimit: wk.WeekLimit},
	}
}

func (s *Server) buildStatusResponse(q *quota.Manager, account model.UpstreamAccount, st *upstream.Status, userID string) statusResponse {
	nowMs := time.Now().UnixMilli()
	return statusResponse{
		Account:  accountInfo(account),
		FiveHour: snapshotType(st, model.ResetFiveHour, nowMs),
		Week:     snapshotType(st, model.ResetWeek, nowMs),
		Quota:    quotaSnapshot(q, userID),
		CooldownUntil: map[string]int64{
			"five_hour": q.CooldownUntil(account.ID, model.ResetFiveHour),
			"week":      q.CooldownUntil(account.ID, model.ResetWeek),
		},
		ServerTime: nowMs,
	}
}

func (s *Server) handleResetStatus(w http.ResponseWriter, r *http.Request) {
	user, _, _ := CurrentUser(r.Context())
	account, client, err := s.resolveClient(user)
	if err != nil {
		s.writeUpstreamSetupError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	st, err := client.Status(ctx)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.buildStatusResponse(s.quota, account, st, user.ID))
}

type usageWindow struct {
	UsedPercent int   `json:"used_percent"`
	NextResetAt int64 `json:"next_reset_at"`
}

type usageResponse struct {
	Account  string        `json:"account"`
	Level    string        `json:"level"`
	FiveHour *usageWindow  `json:"five_hour"`
	Week     *usageWindow  `json:"week"`
	Tool     *usageWindow  `json:"tool,omitempty"`
	ToolInfo *struct {
		Used      int `json:"used"`
		Remaining int `json:"remaining"`
	} `json:"tool_info,omitempty"`
}

func findUsageLimit(limits []upstream.UsageLimit, limitType string, unit int) *upstream.UsageLimit {
	for i := range limits {
		if limits[i].Type == limitType && limits[i].Unit == unit {
			return &limits[i]
		}
	}
	return nil
}

func windowFrom(l *upstream.UsageLimit) *usageWindow {
	if l == nil {
		return nil
	}
	return &usageWindow{UsedPercent: l.Percentage, NextResetAt: l.NextResetTime}
}

// handleResetUsage 返回当前账号 Coding Plan 的真实用量（5 小时 / 周 / 月工具配额），
// 数据来自上游只读监控接口 quota/limit，不消耗任何额度。
func (s *Server) handleResetUsage(w http.ResponseWriter, r *http.Request) {
	user, _, _ := CurrentUser(r.Context())
	account, client, err := s.resolveClient(user)
	if err != nil {
		s.writeUpstreamSetupError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	result, err := client.QuotaLimit(ctx)
	if err != nil {
		s.writeUpstreamError(w, err)
		return
	}
	resp := usageResponse{
		Account:  account.Name,
		Level:    result.Level,
		FiveHour: windowFrom(findUsageLimit(result.Limits, "TOKENS_LIMIT", 3)),
		Week:     windowFrom(findUsageLimit(result.Limits, "TOKENS_LIMIT", 6)),
		Tool:     windowFrom(findUsageLimit(result.Limits, "TIME_LIMIT", 5)),
	}
	if tool := findUsageLimit(result.Limits, "TIME_LIMIT", 5); tool != nil && tool.Usage != nil {
		// 上游契约：usage=总量，currentValue=已用，remaining=剩余。
		used := *tool.Usage
		remaining := 0
		if tool.CurrentValue != nil {
			used = *tool.CurrentValue
		}
		if tool.Remaining != nil {
			remaining = *tool.Remaining
		}
		resp.ToolInfo = &struct {
			Used      int `json:"used"`
			Remaining int `json:"remaining"`
		}{Used: used, Remaining: remaining}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleResetHistory 返回当前用户自己的最近重置记录（成员侧「记录」页数据源）。
func (s *Server) handleResetHistory(w http.ResponseWriter, r *http.Request) {
	user, _, _ := CurrentUser(r.Context())
	logs, err := s.store.ReadResetLogs(1000)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "读取重置记录失败")
		return
	}
	out := make([]model.ResetLog, 0, 32)
	for i := len(logs) - 1; i >= 0 && len(out) < 50; i-- {
		if logs[i].UserID == user.ID {
			out = append(out, logs[i])
		}
	}
	for i := range out {
		out[i].OpportunityKey = "" // 幂等键不外露
		out[i].UseKey = ""
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"records": out})
}

type executeRequest struct {
	ResetType string `json:"reset_type"`
	// DryRun 演练模式（仅管理员）：完整走平台链路校验，但不调上游 use、
	// 不消耗机会、不预占配额。用于上线前验证点击链路而避免真实消耗。
	DryRun bool `json:"dry_run"`
}

type executeOutcome struct {
	ResetType string `json:"reset_type"`
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// handleResetExecute 编排完整重置链路：
// 配额预占 → status 查可用机会 →（无则申领 opportunity，处理 3301/429 冷却）→ use 消耗 → 回传新快照。
func (s *Server) handleResetExecute(w http.ResponseWriter, r *http.Request) {
	user, sess, _ := CurrentUser(r.Context())
	var req executeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ResetType != model.ResetFiveHour && req.ResetType != model.ResetWeek {
		writeErr(w, http.StatusBadRequest, "bad_request", "reset_type 必须是 FIVE_HOUR 或 WEEK")
		return
	}
	if req.DryRun && user.Role != model.RoleAdmin {
		writeErr(w, http.StatusForbidden, "forbidden", "演练模式仅管理员可用")
		return
	}
	resetType := req.ResetType

	account, client, err := s.resolveClient(user)
	if err != nil {
		s.writeUpstreamSetupError(w, err)
		return
	}

	// 演练模式：只读校验（冷却 / 配额余量 / 上游连通），不预占、不写冷却、不调 use。
	if req.DryRun {
		if until := s.quota.CooldownUntil(account.ID, resetType); until > time.Now().UnixMilli() {
			writeErrExtra(w, http.StatusTooManyRequests, "cooldown",
				"演练中止：当前处于冷却期", map[string]interface{}{"next_try_at": until})
			return
		}
		usage := s.quota.Usage(user.ID, resetType)
		if usage.DayUsed >= usage.DayLimit || usage.WeekUsed >= usage.WeekLimit {
			writeErr(w, http.StatusTooManyRequests, "limit_reached", "演练中止：配额已用尽，真实点击也会被拒绝")
			return
		}
		dctx, dcancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer dcancel()
		st, err := client.Status(dctx) // 只读连通性 + 真实机会快照
		if err != nil {
			s.writeUpstreamError(w, err)
			return
		}
		s.logReset(user, sess, account, resetType, true, "", "", 0, "dry_run", "")
		resp := s.buildStatusResponse(s.quota, account, st, user.ID)
		resp.LastResult = &executeOutcome{
			ResetType: resetType,
			Success:   true,
			Message:   "演练成功：链路与配额校验通过，未调用上游、未消耗机会",
			DryRun:    true,
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// 平台冷却：上游 3301/429 的边界必须在本地生效，避免持续撞限流。
	if until := s.quota.CooldownUntil(account.ID, resetType); until > time.Now().UnixMilli() {
		s.logReset(user, sess, account, resetType, false, "", "", 0, "cooldown_blocked", "")
		writeErrExtra(w, http.StatusTooManyRequests, "cooldown",
			"重置太频繁，请稍后再试", map[string]interface{}{"next_try_at": until})
		return
	}

	if err := s.quota.Reserve(user.ID, resetType); err != nil {
		var limitErr *quota.LimitError
		if errors.As(err, &limitErr) {
			s.logReset(user, sess, account, resetType, false, "", "", 0, "limit_reached", "")
			writeErr(w, http.StatusTooManyRequests, "limit_reached", limitErr.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal", "配额检查失败")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	fail := func(status int, code, message, kind, oppKey, useKey string, upstreamCode int) {
		s.quota.Rollback(user.ID, resetType)
		s.logReset(user, sess, account, resetType, false, oppKey, useKey, upstreamCode, kind, message)
		writeErr(w, status, code, message)
	}

	// ① 查询当前可用机会。
	st, err := client.Status(ctx)
	if err != nil {
		fail(http.StatusBadGateway, "upstream_error", "查询重置状态失败: "+err.Error(), "status_error", "", "", 0)
		return
	}
	nowMs := time.Now().UnixMilli()
	available := validOpportunities(st.AvailableFiveHour, nowMs)
	if resetType == model.ResetWeek {
		available = validOpportunities(st.AvailableWeek, nowMs)
	}

	oppKey := ""
	if len(available) == 0 {
		// ② 没有机会则先申领。
		oppKey = upstream.NewIdempotencyKey()
		opp, err := client.RequestOpportunity(ctx, oppKey)
		if err != nil {
			var denied *upstream.DeniedError
			var throttled *upstream.ThrottledError
			switch {
			case errors.As(err, &denied):
				s.quota.SetCooldown(account.ID, resetType, denied.NextTryAt)
				fail(http.StatusTooManyRequests, "no_opportunity", "上游暂未开放重置机会申领", "opportunity_denied", oppKey, "", 3301)
				return
			case errors.As(err, &throttled):
				s.quota.SetCooldown(account.ID, resetType, time.Now().Add(throttled.RetryAfter).UnixMilli())
				fail(http.StatusTooManyRequests, "throttled", "上游限流，请稍后再试", "opportunity_throttled", oppKey, "", 429)
				return
			default:
				var biz *upstream.BusinessError
				upstreamCode := 0
				if errors.As(err, &biz) {
					upstreamCode = biz.Code
				}
				fail(http.StatusBadGateway, "upstream_error", "申领重置机会失败: "+err.Error(), "opportunity_error", oppKey, "", upstreamCode)
				return
			}
		}
		if !opp.Granted {
			s.quota.SetCooldown(account.ID, resetType, opp.NextTryAt)
			fail(http.StatusTooManyRequests, "no_opportunity", "当前没有可申领的重置机会", "opportunity_not_granted", oppKey, "", 3301)
			return
		}
		// 申领成功后重新查询，确认机会已入账。
		st, err = client.Status(ctx)
		if err != nil {
			fail(http.StatusBadGateway, "upstream_error", "申领后查询状态失败: "+err.Error(), "status_after_grant", oppKey, "", 0)
			return
		}
	}

	// ③ 消耗机会。
	useKey := upstream.NewIdempotencyKey()
	if err := client.Use(ctx, useKey, resetType); err != nil {
		var biz *upstream.BusinessError
		upstreamCode := 0
		if errors.As(err, &biz) {
			upstreamCode = biz.Code
		}
		fail(http.StatusBadGateway, "upstream_error", "使用重置机会失败: "+err.Error(), "use_error", oppKey, useKey, upstreamCode)
		return
	}

	// ④ 成功：回传最新快照。
	finalStatus := st
	if refreshed, err := client.Status(ctx); err == nil {
		finalStatus = refreshed
	}
	s.logReset(user, sess, account, resetType, true, oppKey, useKey, 0, "", "")

	resp := s.buildStatusResponse(s.quota, account, finalStatus, user.ID)
	resp.LastResult = &executeOutcome{
		ResetType: resetType,
		Success:   true,
		Message:   "重置成功",
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) writeUpstreamSetupError(w http.ResponseWriter, err error) {
	switch err.Error() {
	case "no_account":
		writeErr(w, http.StatusServiceUnavailable, "account_unavailable", "管理员尚未配置可用的上游账号")
	case "token_decrypt_failed":
		writeErr(w, http.StatusServiceUnavailable, "token_decrypt_failed", "上游令牌无法解密，请管理员重新录入")
	default:
		writeErr(w, http.StatusInternalServerError, "internal", "内部错误")
	}
}

func (s *Server) writeUpstreamError(w http.ResponseWriter, err error) {
	var throttled *upstream.ThrottledError
	if errors.As(err, &throttled) {
		writeErr(w, http.StatusTooManyRequests, "throttled", "上游限流，请稍后再试")
		return
	}
	writeErr(w, http.StatusBadGateway, "upstream_error", "上游请求失败: "+err.Error())
}

func (s *Server) logReset(
	user model.User, sess model.Session, account model.UpstreamAccount,
	resetType string, success bool, oppKey, useKey string, upstreamCode int, kind, message string,
) {
	_ = s.store.AppendResetLog(model.ResetLog{
		At:              time.Now(),
		UserID:          user.ID,
		Username:        user.Username,
		AccountID:       account.ID,
		AccountName:     account.Name,
		ResetType:       resetType,
		Success:         success,
		OpportunityKey:  oppKey,
		UseKey:          useKey,
		UpstreamCode:    upstreamCode,
		UpstreamMessage: message,
		ErrorKind:       kind,
		IP:              sess.IP,
	})
}
