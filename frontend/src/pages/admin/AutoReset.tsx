import { useCallback, useEffect, useState } from "react";
import { api } from "../../api";
import type { AutoResetAccount, AutoResetSettings } from "../../types";
import { Badge, EmptyState, Loading, Modal, Switch, toast } from "../../ui";

interface AccountEdit {
  account: AutoResetAccount;
  mode: "follow" | "override";
  form: AutoResetSettings;
}

export default function AdminAutoReset() {
  const [global, setGlobal] = useState<AutoResetSettings | null>(null);
  const [accounts, setAccounts] = useState<AutoResetAccount[] | null>(null);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [edit, setEdit] = useState<AccountEdit | null>(null);
  const [editSaving, setEditSaving] = useState(false);

  const load = useCallback(async () => {
    try {
      const res = await api<{ global: AutoResetSettings; accounts: AutoResetAccount[] }>("/api/admin/auto-reset");
      setGlobal(res.global);
      setAccounts(res.accounts ?? []);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async () => {
    if (!global || saving) return;
    setSaving(true);
    try {
      const res = await api<{ global: AutoResetSettings }>("/api/admin/auto-reset", {
        method: "PUT",
        json: {
          enabled: global.enabled,
          lead_minutes: Number(global.lead_minutes),
          five_hour_threshold: Number(global.five_hour_threshold),
          week_threshold: Number(global.week_threshold),
        },
      });
      setGlobal(res.global);
      toast("全局配置已保存", "ok");
      await load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "保存失败", "err");
    } finally {
      setSaving(false);
    }
  };

  const openEdit = (a: AutoResetAccount) => {
    setEdit({
      account: a,
      mode: a.source === "override" ? "override" : "follow",
      form: a.override ?? a.effective,
    });
  };

  const saveAccount = async () => {
    if (!edit || editSaving) return;
    setEditSaving(true);
    try {
      const json =
        edit.mode === "follow"
          ? { auto_reset: null }
          : {
              auto_reset: {
                enabled: edit.form.enabled,
                lead_minutes: Number(edit.form.lead_minutes),
                five_hour_threshold: Number(edit.form.five_hour_threshold),
                week_threshold: Number(edit.form.week_threshold),
              },
            };
      await api(`/api/admin/accounts/${edit.account.id}`, { method: "PATCH", json });
      toast("账号配置已保存", "ok");
      setEdit(null);
      await load();
    } catch (err) {
      toast(err instanceof Error ? err.message : "保存失败", "err");
    } finally {
      setEditSaving(false);
    }
  };

  const num = (v: string) => (v === "" ? 0 : Number(v));
  const summary = (c: AutoResetSettings) => `提前 ${c.lead_minutes} 分钟 · 5h 剩余 <${c.five_hour_threshold}% · 周 剩余 <${c.week_threshold}%`;

  return (
    <div>
      <div className="page-card">
        <div className="panel-form-title">全局默认配置</div>
        {error ? <div className="form-error">{error}</div> : null}
        {!global && !error ? (
          <Loading />
        ) : global ? (
          <div className="panel-form">
            <div className="form-row">
              <div className="field">
                <label>启用自动重置</label>
                <Switch checked={global.enabled} onChange={(v) => setGlobal({ ...global, enabled: v })} label="启用" />
              </div>
              <div className="field" style={{ maxWidth: 150 }}>
                <label>提前量（分钟）</label>
                <input
                  className="input"
                  type="number"
                  min={1}
                  max={60}
                  value={global.lead_minutes}
                  onChange={(e) => setGlobal({ ...global, lead_minutes: num(e.target.value) })}
                />
              </div>
              <div className="field" style={{ maxWidth: 170 }}>
                <label>5 小时卡阈值（剩余 %）</label>
                <input
                  className="input"
                  type="number"
                  min={50}
                  max={99}
                  value={global.five_hour_threshold}
                  onChange={(e) => setGlobal({ ...global, five_hour_threshold: num(e.target.value) })}
                />
              </div>
              <div className="field" style={{ maxWidth: 170 }}>
                <label>周卡阈值（剩余 %）</label>
                <input
                  className="input"
                  type="number"
                  min={50}
                  max={99}
                  value={global.week_threshold}
                  onChange={(e) => setGlobal({ ...global, week_threshold: num(e.target.value) })}
                />
              </div>
            </div>
            <div className="form-hint" style={{ marginBottom: 10 }}>
              重置卡到期前「提前量」分钟进入触发窗口；对应窗口剩余额度低于阈值（额度确实被消耗过）时自动用卡重置，
              剩余不低于阈值则不动。自动重置不占用成员的每日/每周重置次数，只在重置日志中标记「自动」。
            </div>
            <div style={{ display: "flex", justifyContent: "flex-end" }}>
              <button className="btn btn-primary btn-sm" disabled={saving} onClick={() => void save()}>
                {saving ? "保存中…" : "保存全局配置"}
              </button>
            </div>
          </div>
        ) : null}
      </div>

      <div className="page-card" style={{ marginTop: 20 }}>
        <div className="admin-head">
          <div className="admin-title">账号配置</div>
        </div>
        {!accounts && !error ? (
          <Loading />
        ) : accounts && accounts.length === 0 ? (
          <EmptyState text="尚未配置上游账号" />
        ) : (
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>账号</th>
                  <th>账号状态</th>
                  <th>自动重置</th>
                  <th>生效配置</th>
                  <th>来源</th>
                  <th style={{ textAlign: "right" }}>操作</th>
                </tr>
              </thead>
              <tbody>
                {(accounts ?? []).map((a) => (
                  <tr key={a.id}>
                    <td style={{ fontWeight: 600 }}>{a.name}</td>
                    <td>{a.enabled ? <Badge kind="green">启用</Badge> : <Badge kind="gray">停用</Badge>}</td>
                    <td>
                      {!a.enabled ? (
                        <span className="muted text-sm">—</span>
                      ) : a.effective.enabled ? (
                        <Badge kind="green">开启</Badge>
                      ) : (
                        <Badge kind="gray">关闭</Badge>
                      )}
                    </td>
                    <td className="text-sm">{!a.enabled || !a.effective.enabled ? "—" : summary(a.effective)}</td>
                    <td>{a.source === "override" ? <Badge kind="violet">账号覆盖</Badge> : <Badge kind="gray">跟随全局</Badge>}</td>
                    <td style={{ textAlign: "right" }}>
                      <button className="btn btn-ghost btn-sm" onClick={() => openEdit(a)}>
                        配置覆盖
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <Modal open={!!edit} title={edit ? `「${edit.account.name}」自动重置覆盖` : ""} onClose={() => setEdit(null)} width={480}>
        {edit ? (
          <div style={{ marginTop: 4 }}>
            <div className="field">
              <label>配置来源</label>
              <select
                className="select"
                value={edit.mode}
                onChange={(e) => setEdit({ ...edit, mode: e.target.value as AccountEdit["mode"] })}
              >
                <option value="follow">跟随全局</option>
                <option value="override">单独覆盖</option>
              </select>
            </div>
            {edit.mode === "override" ? (
              <div className="modal-form-grid">
                <div className="field">
                  <label>启用</label>
                  <Switch checked={edit.form.enabled} onChange={(v) => setEdit({ ...edit, form: { ...edit.form, enabled: v } })} label="启用" />
                </div>
                <div className="field">
                  <label>提前量（分钟）</label>
                  <input
                    className="input"
                    type="number"
                    min={1}
                    max={60}
                    value={edit.form.lead_minutes}
                    onChange={(e) => setEdit({ ...edit, form: { ...edit.form, lead_minutes: num(e.target.value) } })}
                  />
                </div>
                <div className="field">
                  <label>5 小时卡阈值（剩余 %）</label>
                  <input
                    className="input"
                    type="number"
                    min={50}
                    max={99}
                    value={edit.form.five_hour_threshold}
                    onChange={(e) => setEdit({ ...edit, form: { ...edit.form, five_hour_threshold: num(e.target.value) } })}
                  />
                </div>
                <div className="field">
                  <label>周卡阈值（剩余 %）</label>
                  <input
                    className="input"
                    type="number"
                    min={50}
                    max={99}
                    value={edit.form.week_threshold}
                    onChange={(e) => setEdit({ ...edit, form: { ...edit.form, week_threshold: num(e.target.value) } })}
                  />
                </div>
              </div>
            ) : (
              <div className="form-hint" style={{ marginBottom: 14 }}>清除覆盖后，该账号使用上面的全局默认配置。</div>
            )}
            <div className="modal-actions">
              <button className="btn btn-ghost btn-sm" onClick={() => setEdit(null)}>
                取消
              </button>
              <button className="btn btn-primary btn-sm" disabled={editSaving} onClick={() => void saveAccount()}>
                {editSaving ? "保存中…" : "保存"}
              </button>
            </div>
          </div>
        ) : null}
      </Modal>
    </div>
  );
}
