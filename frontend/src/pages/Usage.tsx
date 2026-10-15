import { UsageBody, UsageHead, useUsage } from "../components/UsagePanel";

const pct = (value: number | undefined) => (value === undefined ? "—" : `${Math.round(value)}%`);

export default function Usage() {
  const { usage, error } = useUsage();
  const tool = usage?.tool_info;

  return (
    <div className="usage-page">
      <div className="usage-summary">
        <article className="member-metric">
          <span>5 小时额度</span>
          <strong>{pct(usage?.five_hour?.used_percent)}</strong>
          <p>当前窗口已使用</p>
        </article>
        <article className="member-metric">
          <span>每周额度</span>
          <strong>{pct(usage?.week?.used_percent)}</strong>
          <p>本周累计已使用</p>
        </article>
        <article className="member-metric">
          <span>本月工具调用</span>
          <strong>
            {tool ? tool.used : "—"}
            {tool ? <small> / {tool.used + tool.remaining} 次</small> : null}
          </strong>
          <p>{tool ? `剩余 ${tool.remaining} 次调用` : "本月暂无工具调用配额信息"}</p>
        </article>
      </div>

      <div className="quota-card usage-panel">
        <UsageHead level={usage?.level} />
        <UsageBody usage={usage} error={error} />
      </div>

      <section className="recent-card">
        <h2>关于当前用量</h2>
        <p>
          用量来自当前账号的 Coding Plan，每分钟自动更新，读取用量不会消耗任何额度。额度达到 80% 时圆环转为琥珀色提醒；每个窗口的恢复时间以圆环下方的说明为准。重置成功后，对应窗口的已使用占比会自动刷新。
        </p>
      </section>
    </div>
  );
}
