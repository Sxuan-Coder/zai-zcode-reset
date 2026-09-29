import { useCallback, useEffect, useState } from "react";
import { api, ApiError, fmtTime } from "../api";
import { useAuth } from "../auth";
import type { PeriodQuota, StatusResponse, TypeSnapshot } from "../types";
import { Icons, toast } from "../ui";

/* 依据原型：额度卡片 = 图标 + 标题 + 剩余额度大数字 + 状态点 + 描述 + 全宽按钮 */
function QuotaCard({
  title,
  icon,
  watermark,
  snapshot,
  quota,
  cooldownUntil,
  serverTime,
  busy,
  onUse,
  onDryRun,
  dryBusy,
}: {
  title: string;
  icon: React.ReactNode;
  watermark: React.ReactNode;
  snapshot: TypeSnapshot;
  quota: PeriodQuota;
  cooldownUntil: number;
  serverTime: number;
  busy: boolean;
  onUse: () => void;
  onDryRun?: () => void;
  dryBusy?: boolean;
}) {
  const available = snapshot.available.length;
  const dayExhausted = quota.day_used >= quota.day_limit;
  const weekExhausted = quota.week_used >= quota.week_limit;
  const cooling = cooldownUntil > serverTime;

  let status: React.ReactNode;
  let btnDisabled = false;
  let btnHint = "";

  if (available === 0) {
    status = (
      <span className="q-status none">
        <span className="dot" /> 没重置
      </span>
    );
    btnDisabled = true;
    btnHint = "暂无可用重置机会";
  } else {
    status = (
      <span className="q-status ok">
        <span className="dot" /> 可使用
      </span>
    );
  }
  if (available > 0 && cooling) {
    btnDisabled = true;
    btnHint = `冷却中（至 ${new Date(cooldownUntil).toLocaleTimeString("zh-CN", { hour12: false })}）`;
  }
  if (available > 0 && !cooling && dayExhausted) {
    btnDisabled = true;
    btnHint = "今日次数已用完";
  }
  if (available > 0 && !cooling && !dayExhausted && weekExhausted) {
    btnDisabled = true;
    btnHint = "本周次数已用完";
  }

  return (
    <div className="quota-card">
      <div className="quota-watermark">{watermark}</div>
      <div className="q-head">
        <div className="q-icon">{icon}</div>
        <div className="q-title">{title}</div>
      </div>
      <div className="q-label">剩余重置额度</div>
      <div className="q-value">
        {available}
        <span className="unit">次</span>
      </div>
      {status}
      <div className="q-desc">
        今日已用 {quota.day_used}/{quota.day_limit} · 本周已用 {quota.week_used}/{quota.week_limit}
        <br />
        {snapshot.latest_used_at ? <>最近一次使用：{fmtTime(snapshot.latest_used_at)}</> : <>暂无使用记录</>}
        {available > 0 && (
          <>
            <br />
            机会过期时间：{snapshot.available.map((o) => fmtTime(o.expire_at)).join("、")}
          </>
        )}
      </div>
      <button className="btn btn-primary btn-block q-btn" disabled={btnDisabled || busy} onClick={onUse} title={btnHint}>
        {busy ? "正在重置…" : "去使用"}
      </button>
      {onDryRun ? (
        <button
          className="btn btn-ghost btn-sm btn-block"
          style={{ marginTop: 8 }}
          disabled={dryBusy}
          onClick={onDryRun}
          title="走完整链路校验（登录态/冷却/配额/上游连通），但不调用上游 use，不消耗机会"
        >
          {dryBusy ? "演练中…" : "演练（不消耗机会）"}
        </button>
      ) : null}
    </div>
  );
}

