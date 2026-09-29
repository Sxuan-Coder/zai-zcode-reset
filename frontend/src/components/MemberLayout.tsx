import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { Icons } from "../ui";

export default function MemberLayout() {
  const { me, logout } = useAuth();
  const navigate = useNavigate();
  const initial = (me?.user.username ?? "?").charAt(0).toUpperCase();

  return (
    <div>
      <header className="topbar">
        <div className="brand" onClick={() => navigate("/app")}>
          <div className="logo-mark">Z</div>
          <div className="brand-name">
            ZCode <span className="brand-badge">AI</span>
          </div>
        </div>
        <div className="topbar-right">
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
            title="退出登录"
          >
            <Icons.logout /> 退出
          </button>
        </div>
      </header>

      <div className="layout">
        <nav className="sidebar">
          <NavLink to="/app" end className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}>
            <Icons.gauge /> <span className="nav-label">概览</span>
          </NavLink>
          <NavLink to="/app/records" className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}>
            <Icons.list /> <span className="nav-label">记录</span>
          </NavLink>
          <NavLink to="/app/usage" className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}>
            <Icons.activity /> <span className="nav-label">用量</span>
          </NavLink>
          <NavLink to="/app/settings" className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}>
            <Icons.key /> <span className="nav-label">设置</span>
          </NavLink>
          {me?.user.role === "admin" && (
            <>
              <div className="nav-split" />
              <NavLink to="/admin" className="nav-item">
                <Icons.shield /> <span className="nav-label">管理后台</span>
              </NavLink>
            </>
          )}
        </nav>
        <main className="content">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
