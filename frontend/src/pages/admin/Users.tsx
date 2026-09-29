import { useCallback, useEffect, useState } from "react";
import { api, fmtDate } from "../../api";
import type { AdminUser } from "../../types";
import { Badge, ConfirmModal, EmptyState, Loading, Modal, toast } from "../../ui";

export default function AdminUsers() {
  const [users, setUsers] = useState<AdminUser[] | null>(null);
  const [error, setError] = useState("");
  const [form, setForm] = useState({ username: "", password: "", role: "member" });
  const [creating, setCreating] = useState(false);
  // 一次性密码弹窗（按规范：仅显示一次，无关闭遮罩跳过路径）
  const [initialPassword, setInitialPassword] = useState<{ username: string; password: string } | null>(null);
  const [confirm, setConfirm] = useState<{ kind: "disable" | "enable" | "delete" | "logout" | "pwreset"; user: AdminUser } | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await api<{ users: AdminUser[] }>("/api/admin/users");
      setUsers(res.users ?? []);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const create = async () => {
    if (creating) return;
    setCreating(true);
    try {
      const res = await api<{ user: AdminUser; initial_password?: string }>("/api/admin/users", {
        method: "POST",
        json: { username: form.username.trim(), password: form.password || undefined, role: form.role },
      });
      toast(`用户 ${res.user.username} 已创建`, "ok");
      if (res.initial_password) {
        setInitialPassword({ username: res.user.username, password: res.initial_password });
      }
      setForm({ username: "", password: "", role: "member" });
      await load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "创建失败", "err");
    } finally {
      setCreating(false);
    }
  };

  const patchUser = async (user: AdminUser, body: Record<string, unknown>, okMsg: string) => {
    setBusy(true);
    try {
      await api(`/api/admin/users/${user.id}`, { method: "PATCH", json: body });
      toast(okMsg, "ok");
      await load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "操作失败", "err");
    } finally {
      setBusy(false);
    }
  };

  const doConfirm = async () => {
    if (!confirm) return;
    const { kind, user } = confirm;
    setConfirm(null);
    setBusy(true);
    try {
      if (kind === "disable") await patchUser(user, { status: "disabled" }, `已停用 ${user.username}`);
      else if (kind === "enable") await patchUser(user, { status: "active" }, `已启用 ${user.username}`);
      else if (kind === "logout") {
        const res = await api<{ revoked: number }>(`/api/admin/users/${user.id}/revoke-sessions`, { method: "POST" });
        toast(`已下线 ${user.username} 的 ${res.revoked} 个会话`, "ok");
      } else if (kind === "pwreset") {
        const pw = Math.random().toString(36).slice(2, 10) + Math.random().toString(36).slice(2, 6);
        await patchUser(user, { password: pw }, `已重置 ${user.username} 的密码`);
        setInitialPassword({ username: user.username, password: pw });
      } else if (kind === "delete") {
        await api(`/api/admin/users/${user.id}`, { method: "DELETE" });
        toast(`已删除 ${user.username}`, "ok");
        await load();
      }
    } catch (err) {
      toast(err instanceof Error ? err.message : "操作失败", "err");
    } finally {
      setBusy(false);
    }
  };

  const confirmText: Record<string, { title: string; text?: string; confirmText: string }> = {
    disable: { title: `停用用户 "${confirm?.user.username}"？`, text: "停用后该用户所有会话立即失效，无法登录。", confirmText: "确认停用" },
    enable: { title: `启用用户 "${confirm?.user.username}"？`, confirmText: "确认启用" },
    delete: { title: `删除用户 "${confirm?.user.username}"？`, text: "删除后其会话与专属规则一并清除，登录日志保留。", confirmText: "确认删除" },
    logout: { title: `强制下线 "${confirm?.user.username}"？`, text: "该用户当前全部活跃会话将被吊销。", confirmText: "强制下线" },
    pwreset: { title: `重置 "${confirm?.user.username}" 的密码？`, text: "将生成新密码（显示一次），该用户全部会话下线。", confirmText: "重置密码" },
  };

  return (
    <div>
      <div className="page-card">
        <div className="panel-form-title">新增用户</div>
        <div className="panel-form">
          <div className="form-row">
            <div className="field">
              <label>用户名</label>
              <input className="input" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} placeholder="3-32 位字母/数字/_-，创建后不可改" />
            </div>
            <div className="field">
              <label>初始密码</label>
              <input className="input" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} placeholder="留空自动生成" autoComplete="new-password" />
              <div className="form-hint">留空将生成随机密码，仅显示一次</div>
            </div>
            <div className="field" style={{ maxWidth: 160 }}>
              <label>角色</label>
              <select className="select" value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
                <option value="member">成员</option>
                <option value="admin">管理员</option>
              </select>
            </div>
          </div>
          <div style={{ display: "flex", justifyContent: "flex-end" }}>
            <button className="btn btn-primary btn-sm" disabled={creating || !form.username.trim()} onClick={() => void create()}>
              {creating ? "创建中…" : "创建用户"}
            </button>
          </div>
        </div>
      </div>

      <div className="page-card" style={{ marginTop: 20 }}>
        <div className="admin-head">
          <div className="admin-title">用户列表</div>
        </div>
        {error ? <div className="form-error">{error}</div> : null}
        {!users && !error ? (
          <Loading />
        ) : users && users.length === 0 ? (
          <EmptyState text="还没有用户" />
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>用户名</th>
                  <th>角色</th>
                  <th>状态</th>
                  <th>创建时间</th>
                  <th>最后登录</th>
                  <th style={{ textAlign: "right" }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {(users ?? []).map((u) => (
                  <tr key={u.id}>
                    <td style={{ fontWeight: 600 }}>{u.username}</td>
                    <td>{u.role === "admin" ? <Badge kind="blue">管理员</Badge> : <Badge kind="gray">成员</Badge>}</td>
                    <td>{u.status === "active" ? <Badge kind="green">启用</Badge> : <Badge kind="red">停用</Badge>}</td>
                    <td className="mono">{fmtDate(u.created_at)}</td>
                    <td className="mono">{u.last_login_at ? fmtDate(u.last_login_at) : "—"}</td>
                    <td>
                      <div className="actions" style={{ justifyContent: "flex-end" }}>
                        {u.status === "active" ? (
                          <button className="btn btn-ghost btn-sm" disabled={busy} onClick={() => setConfirm({ kind: "disable", user: u })}>
                            停用
                          </button>
                        ) : (
                          <button className="btn btn-ghost btn-sm" disabled={busy} onClick={() => setConfirm({ kind: "enable", user: u })}>
                            启用
                          </button>
                        )}
                        <button className="btn btn-ghost btn-sm" disabled={busy} onClick={() => setConfirm({ kind: "pwreset", user: u })}>
                          重置密码
                        </button>
                        <button className="btn btn-ghost btn-sm" disabled={busy} onClick={() => setConfirm({ kind: "logout", user: u })}>
                          强制下线
                        </button>
                        <button className="btn btn-danger-ghost btn-sm" disabled={busy} onClick={() => setConfirm({ kind: "delete", user: u })}>
                          删除
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* 一次性密码：不可点击遮罩关闭，必须点「我已安全保存」 */}
      <Modal open={!!initialPassword} title="请立即保存初始密码">
        <div className="modal-text">
          <span style={{ color: "var(--warn)", fontWeight: 600 }}>此密码仅显示一次</span>，关闭后无法再次查看。请复制并安全地交给用户
          「{initialPassword?.username}」。
        </div>
        <div
          className="mono"
          style={{ background: "var(--panel-2)", border: "1px solid var(--border)", borderRadius: 10, padding: "14px 16px", fontSize: 18, textAlign: "center", userSelect: "all" }}
        >
          {initialPassword?.password}
        </div>
        <div className="modal-actions" style={{ marginTop: 16 }}>
          <button
            className="btn btn-ghost btn-sm"
            onClick={() => {
              if (initialPassword) {
                void navigator.clipboard?.writeText(initialPassword.password).then(
                  () => toast("已复制到剪贴板", "ok"),
                  () => toast("复制失败，请手动选择", "warn"),
                );
              }
            }}
          >
            复制
          </button>
          <button className="btn btn-primary btn-sm" autoFocus onClick={() => setInitialPassword(null)}>
            我已安全保存
          </button>
        </div>
      </Modal>

      <ConfirmModal
        open={!!confirm}
        danger={confirm?.kind === "delete" || confirm?.kind === "disable"}
        title={confirm ? confirmText[confirm.kind].title : ""}
        text={confirm ? confirmText[confirm.kind].text : undefined}
        confirmText={confirm ? confirmText[confirm.kind].confirmText : "确认"}
        onCancel={() => setConfirm(null)}
        onConfirm={() => void doConfirm()}
      />
    </div>
  );
}
