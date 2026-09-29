import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../../auth";
import { Icons } from "../../ui";

export default function AdminLayout() {
  const { me, logout } = useAuth();
  const navigate = useNavigate();
  const initial = (me?.user.username ?? "?").charAt(0).toUpperCase();

  const item = (to: string, label: string, icon: React.ReactNode) => (
    <NavLink to={to} end={to === "/admin"} className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}>
      {icon} <span className="nav-label">{label}</span>
    </NavLink>
  );

  return (
    <div>
      <header className="topbar">
        <div className="brand" onClick={() => navigate("/admin")}>
          <div className="logo-mark">Z</div>
          <div className="brand-name">
            ZCode AI <span className="brand-badge" style={{ background: "#6b7280" }}>管理后台</span>
          </div>
        </div>
        <div className="topbar-right">
          <button className="btn btn-ghost btn-sm" onClick={() => navigate("/app")}>
            返回前台
          </button>
          <div className="user-chip">
            <div className="avatar">{initial}</div>
            <span className="username">{me?.user.username}</span>
          </div>
          <button
            className="btn btn-ghost btn-sm"
            onClick={async () => {
              await logout();
              navigate("/login");
            }}
          >
            <Icons.logout /> 退出
          </button>
        </div>
      </header>

      <div className="layout">
        <nav className="sidebar">
          {item("/admin", "总览", <Icons.gauge />)}
          {item("/admin/users", "用户管理", <Icons.users />)}
          {item("/admin/sessions", "会话管理", <Icons.activity />)}
          {item("/admin/rules", "重置规则", <Icons.list />)}
          {item("/admin/accounts", "上游账号", <Icons.key />)}
          {item("/admin/logs", "操作日志", <Icons.shield />)}
        </nav>
        <main className="content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
