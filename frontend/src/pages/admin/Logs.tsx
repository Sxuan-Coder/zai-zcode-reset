import { useCallback, useEffect, useState } from "react";
import { api, fmtDate } from "../../api";
import type { AuditLogItem, LoginLogItem, ResetLogItem } from "../../types";
import { Badge, EmptyState, Loading } from "../../ui";

type Kind = "login" | "reset" | "audit";

const TABS: { key: Kind; label: string }[] = [
  { key: "login", label: "登录日志" },
  { key: "reset", label: "重置日志" },
  { key: "audit", label: "管理审计" },
];

export default function AdminLogs() {
  const [kind, setKind] = useState<Kind>("login");
  const [logs, setLogs] = useState<unknown[] | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(async (k: Kind) => {
    setLogs(null);
    setError("");
    try {
      const res = await api<{ logs: unknown[] }>(`/api/admin/logs/${k}?limit=200`);
      setLogs(res.logs ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    }
  }, []);

  useEffect(() => {
    void load(kind);
  }, [kind, load]);

  return (
    <div className="page-card">
      <div className="admin-head">
        <div className="admin-title">操作日志</div>
      </div>
      <div className="tabs">
        {TABS.map((t) => (
          <button key={t.key} className={`tab${kind === t.key ? " active" : ""}`} onClick={() => setKind(t.key)}>
            {t.label}
          </button>
        ))}
      </div>

      {error ? <div className="form-error">{error}</div> : null}
      {!logs && !error ? <Loading /> : null}

      {logs && kind === "login" && (
        <LogTable empty="暂无登录记录" count={logs.length}>
          {(logs as LoginLogItem[]).map((l, i) => (
            <tr key={i}>
              <td className="mono">{fmtDate(l.at)}</td>
              <td style={{ fontWeight: 600 }}>{l.username}</td>
              <td className="mono">{l.ip}</td>
              <td>{l.success ? <Badge kind="green">成功</Badge> : <Badge kind="red">失败</Badge>}</td>
              <td className="text-sm muted">{l.success ? "—" : l.fail_reason ?? "未知"}</td>
            </tr>
          ))}
        </LogTable>
      )}

      {logs && kind === "reset" && (
        <LogTable empty="暂无重置记录" count={logs.length} typeCol>
          {(logs as ResetLogItem[]).map((l, i) => (
            <tr key={i}>
              <td className="mono">{fmtDate(l.at)}</td>
              <td style={{ fontWeight: 600 }}>{l.username}</td>
              <td>{l.reset_type === "WEEK" ? "周限制" : "5 小时"}</td>
              <td>{l.success ? <Badge kind="green">成功</Badge> : <Badge kind="red">失败</Badge>}</td>
              <td className="mono">{l.upstream_code || "—"}</td>
              <td className="text-sm muted">{l.error_kind || "—"}</td>
              <td className="mono">{l.ip}</td>
            </tr>
          ))}
        </LogTable>
      )}

      {logs && kind === "audit" && (
        <LogTable empty="暂无审计记录" count={logs.length}>
          {(logs as AuditLogItem[]).map((l, i) => (
            <tr key={i}>
              <td className="mono">{fmtDate(l.at)}</td>
              <td style={{ fontWeight: 600 }}>{l.admin}</td>
              <td>
                <Badge kind="blue">{l.action}</Badge>
              </td>
              <td className="mono">{l.target_id || "—"}</td>
              <td className="mono text-sm">{l.detail ? JSON.stringify(l.detail) : "—"}</td>
            </tr>
          ))}
        </LogTable>
      )}
    </div>
  );
}

function LogTable({
  children,
  empty,
  count,
  typeCol = false,
}: {
  children: React.ReactNode;
  empty: string;
  count: number;
  typeCol?: boolean;
}) {
  if (count === 0) return <EmptyState text={empty} />;
  const heads = {
    login: ["时间", "用户名", "IP", "结果", "失败原因"],
    reset: ["时间", "用户名", "类型", "结果", "上游码", "错误类型", "IP"],
    audit: ["时间", "操作者", "动作", "对象", "详情"],
  };
  const kind = typeCol ? "reset" : "login";
  return (
    <div className="table-wrap">
      <table className="table">
        <thead>
          <tr>
            {(heads[kind] as string[]).map((h) => (
              <th key={h}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  );
}
