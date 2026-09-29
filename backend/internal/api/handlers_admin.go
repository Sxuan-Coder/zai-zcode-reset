package api

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"zai-zcode-reset/internal/cryptoutil"
	"zai-zcode-reset/internal/model"
	"zai-zcode-reset/internal/quota"
	"zai-zcode-reset/internal/store"
	"zai-zcode-reset/internal/upstream"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

func (s *Server) audit(r *http.Request, action, targetID string, detail map[string]interface{}) {
	admin, _, _ := CurrentUser(r.Context())
	_ = s.store.AppendAuditLog(model.AuditLog{
		At:       time.Now(),
		AdminID:  admin.ID,
		Admin:    admin.Username,
		Action:   action,
		TargetID: targetID,
		Detail:   detail,
	})
}

// ---------- 总览 ----------

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	users := s.store.ListUsers()
	sessions := s.store.ListActiveSessions()
	accounts := s.store.ListAccounts()
	resets, _ := s.store.ReadResetLogs(500)
	today := time.Now().Format("2006-01-02")
	todaySuccess, todayTotal := 0, 0
	for _, l := range resets {
		if l.At.Format("2006-01-02") == today {
			todayTotal++
			if l.Success {
				todaySuccess++
			}
		}
	}
	defaultAccount := ""
	for _, a := range accounts {
		if a.IsDefault && a.Enabled {
			defaultAccount = a.Name
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"users":                     len(users),
		"active_sessions":           len(sessions),
		"accounts":                  len(accounts),
		"default_account":           defaultAccount,
		"resets_today":              todaySuccess,
		"resets_today_all_attempts": todayTotal,
		"mock_upstream":             s.cfg.MockUpstream,
	})
}

// ---------- 用户管理 ----------

type adminUser struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	Role              string `json:"role"`
	Status            string `json:"status"`
	CreatedAt         string `json:"created_at"`
	LastLoginAt       string `json:"last_login_at"`
	UpstreamAccountID string `json:"upstream_account_id,omitempty"`
}

