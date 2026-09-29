import { Icons } from "../ui";

export default function Usage() {
  return (
    <div className="page-card">
      <div className="admin-head">
        <div className="admin-title">用量统计</div>
      </div>
      <div className="explain-card">
        <div className="explain-icon">
          <Icons.activity />
        </div>
        <div className="explain-body">
          <div className="explain-title">即将上线</div>
          <div className="explain-text">
            用量看板将接入上游的模型用量、工具用量与额度曲线接口，展示每个计费窗口的消耗明细。当前版本聚焦额度重置链路，敬请期待。
          </div>
        </div>
      </div>
    </div>
  );
}
