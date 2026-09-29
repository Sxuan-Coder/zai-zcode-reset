import { useCallback, useEffect, useState } from "react";
import { api, fmtDate } from "../../api";
import type { AdminAccount } from "../../types";
import { Badge, ConfirmModal, EmptyState, Loading, Switch, toast } from "../../ui";

interface AccountForm {
  name: string;
  family: string;
  base_url: string;
  target_type: string;
  org_id: string;
  project_id: string;
  zcode_jwt: string;
  plan_token: string;
  is_default: boolean;
  enabled: boolean;
}

const emptyForm: AccountForm = {
  name: "",
  family: "bigmodel",
  base_url: "",
  target_type: "PERSONAL",
  org_id: "",
  project_id: "",
  zcode_jwt: "",
  plan_token: "",
  is_default: false,
  enabled: true,
};

export default function AdminAccounts() {
  const [accounts, setAccounts] = useState<AdminAccount[] | null>(null);
  const [error, setError] = useState("");
  const [form, setForm] = useState<AccountForm>(emptyForm);
  const [editingId, setEditingId] = useState<string | null>(null); // null=新增模式, ""=收起
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState<AdminAccount | null>(null);
  const [testing, setTesting] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<Record<string, { ok: boolean; message: string }>>({});

  const load = useCallback(async () => {
    try {
      const res = await api<{ accounts: AdminAccount[] }>("/api/admin/accounts");
      setAccounts(res.accounts ?? []);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const openCreate = () => {
    setEditingId("");
    setForm(emptyForm);
  };

  const openEdit = (a: AdminAccount) => {
    setEditingId(a.id);
    setForm({
      name: a.name,
      family: a.family,
      base_url: a.base_url ?? "",
      target_type: a.target_type,
      org_id: a.org_id ?? "",
      project_id: a.project_id ?? "",
      zcode_jwt: "",
      plan_token: "",
      is_default: a.is_default,
      enabled: a.enabled,
    });
  };

  const closeForm = () => {
    setEditingId(null);
    setForm(emptyForm);
  };

  const submit = async () => {
    if (busy) return;
    setBusy(true);
    try {
      if (editingId === "") {
        await api("/api/admin/accounts", { method: "POST", json: form });
        toast("账号已创建", "ok");
      } else {
        const body: Record<string, unknown> = { ...form };
        // 编辑时留空的令牌表示保持不变，不提交空值
        if (!form.zcode_jwt) delete body.zcode_jwt;
        if (!form.plan_token) delete body.plan_token;
        await api(`/api/admin/accounts/${editingId}`, { method: "PATCH", json: body });
        toast("账号已更新", "ok");
      }
      closeForm();
      await load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "保存失败", "err");
    } finally {
      setBusy(false);
    }
  };

  const test = async (a: AdminAccount) => {
    setTesting(a.id);
    try {
      const res = await api<{ ok: boolean; message: string }>(`/api/admin/accounts/${a.id}/test`, { method: "POST" });
      setTestResult((prev) => ({ ...prev, [a.id]: res }));
      toast(res.message, res.ok ? "ok" : "err");
    } catch (err) {
      toast(err instanceof Error ? err.message : "测试失败", "err");
    } finally {
      setTesting(null);
    }
  };

  const toggleEnabled = async (a: AdminAccount, enabled: boolean) => {
    try {
      await api(`/api/admin/accounts/${a.id}`, { method: "PATCH", json: { enabled } });
      toast(enabled ? "账号已启用" : "账号已停用", "ok");
      await load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "操作失败", "err");
    }
  };

  const setDefault = async (a: AdminAccount) => {
    try {
      await api(`/api/admin/accounts/${a.id}`, { method: "PATCH", json: { is_default: true } });
      toast(`已将「${a.name}」设为默认`, "ok");
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
      await api(`/api/admin/accounts/${target.id}`, { method: "DELETE" });
      toast("账号已删除", "ok");
      await load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "删除失败", "err");
    }
  };

  const formOpen = editingId !== null;

  return (
    <div>
      <div className="page-card">
        <div className="admin-head">
          <div className="admin-title">{editingId ? (editingId === "" ? "新增上游账号" : "编辑上游账号") : "上游账号"}</div>
          <div>
            {formOpen ? (
              <button className="btn btn-ghost btn-sm" onClick={closeForm}>
                收起
              </button>
            ) : (
              <button className="btn btn-primary btn-sm" onClick={openCreate}>
                + 新增账号
              </button>
            )}
          </div>
        </div>

        {error ? <div className="form-error">{error}</div> : null}

        {formOpen ? (
          <div className="panel-form">
            <div className="form-row">
              <div className="field">
                <label>名称</label>
                <input className="input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="例如：主力账号" />
              </div>
              <div className="field">
                <label>Family</label>
                <select className="select" value={form.family} onChange={(e) => setForm({ ...form, family: e.target.value })}>
                  <option value="bigmodel">bigmodel（bigmodel.cn）</option>
                  <option value="zai">zai（api.z.ai）</option>
                </select>
              </div>
              <div className="field">
                <label>Target</label>
                <select className="select" value={form.target_type} onChange={(e) => setForm({ ...form, target_type: e.target.value })}>
                  <option value="PERSONAL">PERSONAL（个人）</option>
                  <option value="TEAM">TEAM（团队）</option>
                </select>
              </div>
              <div className="field">
                <label>Base URL（可选）</label>
                <input className="input" value={form.base_url} onChange={(e) => setForm({ ...form, base_url: e.target.value })} placeholder="留空用默认域名" />
              </div>
            </div>
            {form.target_type === "TEAM" ? (
              <div className="form-row">
                <div className="field">
                  <label>Organization ID</label>
                  <input className="input" value={form.org_id} onChange={(e) => setForm({ ...form, org_id: e.target.value })} />
                </div>
                <div className="field">
                  <label>Project ID</label>
                  <input className="input" value={form.project_id} onChange={(e) => setForm({ ...form, project_id: e.target.value })} />
                </div>
              </div>
            ) : null}

            <div style={{ background: "var(--panel)", border: "1px dashed var(--border-strong)", borderRadius: 10, padding: 16, marginBottom: 16 }}>
              <div className="panel-form-title" style={{ marginBottom: 4 }}>
                🔐 上游凭据（服务端 AES-256-GCM 加密存储，永不回显）
              </div>
              <div className="form-hint" style={{ marginBottom: 12 }}>
                {editingId === "" ? "创建时两项均必填" : "留空表示保持原令牌不变"}
              </div>
              <div className="field">
                <label>ZCode JWT（Authorization 头）</label>
                <textarea
                  className="textarea mono"
                  autoComplete="off"
                  value={form.zcode_jwt}
                  onChange={(e) => setForm({ ...form, zcode_jwt: e.target.value })}
                  placeholder={editingId === "" ? "粘贴 zcodejwttoken" : "••••••（保持不变）"}
                />
              </div>
              <div className="field">
                <label>Coding Plan Token（X-Bigmodel-Authorization 头）</label>
                <textarea
                  className="textarea mono"
                  autoComplete="off"
                  value={form.plan_token}
                  onChange={(e) => setForm({ ...form, plan_token: e.target.value })}
                  placeholder={editingId === "" ? "粘贴 oauth access token" : "••••••（保持不变）"}
                />
              </div>
            </div>

            <div style={{ display: "flex", alignItems: "center", gap: 20, justifyContent: "space-between" }}>
              <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                <Switch checked={form.is_default} onChange={(v) => setForm({ ...form, is_default: v })} label="设为默认" />
                <span className="text-sm muted">设为默认（成员未单独绑定时使用）</span>
              </div>
              <div style={{ display: "flex", gap: 10 }}>
                <button className="btn btn-ghost btn-sm" onClick={closeForm}>
                  取消
                </button>
                <button className="btn btn-primary btn-sm" disabled={busy || !form.name.trim() || (editingId === "" && (!form.zcode_jwt.trim() || !form.plan_token.trim()))} onClick={() => void submit()}>
                  {busy ? "保存中…" : editingId === "" ? "创建账号" : "保存修改"}
                </button>
              </div>
            </div>
          </div>
        ) : null}
      </div>

      <div className="page-card" style={{ marginTop: 20 }}>
        <div className="admin-head">
          <div className="admin-title">账号列表</div>
        </div>
        {!accounts && !error ? (
          <Loading />
        ) : accounts && accounts.length === 0 ? (
          <EmptyState text="尚未配置上游账号，新增后成员即可使用重置功能" />
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>Family / Target</th>
                  <th>JWT</th>
                  <th>Plan Token</th>
                  <th>连接</th>
                  <th>启用</th>
                  <th style={{ textAlign: "right" }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {(accounts ?? []).map((a) => (
                  <tr key={a.id}>
                    <td>
                      <span style={{ fontWeight: 600 }}>{a.name}</span> {a.is_default ? <Badge kind="violet">默认</Badge> : null}
                      <div className="text-sm muted">更新于 {fmtDate(a.updated_at)}</div>
                    </td>
                    <td>
                      <Badge kind="blue">{a.family}</Badge> <Badge kind="gray">{a.target_type}</Badge>
                    </td>
                    <td className="mono text-sm">{a.zcode_jwt_mask}</td>
                    <td className="mono text-sm">{a.plan_token_mask}</td>
                    <td>
                      {testResult[a.id] ? (
                        testResult[a.id].ok ? (
                          <Badge kind="green">正常</Badge>
                        ) : (
                          <Badge kind="red">异常</Badge>
                        )
                      ) : (
                        <Badge kind="gray">未测试</Badge>
                      )}
                    </td>
                    <td>
                      <Switch checked={a.enabled} onChange={(v) => void toggleEnabled(a, v)} />
                    </td>
                    <td>
                      <div className="actions" style={{ justifyContent: "flex-end" }}>
                        <button className="btn btn-ghost btn-sm" disabled={testing === a.id} onClick={() => void test(a)}>
                          {testing === a.id ? "测试中…" : "测试连接"}
                        </button>
                        {!a.is_default ? (
                          <button className="btn btn-ghost btn-sm" onClick={() => void setDefault(a)}>
                            设为默认
                          </button>
                        ) : null}
                        <button className="btn btn-ghost btn-sm" onClick={() => openEdit(a)}>
                          编辑
                        </button>
                        <button className="btn btn-danger-ghost btn-sm" onClick={() => setConfirm(a)}>
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

      <ConfirmModal
        open={!!confirm}
        danger
        title={`删除上游账号 "${confirm?.name}"？`}
        text="成员将无法再通过该账号执行重置（若为默认账号需先指定新的默认）。已加密的令牌会被一并删除。"
        confirmText="确认删除"
        onCancel={() => setConfirm(null)}
        onConfirm={() => void remove()}
      />
    </div>
  );
}
