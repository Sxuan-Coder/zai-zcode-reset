import { useCallback, useEffect, useState } from "react";
import { api, fmtDate } from "../api";
import type { ResetRecord } from "../types";
import { Icons, Loading } from "../ui";

export default function Records() {
  const [records, setRecords] = useState<ResetRecord[] | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const r = await api<{ records: ResetRecord[] }>("/api/reset/history");
      setRecords(r.records ?? []);
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <section className="member-wide-card">
      <div className="recent-heading">
        <h2>全部重置记录</h2>
        {records ? <span className="records-count">共 {records.length} 条</span> : null}
      </div>

      {error ? (
        <div className="form-error" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 10 }}>
          <span>{error}</span>
          <button className="btn btn-ghost btn-sm" onClick={() => void load()}>
            重试
          </button>
        </div>
      ) : !records ? (
        <Loading text="正在加载重置记录…" />
      ) : records.length === 0 ? (
        <div className="empty-state">暂无重置记录，去概览页使用一次吧。</div>
      ) : (
        <div className="table-wrap">
          <table className="recent-table">
            <thead>
              <tr>
                <th>重置类型</th>
                <th>操作时间</th>
                <th>状态</th>
                <th>上游编码</th>
                <th>失败原因</th>
                <th>IP</th>
              </tr>
            </thead>
            <tbody>
              {records.map((r, i) => (
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
                  <td className="record-detail">{r.upstream_code || "—"}</td>
                  <td className="record-detail">{r.error_kind || r.upstream_message || "—"}</td>
                  <td className="record-detail">{r.ip}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
