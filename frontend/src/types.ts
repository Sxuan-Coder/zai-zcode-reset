// 与后端 API 契约对应的类型定义。

export interface SessionInfo {
  id: string;
  ip: string;
  user_agent?: string;
  login_at: string;
  last_seen?: string;
  expires_at: string;
}

export interface MeResponse {
  user: { id: string; username: string; role: string };
  session: SessionInfo;
}

export interface Opportunity {
  expire_at: number;
}

export interface TypeSnapshot {
  available: Opportunity[];
  latest_used_at: number | null;
}

export interface PeriodQuota {
  day_used: number;
  day_limit: number;
  week_used: number;
  week_limit: number;
}

export interface ExecuteOutcome {
  reset_type: string;
  success: boolean;
  message: string;
}

export interface StatusResponse {
  account: { id: string; name: string; family: string; target_type: string };
  five_hour: TypeSnapshot;
  week: TypeSnapshot;
  quota: { five_hour: PeriodQuota; week: PeriodQuota };
  cooldown_until: { five_hour: number; week: number };
  server_time: number;
  last_result?: ExecuteOutcome;
}

// Coding Plan 真实用量（上游 quota/limit 只读接口）
export interface UsageWindow {
  used_percent: number;
  next_reset_at: number;
}

export interface UsageResponse {
  account: string;
  level: string;
  five_hour: UsageWindow | null;
  week: UsageWindow | null;
  tool?: UsageWindow | null;
  tool_info?: { used: number; remaining: number } | null;
}

export interface ResetRecord {
  at: string;
  username: string;
  account_name: string;
  reset_type: string;
  success: boolean;
  upstream_code?: number;
  upstream_message?: string;
  error_kind?: string;
  ip: string;
}

// ---------- 管理后台 ----------

export interface AdminUser {
  id: string;
  username: string;
  role: string;
  status: string;
  created_at: string;
  last_login_at: string;
  upstream_account_id?: string;
}

export interface AdminSession {
  id: string;
  user_id: string;
  username: string;
  ip: string;
  user_agent: string;
  created_at: string;
  last_seen: string;
  expires_at: string;
}

export interface AdminRule {
  id: string;
  user_id?: string;
  username?: string;
  reset_type: string;
  period: string;
  max_count: number;
  enabled: boolean;
}

export interface AdminAccount {
  id: string;
  name: string;
  family: string;
  base_url?: string;
  target_type: string;
  org_id?: string;
  project_id?: string;
  enabled: boolean;
  is_default: boolean;
  zcode_jwt_mask: string;
  plan_token_mask: string;
  created_at: string;
  updated_at: string;
}

export interface LoginLogItem {
  at: string;
  username: string;
  ip: string;
  user_agent: string;
  success: boolean;
  fail_reason?: string;
}

export interface ResetLogItem extends ResetRecord {
  user_id: string;
  account_id: string;
}

export interface AuditLogItem {
  at: string;
  admin: string;
  action: string;
  target_id?: string;
  detail?: Record<string, unknown>;
}

export interface OverviewResponse {
  users: number;
  active_sessions: number;
  accounts: number;
  default_account: string;
  resets_today: number;
  resets_today_all_attempts: number;
  mock_upstream: boolean;
}