func toAdminUser(u model.User) adminUser {
	last := ""
	if !u.LastLoginAt.IsZero() {
		last = u.LastLoginAt.Format(time.RFC3339)
	}
	return adminUser{
		ID: u.ID, Username: u.Username, Role: u.Role, Status: u.Status,
		CreatedAt: u.CreatedAt.Format(time.RFC3339), LastLoginAt: last,
		UpstreamAccountID: u.UpstreamAccountID,
	}
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users := s.store.ListUsers()
	out := make([]adminUser, 0, len(users))
	for _, u := range users {
		out = append(out, toAdminUser(u))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": out})
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` // 空则自动生成
	Role     string `json:"role"`
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if !usernamePattern.MatchString(req.Username) {
		writeErr(w, http.StatusBadRequest, "bad_request", "用户名需为 3-32 位字母/数字/下划线/连字符")
		return
	}
	if req.Role != model.RoleAdmin && req.Role != model.RoleMember {
		req.Role = model.RoleMember
	}
	generated := false
	if req.Password == "" {
		pw, err := cryptoutil.RandomPassword(12)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", "生成密码失败")
			return
		}
		req.Password = pw
		generated = true
	}
	if len(req.Password) < 8 || len(req.Password) > 128 {
		writeErr(w, http.StatusBadRequest, "bad_request", "密码长度需在 8-128 之间")
		return
	}
	hash, err := cryptoutil.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "密码哈希失败")
		return
	}
	user, err := s.store.CreateUser(model.User{
		Username: req.Username, PasswordHash: hash, Role: req.Role, Status: model.StatusActive,
	})
	if err == store.ErrConflict {
		writeErr(w, http.StatusConflict, "username_taken", "用户名已存在")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "创建用户失败")
		return
	}
	s.audit(r, "user.create", user.ID, map[string]interface{}{"username": user.Username, "role": user.Role})
	resp := map[string]interface{}{"user": toAdminUser(user)}
	if generated {
		resp["initial_password"] = req.Password // 仅本次响应可见
	}
	writeJSON(w, http.StatusCreated, resp)
}

type patchUserRequest struct {
	Status    *string `json:"status"`
	Role      *string `json:"role"`
	Password  *string `json:"password"`
	AccountID *string `json:"upstream_account_id"`
}

func (s *Server) handlePatchUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req patchUserRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	admin, _, _ := CurrentUser(r.Context())

	var newPasswordHash string
	if req.Password != nil {
		if len(*req.Password) < 8 || len(*req.Password) > 128 {
			writeErr(w, http.StatusBadRequest, "bad_request", "密码长度需在 8-128 之间")
			return
		}
		hash, err := cryptoutil.HashPassword(*req.Password)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", "密码哈希失败")
			return
		}
		newPasswordHash = hash
	}

	updated, err := s.store.UpdateUser(id, func(u *model.User) error {
		if req.Status != nil {
			if *req.Status != model.StatusActive && *req.Status != model.StatusDisabled {
				return errBadRequest("status 取值非法")
			}
			if u.ID == admin.ID && *req.Status == model.StatusDisabled {
				return errBadRequest("不能停用自己的账号")
			}
			u.Status = *req.Status
		}
		if req.Role != nil {
			if *req.Role != model.RoleAdmin && *req.Role != model.RoleMember {
				return errBadRequest("role 取值非法")
			}
			if u.ID == admin.ID && *req.Role == model.RoleMember {
				return errBadRequest("不能移除自己的管理员权限")
			}
			u.Role = *req.Role
		}
		if newPasswordHash != "" {
			u.PasswordHash = newPasswordHash
		}
		if req.AccountID != nil {
			if *req.AccountID != "" {
				if _, ok := s.store.GetAccount(*req.AccountID); !ok {
					return errBadRequest("绑定的上游账号不存在")
				}
			}
			u.UpstreamAccountID = *req.AccountID
		}
		return nil
	})
	if err != nil {
		writeStoreErr(w, err)
		return
	}

	// 停用或改密后强制下线，防止旧会话继续操作。
	if (req.Status != nil && *req.Status == model.StatusDisabled) || newPasswordHash != "" {
		s.store.RevokeUserSessions(updated.ID)
	}

	detail := map[string]interface{}{}
	if req.Status != nil {
		detail["status"] = *req.Status
	}
	if req.Role != nil {
		detail["role"] = *req.Role
	}
	if newPasswordHash != "" {
		detail["password_changed"] = true
	}
	if req.AccountID != nil {
		detail["upstream_account_id"] = *req.AccountID
	}
	s.audit(r, "user.patch", updated.ID, detail)
	writeJSON(w, http.StatusOK, map[string]interface{}{"user": toAdminUser(updated)})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	admin, _, _ := CurrentUser(r.Context())
	if id == admin.ID {
		writeErr(w, http.StatusBadRequest, "bad_request", "不能删除自己的账号")
		return
	}
	if err := s.store.DeleteUser(id); err != nil {
		writeStoreErr(w, err)
		return
	}
	s.audit(r, "user.delete", id, nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func (s *Server) handleRevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.store.GetUser(id); !ok {
		writeErr(w, http.StatusNotFound, "not_found", "用户不存在")
		return
	}
	n := s.store.RevokeUserSessions(id)
	s.audit(r, "user.force_logout", id, map[string]interface{}{"revoked": n})
	writeJSON(w, http.StatusOK, map[string]interface{}{"revoked": n})
}

// ---------- 会话管理 ----------

type adminSession struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
	CreatedAt string `json:"created_at"`
	LastSeen  string `json:"last_seen"`
	ExpiresAt string `json:"expires_at"`
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := s.store.ListActiveSessions()
	users := map[string]string{}
	for _, u := range s.store.ListUsers() {
		users[u.ID] = u.Username
	}
	out := make([]adminSession, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, adminSession{
			ID: sess.ID, UserID: sess.UserID, Username: users[sess.UserID],
			IP: sess.IP, UserAgent: sess.UserAgent,
			CreatedAt: sess.CreatedAt.Format(time.RFC3339),
			LastSeen:  sess.LastSeenAt.Format(time.RFC3339),
			ExpiresAt: sess.ExpiresAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sessions": out})
}

func (s *Server) handleRevokeSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.RevokeSession(id); err != nil {
		writeStoreErr(w, err)
		return
	}
	s.audit(r, "session.revoke", id, nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// ---------- 规则管理 ----------

type adminRule struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id,omitempty"`
	Username  string `json:"username,omitempty"`
	ResetType string `json:"reset_type"`
	Period    string `json:"period"`
	MaxCount  int    `json:"max_count"`
	Enabled   bool   `json:"enabled"`
}

func validResetType(t string) bool {
	return t == model.ResetFiveHour || t == model.ResetWeek || t == model.ResetAll
}

func validPeriod(p string) bool {
	return p == model.PeriodDay || p == model.PeriodWeek
}

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	rules := s.store.ListRules()
	quota.SortRules(rules)
	users := map[string]string{}
	for _, u := range s.store.ListUsers() {
		users[u.ID] = u.Username
	}
	out := make([]adminRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, adminRule{
			ID: rule.ID, UserID: rule.UserID, Username: users[rule.UserID],
			ResetType: rule.ResetType, Period: rule.Period,
			MaxCount: rule.MaxCount, Enabled: rule.Enabled,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"rules": out})
}

