import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { fmtDate } from "../api";

function Row({ label, hint, value, mono = false, wrap = false }: { label: string; hint: string; value: string; mono?: boolean; wrap?: boolean }) {
  return (
    <div className="setting-row">
      <div>
        <h3>{label}</h3>
        <p>{hint}</p>
      </div>
      <span className={`setting-value${mono ? " mono" : ""}${wrap ? " wrap" : ""}`}>{value}</span>
    </div>
  );
}

export default function Settings() {
  const { me, logout } = useAuth();
  const navigate = useNavigate();
  const session = me?.session;

  return (
    <div style={{ maxWidth: 720 }}>
      <section className="recent-card member-settings">
        <Row label="用户名" hint="管理员分配的账号，用于登录本工作空间。" value={me?.user.username ?? "—"} />
        <Row label="角色" hint="管理员可进入管理后台，成员仅使用额度重置。" value={me?.user.role === "admin" ? "管理员" : "成员"} />
        <Row label="登录 IP" hint="本次会话的来源地址。" value={session?.ip ?? "—"} mono />
        <Row label="登录时间" hint="本次会话建立的时间。" value={fmtDate(session?.login_at)} mono />
        <Row label="会话过期" hint="到期后需要重新登录。" value={fmtDate(session?.expires_at)} mono />
        <Row label="User-Agent" hint="本次会话的浏览器与设备标识。" value={session?.user_agent || "—"} mono wrap />
        <div className="setting-row">
          <div>
            <h3>退出登录</h3>
            <p>结束当前会话，回到登录页。其他设备上的会话不受影响。</p>
          </div>
          <button
            className="btn btn-ghost btn-sm"
            onClick={async () => {
              await logout();
              navigate("/login");
            }}
          >
            退出登录
          </button>
        </div>
      </section>
      <p className="member-note">单点登录已开启：在其他设备登录会使当前会话立即失效。如需修改密码请联系管理员。</p>
    </div>
  );
}
