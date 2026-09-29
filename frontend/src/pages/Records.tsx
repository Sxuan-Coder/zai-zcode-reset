import { useEffect, useState } from "react";
import { api, fmtDate } from "../api";
import type { ResetRecord } from "../types";
import { Badge, EmptyState, Loading } from "../ui";

export default function Records() {
  const [records, setRecords] = useState<ResetRecord[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api<{ records: ResetRecord[] }>("/api/reset/history")
      .then((r) => setRecords(r.records ?? []))
      .catch((err) => setError(err instanceof Error ? err.message : "加载失败"));
  }, []);

  return (
    <div className="page-card">
      <div className="admin-head">
        <div className="admin-title">重置记录</div>
      </div>
      {error ? <div className="form-error">{error}</div> : null}
      {!records && !error ? (
        <Loading />
      ) : records && records.length === 0 ? (
        <EmptyState text="暂无重置记录，去概览页使用一次吧" big="⏱" />
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>时间</th>
                <th>类型</th>
                <th>结果</th>
                <th>上游编码</th>
                <th>失败原因</th>
                <th>IP</th>
              </tr>
            </thead>
            <tbody>
              {(records ?? []).map((r, i) => (
                <tr key={i}>
                  <td className="mono">{fmtDate(r.at)}</td>
                  <td>{r.reset_type === "WEEK" ? "周限制" : "5 小时限制"}</td>
                  <td>
                    {r.success ? <Badge kind="green">成功</Badge> : <Badge kind="red">失败</Badge>}
                  </td>
                  <td className="mono">{r.upstream_code || "—"}</td>
                  <td className="text-sm muted">{r.error_kind || r.upstream_message || "—"}</td>
                  <td className="mono">{r.ip}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