type ruleRequest struct {
	UserID    string `json:"user_id"`
	ResetType string `json:"reset_type"`
	Period    string `json:"period"`
	MaxCount  int    `json:"max_count"`
	Enabled   *bool  `json:"enabled"`
}

func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	var req ruleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.UserID != "" {
		if _, ok := s.store.GetUser(req.UserID); !ok {
			writeErr(w, http.StatusBadRequest, "bad_request", "规则指向的用户不存在")
			return
		}
	}
	if !validResetType(req.ResetType) || !validPeriod(req.Period) {
		writeErr(w, http.StatusBadRequest, "bad_request", "reset_type / period 取值非法")
		return
	}
	if req.MaxCount < 0 || req.MaxCount > 1000 {
		writeErr(w, http.StatusBadRequest, "bad_request", "max_count 需在 0-1000 之间")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	rule, err := s.store.CreateRule(model.Rule{
		UserID: req.UserID, ResetType: req.ResetType, Period: req.Period,
		MaxCount: req.MaxCount, Enabled: enabled,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "创建规则失败")
		return
	}
	s.audit(r, "rule.create", rule.ID, map[string]interface{}{
		"user_id": req.UserID, "reset_type": req.ResetType, "period": req.Period, "max_count": req.MaxCount,
	})
	writeJSON(w, http.StatusCreated, map[string]interface{}{"rule": rule})
}

func (s *Server) handlePatchRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req ruleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	rule, err := s.store.UpdateRule(id, func(x *model.Rule) error {
		if req.ResetType != "" {
			if !validResetType(req.ResetType) {
				return errBadRequest("reset_type 取值非法")
			}
			x.ResetType = req.ResetType
		}
		if req.Period != "" {
			if !validPeriod(req.Period) {
				return errBadRequest("period 取值非法")
			}
			x.Period = req.Period
		}
		if req.MaxCount != 0 {
			if req.MaxCount < 0 || req.MaxCount > 1000 {
				return errBadRequest("max_count 需在 0-1000 之间")
			}
			x.MaxCount = req.MaxCount
		}
		if req.Enabled != nil {
			x.Enabled = *req.Enabled
		}
		return nil
	})
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	s.audit(r, "rule.patch", id, nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{"rule": rule})
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteRule(id); err != nil {
		writeStoreErr(w, err)
		return
	}
	s.audit(r, "rule.delete", id, nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// ---------- 上游账号管理（令牌只写不读） ----------

type adminAccount struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Family     string `json:"family"`
	BaseURL    string `json:"base_url,omitempty"`
	TargetType string `json:"target_type"`
	OrgID      string `json:"org_id,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
	Enabled    bool   `json:"enabled"`
	IsDefault  bool   `json:"is_default"`
	JWTMask    string `json:"zcode_jwt_mask"`
	TokenMask  string `json:"plan_token_mask"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func maskToken(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 12 {
		return strings.Repeat("*", len(v))
	}
	return v[:6] + "…" + v[len(v)-4:]
}

func toAdminAccount(a model.UpstreamAccount, box *cryptoutil.SecretBox) adminAccount {
	jwtPlain, err1 := box.Decrypt(a.ZCodeJWT)
	tokenPlain, err2 := box.Decrypt(a.PlanToken)
	jwtMask, tokenMask := "<无法解密>", "<无法解密>"
	if err1 == nil {
		jwtMask = maskToken(jwtPlain)
	}
	if err2 == nil {
		tokenMask = maskToken(tokenPlain)
	}
	return adminAccount{
		ID: a.ID, Name: a.Name, Family: a.Family, BaseURL: a.BaseURL,
		TargetType: a.TargetType, OrgID: a.OrgID, ProjectID: a.ProjectID,
		Enabled: a.Enabled, IsDefault: a.IsDefault,
		JWTMask: jwtMask, TokenMask: tokenMask,
		CreatedAt: a.CreatedAt.Format(time.RFC3339), UpdatedAt: a.UpdatedAt.Format(time.RFC3339),
	}
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts := s.store.ListAccounts()
	out := make([]adminAccount, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, toAdminAccount(a, s.box))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"accounts": out})
}

