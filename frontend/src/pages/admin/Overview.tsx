import { useEffect, useState } from "react";
import { api } from "../../api";
import type { OverviewResponse } from "../../types";
import { Badge, Loading } from "../../ui";

export default function AdminOverview() {
  const [data, setData] = useState<OverviewResponse | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api<OverviewResponse>("/api/admin/overview")
      .then(setData)
      .catch((err) => setError(err instanceof Error ? err.message : "加载失败"));
  }, []);

  if (error) return <div className="page-card"><div className="form-error">{error}</div></div>;
  if (!data) return <Loading />;

  return (
    <div>
      {data.mock_upstream ? (
        <div className="form-error" style={{ background: "var(--warn-bg)", color: "var(--warn)" }}>
          ⚠ 当前运行在 MOCK_UPSTREAM 模式，所有上游交互均为模拟数据，请勿用于生产。
        </div>
      ) : null}
      <div className="stat-grid">
        <div className="stat-card">
          <div className="stat-label">用户数</div>
          <div className="stat-value">{data.users}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">活跃会话</div>
          <div className="stat-value">{data.active_sessions}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">今日成功重置</div>
          <div className="stat-value">{data.resets_today}</div>
          <div className="stat-extra">今日总尝试 {data.resets_today_all_attempts} 次</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">上游账号</div>
          <div className="stat-value">{data.accounts}</div>
          <div className="stat-extra">
            默认账号：{data.default_account ? <Badge kind="violet">{data.default_account}</Badge> : "未配置"}
          </div>
        </div>
      </div>
      <div className="page-card">
        <div className="admin-head">
          <div className="admin-title">运行状态</div>
        </div>
        <dl className="kv-list">
          <dt>重置链路</dt>
          <dd>{data.accounts > 0 ? <Badge kind="green">已就绪</Badge> : <Badge kind="amber">请先配置上游账号</Badge>}</dd>
          <dt>默认账号</dt>
          <dd>{data.default_account || "—"}</dd>
          <dt>配额规则</dt>
          <dd>在「重置规则」中配置全局默认与按用户上限</dd>
        </dl>
      </div>
    </div>
  );
}
