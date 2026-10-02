// Package upstream 封装 BigModel / ZAI Coding Plan 的额度重置 API。
// 信封语义（与 ZCode 客户端核实一致）：code=0 成功；3301 = 申领被拒
// （data.next_try_at 为毫秒冷却边界）；HTTP 429 = 限流。时间戳一律为毫秒。
package upstream

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"zai-zcode-reset/internal/model"
)

const (
	businessDenied = 3301
	requestTimeout = 15 * time.Second
)

type Credentials struct {
	ZCodeJWT   string
	PlanToken  string
	Family     string // bigmodel | zai（用量监控接口按 family 选主机）
	TargetType string
	OrgID      string
	ProjectID  string
	BaseURL    string // 覆盖 family 默认 origin
}

type Client struct {
	http  *http.Client
	creds Credentials
	mock  bool
	// mock 模式下按账号共享状态：真实上游的可用机会跨请求持久，
	// mock 也必须如此，否则每次 status 都回到空态。
	mockKey string
}

// mock 模式的全局状态按账号隔离（JWT+token 组合即一个账号）。
var (
	mockMu    sync.Mutex
	mockStore = map[string]*mockState{}
)

func sharedMockState(key string) *mockState {
	mockMu.Lock()
	defer mockMu.Unlock()
	st := mockStore[key]
	if st == nil {
		st = &mockState{opportunities: map[string][]Opportunity{}}
		mockStore[key] = st
	}
	return st
}

type Opportunity struct {
	ExpireAt int64 `json:"expire_at"`
}

type History struct {
	UsedAt int64 `json:"used_at"`
}

type Status struct {
	AvailableFiveHour []Opportunity `json:"available_five_hour_resets"`
	AvailableWeek     []Opportunity `json:"available_week_resets"`
	LatestFiveHour    *History      `json:"latest_five_hour_reset_history"`
	LatestWeek        *History      `json:"latest_week_reset_history"`
	HasUnreadHistory  bool          `json:"has_unread_history"`
}

type OpportunityResult struct {
	Granted   bool
	NextTryAt int64 // 毫秒；0 表示无冷却信息
}

// UsageLimit 是 quota/limit 返回的单个额度桶。
// 契约（与 ZCode 客户端 bigmodelUsageQuotaMapper 核对一致）：
//   - TOKENS_LIMIT / unit=3 → 5 小时窗口 token 配额；unit=6 → 周窗口 token 配额
//   - TIME_LIMIT  / unit=5 → 每月工具调用配额（带 usage/remaining）
//   - percentage 为「已使用」占比；nextResetTime 为毫秒时间戳
type UsageLimit struct {
	Type         string `json:"type"`
	Unit         int    `json:"unit"`
	Number       int    `json:"number"`
	Usage        *int   `json:"usage,omitempty"`
	CurrentValue *int   `json:"currentValue,omitempty"`
	Remaining    *int   `json:"remaining,omitempty"`
	Percentage   int    `json:"percentage"`
	NextResetTime int64 `json:"nextResetTime"`
}

type QuotaLimitResult struct {
	Level  string       `json:"level"`
	Limits []UsageLimit `json:"limits"`
}

// DeniedError 上游业务码 3301：当前不允许申领（带服务端冷却边界）。
type DeniedError struct{ NextTryAt int64 }

func (e *DeniedError) Error() string {
	return fmt.Sprintf("上游拒绝申领重置机会（3301），next_try_at=%d", e.NextTryAt)
}

// ThrottledError HTTP 429：请求过快，需要本地退避。
type ThrottledError struct{ RetryAfter time.Duration }

func (e *ThrottledError) Error() string { return "上游限流（HTTP 429），请稍后重试" }

// BusinessError 其他非 0 业务码或 HTTP 层错误。
type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string {
	return fmt.Sprintf("上游业务错误 %d: %s", e.Code, e.Message)
}

type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func NewClient(creds Credentials, mock bool) *Client {
	return &Client{
		http:    &http.Client{Timeout: requestTimeout},
		creds:   creds,
		mock:    mock,
		mockKey: creds.ZCodeJWT + "|" + creds.PlanToken + "|" + creds.BaseURL,
	}
}

