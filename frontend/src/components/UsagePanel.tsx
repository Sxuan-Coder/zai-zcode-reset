import { useCallback, useEffect, useState } from "react";
import { api, fmtTime } from "../api";
import type { UsageResponse } from "../types";

/* Coding Plan 真实用量：来自上游 quota/limit 只读接口，不消耗任何额度；每分钟自动刷新。 */
export function useUsage(revision = 0) {
  const [usage, setUsage] = useState<UsageResponse | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      setUsage(await api<UsageResponse>("/api/reset/usage"));
      setError("");
    } catch {
      setError("用量读取失败");
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), 60_000);
    return () => clearInterval(timer);
  }, [load, revision]);

  return { usage, error };
}

/* 用量圆环：满 80% 转为琥珀色提醒 */
function Ring({ percent, label, nextResetAt, color }: { percent: number; label: string; nextResetAt: number; color: string }) {
  const r = 40;
  const c = 2 * Math.PI * r;
  const p = Math.min(100, Math.max(0, percent));
  const warn = p >= 80;
  return (
    <div className="usage-ring">
      <svg width={96} height={96} viewBox="0 0 96 96">
        <circle cx="48" cy="48" r={r} fill="none" stroke="var(--border)" strokeWidth="7" />
        <circle
          cx="48"
          cy="48"
          r={r}
          fill="none"
          stroke={warn ? "#d3a164" : color}
          strokeWidth="7"
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={c * (1 - p / 100)}
          transform="rotate(-90 48 48)"
          style={{ transition: "stroke-dashoffset 0.6s ease" }}
        />
        <text x="48" y="46" textAnchor="middle" fontSize="18" fontWeight="700" fill="var(--text-1)">
          {Math.round(p)}%
        </text>
        <text x="48" y="62" textAnchor="middle" fontSize="10" fill="var(--text-3)">
          已使用
        </text>
      </svg>
      <div className="usage-ring-label">{label}</div>
      {nextResetAt > 0 ? (
        <div className="usage-ring-sub">
          窗口重置于
          <br />
          {fmtTime(nextResetAt)}
        </div>
      ) : null}
    </div>
  );
}

export function UsageHead({ level }: { level?: string }) {
  return (
    <div className="q-head">
      <div className="q-title">
        Coding Plan 用量
        {level ? <span className="plan-level">{level.toUpperCase()}</span> : null}
      </div>
    </div>
  );
}

export function UsageBody({ usage, error }: { usage: UsageResponse | null; error: string }) {
  if (error) {
    return (
      <div className="q-desc" style={{ flex: 1 }}>
        {error}，稍后自动重试。
      </div>
    );
  }
  if (!usage) return <div className="page-loading">正在读取用量…</div>;
  return (
    <>
      <div className="usage-rings">
        {usage.five_hour ? <Ring percent={usage.five_hour.used_percent} label="5 小时额度" nextResetAt={usage.five_hour.next_reset_at} color="var(--success)" /> : null}
        {usage.week ? <Ring percent={usage.week.used_percent} label="周额度" nextResetAt={usage.week.next_reset_at} color="var(--success)" /> : null}
      </div>
      {usage.tool_info ? (
        <div className="q-desc">
          <span>本月工具调用</span>
          <span>
            {usage.tool_info.used} / {usage.tool_info.used + usage.tool_info.remaining} 次
          </span>
        </div>
      ) : (
        <div className="q-desc">用量达到 80% 时以琥珀色提醒。</div>
      )}
    </>
  );
}

export function UsageCard({ revision = 0 }: { revision?: number }) {
  const { usage, error } = useUsage(revision);
  return (
    <div className="quota-card usage-panel">
      <UsageHead level={usage?.level} />
      <UsageBody usage={usage} error={error} />
    </div>
  );
}
