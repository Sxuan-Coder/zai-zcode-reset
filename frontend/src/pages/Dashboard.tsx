import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, ApiError, fmtDate, fmtTime } from "../api";
import { useAuth } from "../auth";
import type { ResetRecord, PeriodQuota, StatusResponse, TypeSnapshot } from "../types";
import { Icons, toast, Modal, ConfirmModal } from "../ui";
import { UsageCard } from "../components/UsagePanel";

/* 额度卡片：图标 + 标题 + 状态胶囊 + 剩余机会大数字 + 立即重置 / 演练 */
function QuotaCard({
  title,
  icon,
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

  let btnDisabled = false;
  let btnHint = "";

  if (available === 0) {
    btnDisabled = true;
    btnHint = "暂无可用重置机会";
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

  const status = btnDisabled ? (
    <span className="q-status none">
      <span className="dot" />
      {cooling ? "冷却中" : available === 0 ? "暂无机会" : "已达上限"}
    </span>
  ) : (
    <span className="q-status ok">
      <span className="dot" />
      可使用
    </span>
  );

  return (
    <div className="quota-card">
      <div className="q-head">
        {icon}
        <div className="q-title">{title}</div>
        {status}
      </div>
      <div className="q-label">剩余重置机会</div>
      <div className="q-value">
        {available}
        <span className="unit">次</span>
      </div>
      <div className="q-desc">
        今日已用 {quota.day_used}/{quota.day_limit} · 本周已用 {quota.week_used}/{quota.week_limit}
      </div>
      <div className="quota-actions">
        <button className="btn btn-primary btn-block q-btn" disabled={btnDisabled || busy} onClick={onUse} title={btnHint}>
          {busy ? "正在重置…" : <>立即重置 <span>→</span></>}
        </button>
        {onDryRun ? (
          <button className="btn btn-ghost btn-sm btn-block" disabled={dryBusy} onClick={onDryRun} title="预览重置流程，不调用上游、不消耗机会">
            {dryBusy ? "演练中…" : "演练"}
          </button>
        ) : null}
      </div>
      {btnHint ? <p className="quota-hint">{btnHint}</p> : null}
      <div className="quota-last">
        <span>
          <Icons.refresh /> 最近一次重置
        </span>
        <time>{snapshot.latest_used_at ? fmtTime(snapshot.latest_used_at) : "暂无记录"}</time>
      </div>
      {snapshot.available.length > 0 ? (
        <details className="quota-expiry">
          <summary>机会有效期</summary>
          {snapshot.available.map((o, i) => (
            <div key={i}>{fmtTime(o.expire_at)}</div>
          ))}
        </details>
      ) : null}
    </div>
  );
}

export default function Dashboard() {
  const { me } = useAuth();
  const isAdmin = me?.user.role === "admin";
  const [status, setStatus] = useState<StatusResponse | null>(null);
  const [error, setError] = useState("");
  const [busyType, setBusyType] = useState("");
  const [dryBusyType, setDryBusyType] = useState("");
  const [helpOpen, setHelpOpen] = useState(false);
  const [pending, setPending] = useState<"FIVE_HOUR" | "WEEK" | null>(null);
  const [records, setRecords] = useState<ResetRecord[]>([]);
  const [historyError, setHistoryError] = useState("");
  const [historyLoading, setHistoryLoading] = useState(true);
  const [revision, setRevision] = useState(0);

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

  const loadHistory = useCallback(async () => {
    try {
      const result = await api<{ records: ResetRecord[] }>("/api/reset/history");
      setRecords(result.records ?? []);
      setHistoryError("");
    } catch {
      setHistoryError("最近活动暂时无法读取");
    } finally {
      setHistoryLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadHistory();
  }, [loadHistory, status]);

  const total = status ? status.five_hour.available.length + status.week.available.length : null;
  const busy = Boolean(busyType || dryBusyType);

  const card = (kind: "FIVE_HOUR" | "WEEK") => {
    if (!status) return null;
    const hour = kind === "FIVE_HOUR";
    return (
      <QuotaCard
        key={kind}
        title={hour ? "5 小时额度" : "每周额度"}
        icon={<span className={hour ? "q-icon green" : "q-icon blue"}>{hour ? <Icons.clock /> : <Icons.calendar />}</span>}
        snapshot={hour ? status.five_hour : status.week}
        quota={hour ? status.quota.five_hour : status.quota.week}
        cooldownUntil={hour ? status.cooldown_until.five_hour : status.cooldown_until.week}
        serverTime={status.server_time}
        busy={busy}
        onUse={() => setPending(kind)}
        onDryRun={isAdmin ? () => void execute(kind, true) : undefined}
        dryBusy={busy}
      />
    );
  };

  return (
    <div className="dashboard">
      <section className="flow-hero">
        <div className="flow-copy">
          <div className="flow-kicker">✧ &nbsp; MORE ROOM TO CREATE</div>
          <h2>随时重置，继续你的心流。</h2>
          <p>
            {total === null ? (
              "正在获取你的重置机会。"
            ) : (
              <>
                你还有 <b>{total} 次</b>额度重置机会。
              </>
            )}
            准备好，开始下一次创造。
          </p>
        </div>
        <div className="flow-art" aria-hidden="true">
          <i />
          <i />
          <i />
          <strong>Z</strong>
          <span>✦</span>
          <small>STAY IN THE FLOW</small>
        </div>
      </section>

      <div className="quota-heading">
        <h2>
          额度管理 <small>重置成功后立即生效</small>
        </h2>
        <button className="member-text-button" onClick={() => setHelpOpen(true)}>
          ⓘ &nbsp; 重置规则
        </button>
      </div>

      {error ? (
        <div className="form-error" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 10 }}>
          <span>{error}</span>
          <button className="btn btn-ghost btn-sm" onClick={() => void load()}>
            重试
          </button>
        </div>
      ) : null}

      <div className="dashboard-cards">
        {!status ? (
          <div className="quota-card page-loading" style={{ gridColumn: "1 / -1" }}>
            {error ? "额度暂不可用" : "正在加载重置额度…"}
          </div>
        ) : (
          <>
            {card("FIVE_HOUR")}
            {card("WEEK")}
          </>
        )}
        <UsageCard revision={revision} />
      </div>

      <div className="dashboard-lower">
        <section className="recent-card">
          <div className="recent-heading">
            <h2>最近活动</h2>
            <Link to="/app/records">全部记录　→</Link>
          </div>
          {historyError ? (
            <div className="empty-state">
              {historyError}
              <button className="member-text-button" style={{ marginLeft: 8 }} onClick={() => void loadHistory()}>
                重试
              </button>
            </div>
          ) : historyLoading ? (
            <div className="empty-state">正在加载活动…</div>
          ) : records.length === 0 ? (
            <div className="empty-state">暂无重置记录，下一次继续从这里开始。</div>
          ) : (
            <div className="table-wrap">
              <table className="recent-table">
                <thead>
                  <tr>
                    <th>重置类型</th>
                    <th>操作时间</th>
                    <th>状态</th>
                  </tr>
                </thead>
                <tbody>
                  {records.slice(0, 3).map((r, i) => (
                    <tr key={i}>
                      <td>
                        <span className="record-kind">
                          {r.reset_type === "WEEK" ? <Icons.calendar /> : <Icons.clock />}
                          {r.reset_type === "WEEK" ? "每周额度" : "5 小时额度"}
                        </span>
                      </td>
                      <td>{fmtDate(r.at)}</td>
                      <td>
                        <span className={r.success ? "record-success" : "record-failure"}>● {r.success ? "重置成功" : "重置失败"}</span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section className="guidance-card">
          <div className="member-eyebrow">A LITTLE GUIDANCE</div>
          <h2>把额度用在，灵感涌现时。</h2>
          <p>
            额度重置后即可继续使用。想提前了解流程？
            {isAdmin ? "试试演练模式，不消耗任何重置机会。" : "查看重置规则，安排你的创作节奏。"}
          </p>
          <button className="member-text-button" disabled={busy} onClick={() => (isAdmin ? void execute("FIVE_HOUR", true) : setHelpOpen(true))}>
            {isAdmin ? "体验一次演练" : "了解重置规则"}　→
          </button>
          <div className="guide-leaf" aria-hidden="true">
            <Icons.leaf />
          </div>
        </section>
      </div>

      <Modal open={helpOpen} title="关于额度重置" onClose={() => setHelpOpen(false)}>
        <div className="modal-text">
          <p>重置成功后，对应 Coding Plan 窗口的已使用额度将重置，并消耗一次重置机会。</p>
          <p>每日与每周次数上限以额度卡片显示为准。冷却期间、次数用完或没有可用机会时，暂时无法重置；每次机会的有效期可在卡片中展开查看。</p>
          {isAdmin ? <p>演练会校验登录状态、冷却与配额，不调用上游重置，也不消耗机会。</p> : null}
        </div>
        <button className="btn btn-primary" onClick={() => setHelpOpen(false)}>
          我知道了
        </button>
      </Modal>

      <ConfirmModal
        open={pending !== null}
        title={pending === "WEEK" ? "重置每周额度" : "重置 5 小时额度"}
        text="确认后将消耗 1 次重置机会，成功后立即生效。"
        confirmText="确认重置"
        onCancel={() => setPending(null)}
        onConfirm={() => {
          const kind = pending;
          setPending(null);
          if (kind) void execute(kind).then(() => setRevision((r) => r + 1));
        }}
      />
    </div>
  );
}
