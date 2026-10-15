import { useEffect, useState } from "react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { Icons } from "../ui";

/* 页面文案与原型一致：标题 / 描述由布局统一渲染，页面只负责主体内容 */
const pages = [
  { path: "/app", label: "工作概览", crumb: "概览", icon: Icons.grid, title: "每一份灵感，都值得继续。", description: "管理你的使用额度，让专注不被打断。" },
  { path: "/app/records", label: "重置记录", crumb: "重置记录", icon: Icons.history, title: "每一次继续，都有迹可循。", description: "查看你的额度重置历史与操作结果。" },
  { path: "/app/usage", label: "用量分析", crumb: "用量分析", icon: Icons.chart, title: "掌握用量，安排你的节奏。", description: "了解当前使用情况，为下一次创造做好准备。" },
  { path: "/app/settings", label: "偏好设置", crumb: "偏好设置", icon: Icons.settings, title: "把工作空间，调成你的节奏。", description: "查看账户信息，管理你的登录会话。" },
];

export default function MemberLayout() {
  const { me, logout } = useAuth();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const [open, setOpen] = useState(false);
  const page = pages.find((p) => p.path === pathname) ?? pages[0];
  const initial = (me?.user.username ?? "?").charAt(0).toUpperCase();
  const today = new Date().toLocaleDateString("zh-CN", { year: "numeric", month: "long", day: "numeric", weekday: "long" });

  useEffect(() => setOpen(false), [pathname]);
  useEffect(() => {
    if (!open) return;
    const close = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [open]);

  return (
    <div className="member-app">
      {open ? <button className="sidebar-scrim" aria-label="关闭导航" onClick={() => setOpen(false)} /> : null}
      <aside id="member-sidebar" className={`member-sidebar${open ? " open" : ""}`}>
        <NavLink to="/app" className="member-brand" aria-label="ZCode AI 工作台">
          <span className="member-logo">Z</span>
          <strong>ZCode</strong>
          <sup>AI</sup>
        </NavLink>
        <div className="workspace-label">WORKSPACE</div>
        <nav aria-label="工作空间导航">
          {pages.map((p) => (
            <NavLink key={p.path} to={p.path} end={p.path === "/app"} className={({ isActive }) => `member-nav${isActive ? " active" : ""}`}>
              <p.icon />
              <span>{p.label}</span>
            </NavLink>
          ))}
          {me?.user.role === "admin" ? (
            <NavLink to="/admin" className="member-nav">
              <Icons.shield />
              <span>管理后台</span>
            </NavLink>
          ) : null}
        </nav>
        <div className="member-sidebar-bottom">
          <div className="member-plan">
            <strong>✧ &nbsp;Coding Plan</strong>
            <p>
              让灵感持续发生，
              <br />
              让每一次编码都更从容。
            </p>
            <NavLink to="/app/usage">
              查看当前用量 <span>→</span>
            </NavLink>
          </div>
          <div className="member-user">
            <span className="member-avatar">{initial}</span>
            <div>
              <strong>{me?.user.username}</strong>
              <small>个人工作空间</small>
            </div>
            <button
              className="member-icon"
              aria-label="退出登录"
              title="退出登录"
              onClick={async () => {
                await logout();
                navigate("/login");
              }}
            >
              <Icons.logout />
            </button>
          </div>
        </div>
      </aside>

      <main className="member-main">
        <header className="member-topbar">
          <div className="member-crumb">
            <button
              className="member-icon mobile-menu"
              aria-label={open ? "关闭导航" : "打开导航"}
              aria-expanded={open}
              aria-controls="member-sidebar"
              onClick={() => setOpen(!open)}
            >
              <Icons.menu />
            </button>
            <span>工作空间</span>
            <span>/</span>
            <strong>{page.crumb}</strong>
          </div>
          <NavLink to="/app/settings" className="member-avatar" aria-label="账户设置">
            {initial}
          </NavLink>
        </header>

        <div className="member-page-header">
          <div>
            <div className="member-eyebrow">YOUR CODING, UNINTERRUPTED.</div>
            <h1>{page.title}</h1>
            <p>{page.description}</p>
          </div>
          <div className="member-date">
            <Icons.calendar />
            {today}
          </div>
        </div>

        <Outlet />

        <footer className="member-footer">
          <span>© {new Date().getFullYear()} ZCode AI　·　为每一个创造者，留出更多空间。</span>
          <span>
            <Icons.shield /> 让专注持续发生
          </span>
        </footer>
      </main>
    </div>
  );
}
