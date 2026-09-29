import { useCallback, useEffect, useState } from "react";
import { api, fmtDate } from "../../api";
import type { AdminSession } from "../../types";
import { ConfirmModal, EmptyState, Loading, toast } from "../../ui";

export default function AdminSessions() {
  const [sessions, setSessions] = useState<AdminSession[] | null>(null);
  const [error, setError] = useState("");
  const [confirm, setConfirm] = useState<AdminSession | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await api<{ sessions: AdminSession[] }>("/api/admin/sessions");
      setSessions(res.sessions ?? []);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), 15_000);
    return () => clearInterval(timer);
  }, [load]);

  const revoke = async () => {
    if (!confirm) return;
    const target = confirm;
    setConfirm(null);
    try {
      await api(`/api/admin/sessions/${target.id}`, { method: "DELETE" });
      toast(`已下线 ${target.username}（${target.ip}）`, "ok");
      await load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "操作失败", "err");
    }
  };

  return (
    <div className="page-card">
      <div className="admin-head">
        <div className="admin-title">活跃会话</div>
        <span className="text-sm muted">每 15 秒自动刷新 · 单点登录开启时同一用户新登录会顶替旧会话</span>
      </div>
      {error ? <div className="form-error">{error}</div> : null}
      {!sessions && !error ? (
        <Loading />
      ) : sessions && sessions.length === 0 ? (
        <EmptyState text="当前没有活跃会话" />
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>用户</th>
                <th>IP</th>
                <th>User-Agent</th>
                <th>登录时间</th>
                <th>最后活跃</th>
                <th style={{ textAlign: "right" }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {(sessions ?? []).map((s) => (
                <tr key={s.id}>
                  <td style={{ fontWeight: 600 }}>{s.username || <span className="muted">已删除用户</span>}</td>
                  <td className="mono">{s.ip}</td>
                  <td className="mono text-sm" style={{ maxWidth: 280, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={s.user_agent}>
                    {s.user_agent || "—"}
                  </td>
                  <td className="mono">{fmtDate(s.created_at)}</td>
                  <td className="mono">{fmtDate(s.last_seen)}</td>
                  <td style={{ textAlign: "right" }}>
                    <button className="btn btn-danger-ghost btn-sm" onClick={() => setConfirm(s)}>
                      下线
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <ConfirmModal
        open={!!confirm}
        danger
        title={`下线 "${confirm?.username}" 的会话？`}
        text={`该会话来自 ${confirm?.ip}，下线后需要重新登录。`}
        confirmText="确认下线"
        onCancel={() => setConfirm(null)}
        onConfirm={() => void revoke()}
      />
    </div>
  );
}
