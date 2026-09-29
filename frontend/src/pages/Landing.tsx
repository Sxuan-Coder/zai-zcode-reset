import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth";
import { Icons } from "../ui";

export default function Landing() {
  const navigate = useNavigate();
  const { me } = useAuth();

  return (
    <div className="landing">
      <header className="topbar">
        <div className="brand">
          <div className="logo-mark">Z</div>
          <div className="brand-name">
            ZCode <span className="brand-badge">AI</span>
          </div>
        </div>
        <button className="btn btn-ghost btn-sm" onClick={() => navigate(me ? "/app" : "/login")}>
          {me ? "进入控制台" : "登录"}
        </button>
      </header>

      <main className="landing-main">
        <div className="landing-hero-badge">✦ Coding Plan 额度重置共享平台</div>
        <h1>
          安全共享你的
          <br />
          <span className="grad">Coding Plan 重置额度</span>
        </h1>
        <p className="landing-sub">
          上游令牌只存于服务端并全程加密。成员在受控配额内一键完成 5 小时限制与周限制重置，管理员可随时审计与强制下线。
        </p>
        <div className="landing-cta">
          <button className="btn btn-primary" onClick={() => navigate(me ? "/app" : "/login")}>
            {me ? "进入控制台" : "立即登录"}
          </button>
          <button className="btn btn-ghost" onClick={() => navigate("/login")}>
            了解配额规则
          </button>
          <a
            className="btn btn-ghost github-star-btn"
            href="https://github.com/Sxuan-Coder/zai-zcode-reset"
            target="_blank"
            rel="noopener noreferrer"
          >
            <Icons.github />
            <span>去 GitHub 点个 Star ⭐</span>
          </a>
        </div>

        <div className="feature-grid">
          <div className="feature-card">
            <div className="feature-icon">
              <Icons.shield />
            </div>
            <div className="feature-title">令牌服务端托管</div>
            <div className="feature-text">
              上游双重凭据以 AES-256-GCM 加密存储，仅在调用上游接口时解密使用。任何页面与接口都不回显明文，浏览器永不接触令牌。
            </div>
          </div>
          <div className="feature-card">
            <div className="feature-icon">
              <Icons.users />
            </div>
            <div className="feature-title">成员权限管理</div>
            <div className="feature-text">
              管理员手动创建成员账号，单点登录、登录 IP 全程记录，可随时强制下线；停用账号即刻失去全部访问能力。
            </div>
          </div>
          <div className="feature-card">
            <div className="feature-icon">
              <Icons.gauge />
            </div>
            <div className="feature-title">灵活配额规则</div>
            <div className="feature-text">
              按用户或全局配置每日 / 每周重置次数上限，规则按 specificity 就近生效；上游冷却边界自动透传，避免触发限流。
            </div>
          </div>
        </div>
      </main>

      <footer className="landing-footer">
        <a className="footer-github" href="https://github.com/Sxuan-Coder/zai-zcode-reset" target="_blank" rel="noopener noreferrer">
          <Icons.github />
          <span>Sxuan-Coder/zai-zcode-reset</span>
        </a>
        <span> · 觉得好用就去仓库点个 Star 支持一下 · 仅供学习与内部使用 · 请遵守上游服务条款</span>
      </footer>
    </div>
  );
}
