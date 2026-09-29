// Package model 定义全部持久化实体与领域常量。
package model

import "time"

const (
	RoleAdmin  = "admin"
	RoleMember = "member"

	StatusActive   = "active"
	StatusDisabled = "disabled"

	FamilyBigModel = "bigmodel"
	FamilyZAI      = "zai"

	TargetPersonal = "PERSONAL"
	TargetTeam     = "TEAM"

	ResetFiveHour = "FIVE_HOUR"
	ResetWeek     = "WEEK"
	ResetAll      = "ALL"

	PeriodDay  = "DAY"
	PeriodWeek = "WEEK"
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	LastLoginAt  time.Time `json:"last_login_at,omitempty"`
	// UpstreamAccountID 预留多账号绑定；空串表示使用默认上游账号。
	UpstreamAccountID string `json:"upstream_account_id,omitempty"`
}

type Session struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	TokenHash  string     `json:"token_hash"`
	IP         string     `json:"ip"`
	UserAgent  string     `json:"user_agent"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type Rule struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id,omitempty"` // 空 = 全局规则
	ResetType string    `json:"reset_type"`        // FIVE_HOUR | WEEK | ALL
	Period    string    `json:"period"`            // DAY | WEEK
	MaxCount  int       `json:"max_count"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

type UpstreamAccount struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Family     string    `json:"family"` // bigmodel | zai
	BaseURL    string    `json:"base_url,omitempty"`
	TargetType string    `json:"target_type"` // PERSONAL | TEAM
	OrgID      string    `json:"org_id,omitempty"`
	ProjectID  string    `json:"project_id,omitempty"`
	ZCodeJWT   string    `json:"zcode_jwt_enc"` // enc:v1:… AES-256-GCM
	PlanToken  string    `json:"plan_token_enc"`
	Enabled    bool      `json:"enabled"`
	IsDefault  bool      `json:"is_default"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type LoginLog struct {
	At        time.Time `json:"at"`
	Username  string    `json:"username"`
	UserID    string    `json:"user_id,omitempty"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	Success   bool      `json:"success"`
	Reason    string    `json:"fail_reason,omitempty"`
}

type ResetLog struct {
	At              time.Time `json:"at"`
	UserID          string    `json:"user_id"`
	Username        string    `json:"username"`
	AccountID       string    `json:"account_id"`
	AccountName     string    `json:"account_name"`
	ResetType       string    `json:"reset_type"`
	Success         bool      `json:"success"`
	OpportunityKey  string    `json:"opportunity_key,omitempty"`
	UseKey          string    `json:"use_key,omitempty"`
	UpstreamCode    int       `json:"upstream_code,omitempty"`
	UpstreamMessage string    `json:"upstream_message,omitempty"`
	ErrorKind       string    `json:"error_kind,omitempty"`
	IP              string    `json:"ip"`
}

type AuditLog struct {
	At       time.Time              `json:"at"`
	AdminID  string                 `json:"admin_id"`
	Admin    string                 `json:"admin"`
	Action   string                 `json:"action"`
	TargetID string                 `json:"target_id,omitempty"`
	Detail   map[string]interface{} `json:"detail,omitempty"`
}