// DefaultBaseURL 返回 coding-plan reset API 的默认主机。
// 核实 ZCode 客户端：status/opportunity/use/history/read 全部走 zcode.z.ai
// （buildRuntimeZCodeApiUrl），与 family 无关；bigmodel.cn / api.z.ai 只承载
// 监控用量接口（model-usage 等），reset 请求发过去会 404。
// NewIdempotencyKey 生成 ZCode 客户端同款幂等键：UUID v4
// （crypto.randomUUID 的 Go 等价实现，见 codingPlanQuotaResetCoordinator.ts
// 的 createIdempotencyKey）。客户端仅在 crypto 不可用时才退回
// `<毫秒时间戳36进制>-<随机36进制>`；上游对 key 校验严格，
// 非 UUID 格式会被 use 接口判 3001 parameter error，故与主路径保持一致。
func NewIdempotencyKey() string {
	b := make([]byte, 16)
	_, _ = crand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10xx
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func DefaultBaseURL(family string) string {
	return "https://zcode.z.ai"
}

func (c *Client) url(path string) string {
	base := strings.TrimRight(c.creds.BaseURL, "/")
	if base == "" {
		base = "https://bigmodel.cn"
	}
	return base + path
}

func (c *Client) headers() map[string]string {
	// Authorization 必须带 Bearer 前缀（ZCode 客户端：不匹配 /^Bearer\s/i 时补前缀）；
	// X-Bigmodel-Authorization 按后端契约直传、不套 Bearer。
	zcodeAuth := c.creds.ZCodeJWT
	if !strings.HasPrefix(zcodeAuth, "Bearer ") {
		zcodeAuth = "Bearer " + zcodeAuth
	}
	h := map[string]string{
		"Authorization":            zcodeAuth,
		"X-Bigmodel-Authorization": c.creds.PlanToken,
		"Accept":                   "application/json",
	}
	if c.creds.TargetType == model.TargetTeam {
		h["Bigmodel-Target-Type"] = model.TargetTeam
		if c.creds.OrgID != "" {
			h["Bigmodel-Organization"] = c.creds.OrgID
		}
		if c.creds.ProjectID != "" {
			h["Bigmodel-Project"] = c.creds.ProjectID
		}
	} else {
		h["Bigmodel-Target-Type"] = model.TargetPersonal
	}
	return h
}

// doJSON 执行请求并按信封语义解码；acceptedCodes 中的业务码原样返回给调用方分支。
func (c *Client) doJSON(ctx context.Context, method, url string, body []byte, acceptedCodes ...int) (*envelope, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	for k, v := range c.headers() {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var env envelope
	parseErr := json.Unmarshal(raw, &env)
	if parseErr == nil && env.Code != 0 {
		for _, code := range acceptedCodes {
			if env.Code == code {
				return &env, nil
			}
		}
		return &env, &BusinessError{Code: env.Code, Message: env.Msg}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &ThrottledError{RetryAfter: 30 * time.Second}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(env.Msg)
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, &BusinessError{Code: resp.StatusCode, Message: msg}
	}
	if parseErr != nil {
		return nil, fmt.Errorf("上游响应不是合法 JSON: %w", parseErr)
	}
	return &env, nil
}

func (c *Client) Status(ctx context.Context) (*Status, error) {
	if c.mock {
		return c.mockStatus(), nil
	}
	env, err := c.doJSON(ctx, http.MethodGet, c.url("/api/v1/coding-plan/reset/status"), nil)
	if err != nil {
		return nil, err
	}
	var st Status
	if err := json.Unmarshal(env.Data, &st); err != nil {
		return nil, fmt.Errorf("status 响应解析失败: %w", err)
	}
	return &st, nil
}

// RequestOpportunity 申领一次重置机会； DeniedError / ThrottledError 供上层写冷却。
func (c *Client) RequestOpportunity(ctx context.Context, idempotencyKey string) (*OpportunityResult, error) {
	if c.mock {
		return c.mockOpportunity(idempotencyKey)
	}
	body, _ := json.Marshal(map[string]string{"idempotency_key": idempotencyKey})
	env, err := c.doJSON(ctx, http.MethodPost, c.url("/api/v1/coding-plan/reset/opportunity"), body, businessDenied)
	if err != nil {
		return nil, err
	}
	if env.Code == businessDenied {
		var denied struct {
			Granted   bool  `json:"granted"`
			NextTryAt int64 `json:"next_try_at"`
		}
		_ = json.Unmarshal(env.Data, &denied)
		return &OpportunityResult{Granted: false, NextTryAt: denied.NextTryAt}, nil
	}
	var granted struct {
		Granted bool `json:"granted"`
	}
	if err := json.Unmarshal(env.Data, &granted); err != nil {
		return nil, fmt.Errorf("opportunity 响应解析失败: %w", err)
	}
	return &OpportunityResult{Granted: granted.Granted}, nil
}

// Use 消耗一次已申领的重置机会。
func (c *Client) Use(ctx context.Context, idempotencyKey, resetType string) error {
	if c.mock {
		return c.mockUse(idempotencyKey, resetType)
	}
	body, _ := json.Marshal(map[string]string{
		"idempotency_key": idempotencyKey,
		"reset_type":      resetType,
	})
	_, err := c.doJSON(ctx, http.MethodPost, c.url("/api/v1/coding-plan/reset/use"), body)
	return err
}

// QuotaLimit 查询 Coding Plan 用量监控接口（只读，不消耗任何额度）。
// 与 reset API 不同：主机按 family 选择（bigmodel→bigmodel.cn / zai→api.z.ai），
// 鉴权是 authorization 头直传业务 Key（不套 Bearer），信封成功码为 200。
func (c *Client) QuotaLimit(ctx context.Context) (*QuotaLimitResult, error) {
	if c.mock {
		usage, used, remaining := 1000, 23, 977
		return &QuotaLimitResult{
			Level: "pro",
			Limits: []UsageLimit{
				{Type: "TOKENS_LIMIT", Unit: 3, Number: 5, Percentage: 30, NextResetTime: time.Now().Add(2 * time.Hour).UnixMilli()},
				{Type: "TOKENS_LIMIT", Unit: 6, Number: 1, Percentage: 12, NextResetTime: time.Now().Add(72 * time.Hour).UnixMilli()},
				{Type: "TIME_LIMIT", Unit: 5, Number: 1, Usage: &usage, CurrentValue: &used, Remaining: &remaining, Percentage: 2, NextResetTime: time.Now().Add(240 * time.Hour).UnixMilli()},
			},
		}, nil
	}
	u := c.usageURL("/api/monitor/usage/quota/limit")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	// monitor 契约：authorization 直传业务 Key，不加 Bearer；Team Plan 另带组织头与 type=2。
	for k, v := range c.usageHeaders() {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("quota/limit 响应不是合法 JSON: %w", err)
	}
	// monitor 信封成功码是 200（区别于 reset API 的 0）。
	if env.Code != 0 && env.Code != 200 {
		return nil, &BusinessError{Code: env.Code, Message: env.Msg}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &BusinessError{Code: resp.StatusCode, Message: env.Msg}
	}
	var payload struct {
		Level  string       `json:"level"`
		Limits []UsageLimit `json:"limits"`
	}
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		return nil, fmt.Errorf("quota/limit 数据解析失败: %w", err)
	}
	return &QuotaLimitResult{Level: payload.Level, Limits: payload.Limits}, nil
}

// usageBase 按 family 选择用量监控主机；BaseURL 覆盖仅作用于 reset API，
// 用量监控没有本地部署场景，始终走 family 对应的官方业务域名。
func (c *Client) usageBase() string {
	if c.creds.Family == "zai" {
		return "https://api.z.ai"
	}
	return "https://bigmodel.cn"
}

func (c *Client) usageURL(path string) string {
	u := c.usageBase() + path
	if c.creds.TargetType == model.TargetTeam {
		u += "?type=2"
	}
	return u
}

func (c *Client) usageHeaders() map[string]string {
	h := map[string]string{
		"authorization": c.creds.PlanToken,
		"Accept":        "application/json",
	}
	if c.creds.TargetType == model.TargetTeam {
		if c.creds.OrgID != "" {
			h["bigmodel-organization"] = c.creds.OrgID
		}
		if c.creds.ProjectID != "" {
			h["bigmodel-project"] = c.creds.ProjectID
		}
	}
	return h
}

// ---------- MOCK 上游（MOCK_UPSTREAM=true，仅开发联调） ----------

type mockState struct {
	opportunities map[string][]Opportunity
	used          []History
}

func (c *Client) mockStatus() *Status {
	ms := sharedMockState(c.mockKey)
	mockMu.Lock()
	defer mockMu.Unlock()
	st := &Status{
		AvailableFiveHour: append([]Opportunity{}, ms.opportunities[model.ResetFiveHour]...),
		AvailableWeek:     append([]Opportunity{}, ms.opportunities[model.ResetWeek]...),
	}
	if len(ms.used) > 0 {
		last := ms.used[len(ms.used)-1]
		st.LatestWeek = &last
		st.HasUnreadHistory = true
	}
	return st
}

func (c *Client) mockOpportunity(string) (*OpportunityResult, error) {
	ms := sharedMockState(c.mockKey)
	mockMu.Lock()
	defer mockMu.Unlock()
	// 真实上游的 opportunity 不区分类型，mock 同时给两类各发一个机会。
	for _, t := range []string{model.ResetFiveHour, model.ResetWeek} {
		ms.opportunities[t] = append(ms.opportunities[t], Opportunity{
			ExpireAt: time.Now().Add(30 * time.Minute).UnixMilli(),
		})
	}
	return &OpportunityResult{Granted: true}, nil
}

func (c *Client) mockUse(_, resetType string) error {
	ms := sharedMockState(c.mockKey)
	mockMu.Lock()
	defer mockMu.Unlock()
	list := ms.opportunities[resetType]
	if len(list) == 0 {
		return &BusinessError{Code: 2001, Message: "没有可用重置机会（mock）"}
	}
	ms.opportunities[resetType] = list[1:]
	ms.used = append(ms.used, History{UsedAt: time.Now().UnixMilli()})
	return nil
}