type accountRequest struct {
	Name       string `json:"name"`
	Family     string `json:"family"`
	BaseURL    string `json:"base_url"`
	TargetType string `json:"target_type"`
	OrgID      string `json:"org_id"`
	ProjectID  string `json:"project_id"`
	ZCodeJWT   string `json:"zcode_jwt"`  // 创建必填；更新时留空表示保持不变
	PlanToken  string `json:"plan_token"` // 同上
	Enabled    *bool  `json:"enabled"`
	IsDefault  *bool  `json:"is_default"`
}

func (s *Server) buildAccount(req accountRequest, existing *model.UpstreamAccount) (model.UpstreamAccount, error) {
	acc := model.UpstreamAccount{}
	if existing != nil {
		acc = *existing
	}
	if req.Name != "" {
		acc.Name = strings.TrimSpace(req.Name)
	}
	if req.Family != "" {
		if req.Family != model.FamilyBigModel && req.Family != model.FamilyZAI {
			return acc, errBadRequest("family 必须是 bigmodel 或 zai")
		}
		acc.Family = req.Family
	}
	if req.BaseURL != "" {
		acc.BaseURL = strings.TrimSpace(req.BaseURL)
	}
	if req.TargetType != "" {
		if req.TargetType != model.TargetPersonal && req.TargetType != model.TargetTeam {
			return acc, errBadRequest("target_type 必须是 PERSONAL 或 TEAM")
		}
		acc.TargetType = req.TargetType
	}
	if req.OrgID != "" {
		acc.OrgID = strings.TrimSpace(req.OrgID)
	}
	if req.ProjectID != "" {
		acc.ProjectID = strings.TrimSpace(req.ProjectID)
	}
	if req.Enabled != nil {
		acc.Enabled = *req.Enabled
	}
	if req.IsDefault != nil {
		acc.IsDefault = *req.IsDefault
	}
	if req.ZCodeJWT != "" {
		enc, err := s.box.Encrypt(strings.TrimSpace(req.ZCodeJWT))
		if err != nil {
			return acc, errBadRequest("令牌加密失败")
		}
		acc.ZCodeJWT = enc
	}
	if req.PlanToken != "" {
		enc, err := s.box.Encrypt(strings.TrimSpace(req.PlanToken))
		if err != nil {
			return acc, errBadRequest("令牌加密失败")
		}
		acc.PlanToken = enc
	}
	return acc, nil
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req accountRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "账号名称不能为空")
		return
	}
	if req.ZCodeJWT == "" || req.PlanToken == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "必须同时提供 zcode_jwt 与 plan_token")
		return
	}
	acc, err := s.buildAccount(req, nil)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	acc.Family = orDefault(acc.Family, model.FamilyBigModel)
	acc.TargetType = orDefault(acc.TargetType, model.TargetPersonal)
	acc.Enabled = true
	created, err := s.store.CreateAccount(acc)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "创建账号失败")
		return
	}
	// 首个账号自动设为默认。
	if len(s.store.ListAccounts()) == 1 && !created.IsDefault {
		created, _ = s.store.UpdateAccount(created.ID, func(a *model.UpstreamAccount) error {
			a.IsDefault = true
			return nil
		})
	}
	s.audit(r, "account.create", created.ID, map[string]interface{}{
		"name": created.Name, "family": created.Family, "target_type": created.TargetType,
	})
	writeJSON(w, http.StatusCreated, map[string]interface{}{"account": toAdminAccount(created, s.box)})
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func (s *Server) handlePatchAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, ok := s.store.GetAccount(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "账号不存在")
		return
	}
	var req accountRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	updated, err := s.buildAccount(req, &existing)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	saved, err := s.store.UpdateAccount(id, func(a *model.UpstreamAccount) error {
		*a = updated
		a.ID = id // 防御：不允许改 ID
		return nil
	})
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	s.audit(r, "account.patch", id, map[string]interface{}{"name": saved.Name})
	writeJSON(w, http.StatusOK, map[string]interface{}{"account": toAdminAccount(saved, s.box)})
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteAccount(id); err != nil {
		writeStoreErr(w, err)
		return
	}
	s.audit(r, "account.delete", id, nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func (s *Server) handleTestAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	account, ok := s.store.GetAccount(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "账号不存在")
		return
	}
	jwt, err1 := s.box.Decrypt(account.ZCodeJWT)
	token, err2 := s.box.Decrypt(account.PlanToken)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": "令牌无法解密，请重新录入"})
		return
	}
	baseURL := account.BaseURL
	if baseURL == "" {
		baseURL = upstream.DefaultBaseURL(account.Family)
	}
	client := upstream.NewClient(upstream.Credentials{
		ZCodeJWT: jwt, PlanToken: token, TargetType: account.TargetType,
		OrgID: account.OrgID, ProjectID: account.ProjectID, BaseURL: baseURL,
	}, s.cfg.MockUpstream)
	ctx := r.Context()
	st, err := client.Status(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "message": "连接失败: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true, "message": "连接成功",
		"five_hour_available": len(st.AvailableFiveHour),
		"week_available":      len(st.AvailableWeek),
	})
}

