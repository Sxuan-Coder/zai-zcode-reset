import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { fmtDate } from "../api";

export default function Settings() {
  const { me, logout } = useAuth();
  const navigate = useNavigate();

  return (
    <div className="page-card" style={{ maxWidth: 560 }}>
      <div className="admin-head">
        <div className="admin-title">设置</div>
      </div>
      <dl className="kv-list">
        <dt>用户名</dt>
        <dd>{me?.user.username}</dd>
        <dt>角色</dt>
        <dd>{me?.user.role === "admin" ? "管理员" : "成员"}</dd>
        <dt>登录 IP</dt>
        <dd className="mono">{me?.session.ip ?? "—"}</dd>
        <dt>登录时间</dt>
        <dd className="mono">{fmtDate(me?.session.login_at)}</dd>
        <dt>会话过期</dt>
        <dd className="mono">{fmtDate(me?.session.expires_at)}</dd>
        <dt>User-Agent</dt>
        <dd className="mono text-sm" style={{ wordBreak: "break-all" }}>
          {me?.session.user_agent ?? "—"}
        </dd>
      </dl>
      <div style={{ marginTop: 24, display: "flex", gap: 12 }}>
        <button
          className="btn btn-primary"
          onClick={async () => {
            await logout();
            navigate("/login");
          }}
        >
          退出登录
        </button>
      </div>
      <p className="form-hint" style={{ marginTop: 16 }}>
        单点登录已开启：在其他设备登录会使当前会话立即失效。如需修改密码请联系管理员。
      </p>
    </div>
  );
}
