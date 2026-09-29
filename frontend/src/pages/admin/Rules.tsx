import { useCallback, useEffect, useState } from "react";
import { api, fmtDate } from "../../api";
import type { AdminRule, AdminUser } from "../../types";
import { Badge, ConfirmModal, EmptyState, Loading, Switch, toast } from "../../ui";

const TYPE_LABEL: Record<string, string> = { ALL: "全部类型", FIVE_HOUR: "5 小时限制", WEEK: "周限制" };
const PERIOD_LABEL: Record<string, string> = { DAY: "每日", WEEK: "每周" };

export default function AdminRules() {
  const [rules, setRules] = useState<AdminRule[] | null>(null);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [error, setError] = useState("");
  const [form, setForm] = useState({ scope: "global", userId: "", resetType: "ALL", period: "DAY", maxCount: 1 });
  const [creating, setCreating] = useState(false);
  const [confirm, setConfirm] = useState<AdminRule | null>(null);

  const load = useCallback(async () => {
    try {
      const [ruleRes, userRes] = await Promise.all([
        api<{ rules: AdminRule[] }>("/api/admin/rules"),
        api<{ users: AdminUser[] }>("/api/admin/users"),
      ]);
      setRules(ruleRes.rules ?? []);
      setUsers(userRes.users ?? []);
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
      await api("/api/admin/rules", {
        method: "POST",
        json: {
          user_id: form.scope === "user" ? form.userId : "",
          reset_type: form.resetType,
          period: form.period,
          max_count: Number(form.maxCount) || 0,
        },
      });
      toast("规则已创建", "ok");
      setForm({ ...form, maxCount: 1 });
      await load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "创建失败", "err");
    } finally {
      setCreating(false);
    }
  };

  const toggle = async (rule: AdminRule, enabled: boolean) => {
    try {
      await api(`/api/admin/rules/${rule.id}`, { method: "PATCH", json: { enabled } });
      toast(enabled ? "规则已启用" : "规则已停用", "ok");
      await load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "操作失败", "err");
    }
  };

  const remove = async () => {
    if (!confirm) return;
    const target = confirm;
    setConfirm(null);
    try {
      await api(`/api/admin/rules/${target.id}`, { method: "DELETE" });
      toast("规则已删除", "ok");
      await load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "删除失败", "err");
    }
  };

  return (
    <div>
      <div className="page-card">
        <div className="panel-form-title">新增规则</div>
        <div className="panel-form">
          <div className="form-row">
            <div className="field">
              <label>作用域</label>
              <select className="select" value={form.scope} onChange={(e) => setForm({ ...form, scope: e.target.value })}>
                <option value="global">全局（所有人）</option>
                <option value="user">指定用户</option>
              </select>
            </div>
            {form.scope === "user" ? (
              <div className="field">
                <label>用户</label>
                <select className="select" value={form.userId} onChange={(e) => setForm({ ...form, userId: e.target.value })}>
                  <option value="">请选择…</option>
                  {users.map((u) => (
                    <option key={u.id} value={u.id}>
                      {u.username}
                    </option>
                  ))}
                </select>
              </div>
            ) : null}
            <div className="field">
              <label>重置类型</label>
              <select className="select" value={form.resetType} onChange={(e) => setForm({ ...form, resetType: e.target.value })}>
                <option value="ALL">全部类型</option>
                <option value="FIVE_HOUR">5 小时限制</option>
                <option value="WEEK">周限制</option>
              </select>
            </div>
            <div className="field">
              <label>周期</label>
              <select className="select" value={form.period} onChange={(e) => setForm({ ...form, period: e.target.value })}>
                <option value="DAY">每日</option>
                <option value="WEEK">每周</option>
              </select>
            </div>
            <div className="field" style={{ maxWidth: 140 }}>
              <label>次数上限</label>
              <input
                className="input"
                type="number"
                min={0}
                max={1000}
                value={form.maxCount}
                onChange={(e) => setForm({ ...form, maxCount: Number(e.target.value) })}
              />
            </div>
          </div>
          <div className="form-hint" style={{ marginBottom: 10 }}>
            匹配优先级：用户+类型 &gt; 用户+全部 &gt; 全局+类型 &gt; 全局+全部；无任何匹配时上限为 0（默认拒绝）。
          </div>
          <div style={{ display: "flex", justifyContent: "flex-end" }}>
            <button className="btn btn-primary btn-sm" disabled={creating || (form.scope === "user" && !form.userId)} onClick={() => void create()}>
              {creating ? "创建中…" : "创建规则"}
            </button>
          </div>
        </div>
      </div>

      <div className="page-card" style={{ marginTop: 20 }}>
        <div className="admin-head">
          <div className="admin-title">规则列表</div>
        </div>
        {error ? <div className="form-error">{error}</div> : null}
        {!rules && !error ? (
          <Loading />
        ) : rules && rules.length === 0 ? (
          <EmptyState text="暂无规则，请先创建" />
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>作用域</th>
                  <th>重置类型</th>
                  <th>周期</th>
                  <th>次数上限</th>
                  <th>状态</th>
                  <th style={{ textAlign: "right" }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {(rules ?? []).map((r) => (
                  <tr key={r.id}>
                    <td>
                      {r.user_id ? (
                        <>
                          <span style={{ fontWeight: 600 }}>{r.username}</span> <Badge kind="gray">专属</Badge>
                        </>
                      ) : (
                        <Badge kind="violet">全局</Badge>
                      )}
                    </td>
                    <td>{TYPE_LABEL[r.reset_type] ?? r.reset_type}</td>
                    <td>{PERIOD_LABEL[r.period] ?? r.period}</td>
                    <td style={{ fontWeight: 600 }}>{r.max_count} 次</td>
                    <td>
                      <Switch checked={r.enabled} onChange={(v) => void toggle(r, v)} label={r.enabled ? "停用" : "启用"} />
                    </td>
                    <td style={{ textAlign: "right" }}>
                      <button className="btn btn-danger-ghost btn-sm" onClick={() => setConfirm(r)}>
                        删除
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <ConfirmModal
        open={!!confirm}
        danger
        title="删除该规则？"
        text={confirm ? `将删除 ${confirm.user_id ? `用户「${confirm.username}」` : "全局"}的 ${PERIOD_LABEL[confirm.period]}上限规则。` : ""}
        confirmText="确认删除"
        onCancel={() => setConfirm(null)}
        onConfirm={() => void remove()}
      />
    </div>
  );
}