// ---------- 日志 ----------

func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	switch kind {
	case "login":
		items, err := s.store.ReadLoginLogs(limit)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", "读取登录日志失败")
			return
		}
		reverseLogin(items)
		writeJSON(w, http.StatusOK, map[string]interface{}{"logs": items})
	case "reset":
		items, err := s.store.ReadResetLogs(limit)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", "读取重置日志失败")
			return
		}
		reverseReset(items)
		writeJSON(w, http.StatusOK, map[string]interface{}{"logs": items})
	case "audit":
		items, err := s.store.ReadAuditLogs(limit)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "internal", "读取审计日志失败")
			return
		}
		reverseAudit(items)
		writeJSON(w, http.StatusOK, map[string]interface{}{"logs": items})
	default:
		writeErr(w, http.StatusNotFound, "not_found", "日志类型不存在")
	}
}

func reverseLogin(in []model.LoginLog) {
	for i, j := 0, len(in)-1; i < j; i, j = i+1, j-1 {
		in[i], in[j] = in[j], in[i]
	}
}

func reverseReset(in []model.ResetLog) {
	for i, j := 0, len(in)-1; i < j; i, j = i+1, j-1 {
		in[i], in[j] = in[j], in[i]
	}
}

func reverseAudit(in []model.AuditLog) {
	for i, j := 0, len(in)-1; i < j; i, j = i+1, j-1 {
		in[i], in[j] = in[j], in[i]
	}
}

// ---------- 错误映射 ----------

type badRequestError struct{ msg string }

func (e *badRequestError) Error() string { return e.msg }

func errBadRequest(msg string) error { return &badRequestError{msg: msg} }

func writeStoreErr(w http.ResponseWriter, err error) {
	if _, ok := err.(*badRequestError); ok {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err == store.ErrNotFound {
		writeErr(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	if err == store.ErrConflict {
		writeErr(w, http.StatusConflict, "conflict", "资源冲突")
		return
	}
	writeErr(w, http.StatusInternalServerError, "internal", "内部错误")
}