export default function Dashboard() {
  const { me } = useAuth();
  const isAdmin = me?.user.role === "admin";
  const [status, setStatus] = useState<StatusResponse | null>(null);
  const [error, setError] = useState("");
  const [busyType, setBusyType] = useState<string>("");
  const [dryBusyType, setDryBusyType] = useState<string>("");

  const load = useCallback(async () => {
    try {
      setStatus(await api<StatusResponse>("/api/reset/status"));
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), 30_000);
    return () => clearInterval(timer);
  }, [load]);

  const execute = async (resetType: "FIVE_HOUR" | "WEEK", dryRun = false) => {
    if (busyType || dryBusyType) return;
    if (dryRun) {
      setDryBusyType(resetType);
    } else {
      setBusyType(resetType);
    }
    try {
      const resp = await api<StatusResponse>("/api/reset/execute", {
        method: "POST",
        json: { reset_type: resetType, dry_run: dryRun || undefined },
      });
      setStatus(resp);
      const prefix = dryRun ? "[演练] " : "";
      toast(prefix + (resp.last_result?.message || "已执行"), resp.last_result?.success ? "ok" : "warn");
    } catch (err) {
      if (err instanceof ApiError) {
        const nextTry = err.extra?.next_try_at as number | undefined;
        const extra = nextTry ? `（可重试时间：${fmtTime(nextTry)}）` : "";
        toast(`${err.message}${extra}`, "err");
      } else {
        toast("请求失败，请稍后重试", "err");
      }
    } finally {
      setBusyType("");
      setDryBusyType("");
      void load();
    }
  };

  return (
    <div>
      <div className="page-card">
        <div className="product-head">
          <div className="product-logo">Z</div>
          <div>
            <div className="product-title">
              ZCode AI
              {status?.account ? <span className="muted text-sm"> · {status.account.name}</span> : null}
            </div>
            <div className="product-sub">额度重置 · 5 小时限制与周限制</div>
          </div>
        </div>

        <div className="section-title" style={{ marginTop: 0 }}>
          额度重置
        </div>
        <div className="section-desc">使用重置机会前会自动申领，成功后立即生效</div>

        {error ? (
          <div className="form-error" style={{ marginTop: 16 }}>
            {error}
            <button className="btn btn-ghost btn-sm" style={{ marginLeft: 12 }} onClick={() => void load()}>
              重试
            </button>
          </div>
        ) : null}

        {!status && !error ? (
          <div className="page-loading">正在加载重置额度…</div>
        ) : (
          <div className="quota-grid">
            <QuotaCard
              title="5 小时限制"
              icon={
                <span className="q-icon blue">
                  <Icons.clock />
                </span>
              }
              watermark={<Icons.clockBig />}
              snapshot={status?.five_hour ?? { available: [], latest_used_at: null }}
              quota={status?.quota.five_hour ?? { day_used: 0, day_limit: 0, week_used: 0, week_limit: 0 }}
              cooldownUntil={status?.cooldown_until.five_hour ?? 0}
              serverTime={status?.server_time ?? Date.now()}
              busy={busyType === "FIVE_HOUR"}
              onUse={() => void execute("FIVE_HOUR")}
              onDryRun={isAdmin ? () => void execute("FIVE_HOUR", true) : undefined}
              dryBusy={dryBusyType === "FIVE_HOUR"}
            />
            <QuotaCard
              title="周限制"
              icon={
                <span className="q-icon green">
                  <Icons.calendar />
                  <span className="q-refresh-badge">
                    <Icons.refresh />
                  </span>
                </span>
              }
              watermark={<Icons.calendarBig />}
              snapshot={status?.week ?? { available: [], latest_used_at: null }}
              quota={status?.quota.week ?? { day_used: 0, day_limit: 0, week_used: 0, week_limit: 0 }}
              cooldownUntil={status?.cooldown_until.week ?? 0}
              serverTime={status?.server_time ?? Date.now()}
              busy={busyType === "WEEK"}
              onUse={() => void execute("WEEK")}
              onDryRun={isAdmin ? () => void execute("WEEK", true) : undefined}
              dryBusy={dryBusyType === "WEEK"}
            />
          </div>
        )}
      </div>

      <div className="page-card" style={{ marginTop: 20 }}>
        <div className="section-title" style={{ marginTop: 0 }}>
          额度说明
        </div>
        <div className="explain-card" style={{ marginTop: 12 }}>
          <div className="explain-icon">?</div>
          <div className="explain-body">
            <div className="explain-title">什么是重置机会？</div>
            <div className="explain-text">
              上游 Coding Plan 在一个计费窗口内提供有限的「额度重置」机会。有可用机会时点击「去使用」即可立即重置对应窗口的用量；
              当上游暂未发放机会（或机会已过期消耗）时，卡片会显示「没重置」，此时按钮不可点击。平台还会按管理员配置的
              每日 / 每周次数限制你的使用频率。
            </div>
            <div className="explain-demo">
              <div className="mini-card">
                <div className="mini-head">
                  <span className="mini-icon gray">
                    <Icons.clock />
                  </span>
                  <span className="mini-title">5 小时限制（示例）</span>
                </div>
                <div className="q-status none">
                  <span className="dot" /> 没重置
                </div>
                <button className="mini-btn" disabled style={{ width: "100%" }}>
                  去使用
                </button>
              </div>
              <div className="mini-card">
                <div className="mini-head">
                  <span className="mini-icon gray">
                    <Icons.calendar />
                  </span>
                  <span className="mini-title">周限制（示例）</span>
                </div>
                <div className="q-status none">
                  <span className="dot" /> 没重置
                </div>
                <button className="mini-btn" disabled style={{ width: "100%" }}>
                  去使用
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
