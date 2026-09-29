# zai-zcode-reset —— Coding Plan 额度重置共享平台 · 通用方案设计

> 目标：把"重置Coding Plan 额度"这个个人需求，泛化为一套**通用的、可运营的额度重置共享平台**：公开落地页 + 受控成员体系 + 管理后台 + 可配置的重置配额规则，上游凭据全程只存在于服务端。

---

## 1. 背景与问题域

上游（BigModel / ZAI Coding Plan）提供额度重置能力：

| 上游端点 | 方法 | 作用 |
| --- | --- | --- |
| `/api/v1/coding-plan/reset/status` | GET | 查询可用的 5 小时 / 周重置机会与历史 |
| `/api/v1/coding-plan/reset/opportunity` | POST | 申领一次重置机会（body: `idempotency_key`） |
| `/api/v1/coding-plan/reset/use` | POST | 消耗一次重置机会（body: `idempotency_key`, `reset_type: FIVE_HOUR \| WEEK`） |
| `/api/v1/coding-plan/reset/history/read` | POST | 标记历史已读 |

关键上游语义：

- 鉴权需要**双重令牌**：`Authorization: <zcode JWT>` + `X-Bigmodel-Authorization: <Coding Plan token>`；Team 还要带 `Bigmodel-Target-Type: TEAM` + `Bigmodel-Organization` / `Bigmodel-Project`。
- 统一信封 `{code, msg, data}`，**成功码是 `0`**；`3301` = 申领被拒（data 含 `next_try_at` 毫秒时间戳，是必须遵守的服务端冷却边界）；HTTP 429 = 限流。
- `expire_at` / `used_at` 均为**毫秒时间戳**。
- 令牌本身不带 `exp`，长期有效、服务端可吊销；泄露的补救手段是重新登录上游使旧 JWT 失效。
- 重置的完整链路是两段式：**先申领机会（opportunity），再消耗使用（use）**；status 的 available 列表里只有已申领且未过期、未消耗的机会。

因此本平台的核心职责是：**替成员安全地保管令牌、按规则限流、编排两段式重置链路，并全程审计**。

## 2. 总体架构

```
                    ┌────────────────────────────────────────────────┐
   访客             │              落地页 /  (公开，无需登录)            │
                    └────────────────────────────────────────────────┘
   成员             │   登录 → 重置面板（5 小时重置 / 周重置 卡片）        │
                    │        · 显示可用机会与过期时间，无则显示「没重置」  │
                    │        · 点击「使用重置机会」触发服务端编排          │
                    ├────────────────────────────────────────────────┤
   管理员           │   管理后台：用户 / 会话(强制下线) / 重置规则 /       │
                    │             上游账号(令牌托管) / 审计日志           │
                    └───────────────┬────────────────────────────────┘
                                    │  HTTPS（同源，HttpOnly Cookie 会话）
                    ┌───────────────▼────────────────────────────────┐
                    │                Go 后端（单进程）                  │
                    │  ┌──────────┐ ┌──────────┐ ┌──────────────────┐ │
                    │  │ 会话/认证  │ │ 配额规则  │ │  重置编排器        │ │
                    │  └──────────┘ └──────────┘ └──────────────────┘ │
                    │  ┌──────────┐ ┌──────────┐ ┌──────────────────┐ │
                    │  │ 登录限流  │ │ 审计日志  │ │  令牌保险箱(AES)   │ │
                    │  └──────────┘ └──────────┘ └──────────────────┘ │                    │        持久化：data/db.json + *.jsonl           │
                    └───────────────┬────────────────────────────────┘
                                    │ 双重令牌仅在此处出现
                    ┌───────────────▼────────────────┐
                    │  上游 BigModel / ZAI reset API   │
                    └────────────────────────────────┘
```

安全不变量：

1. **上游令牌只存在于服务端**（磁盘上 AES-256-GCM 加密，内存中只在调用上游时解密），任何 API 响应只回显掩码。
2. 前端与成员之间只传递 HttpOnly 会话 Cookie，令牌永不出现在浏览器。
3. 一切重置动作先过「会话有效 → 用户启用 → 配额规则 → 计数预占」四道闸门，再触上游。

## 3. 模块设计

### 3.1 落地页（公开）

- 纯静态内容：产品定位、特性（令牌服务端托管 / 成员权限 / 灵活配额）、登录入口。
- 不做任何数据请求；未登录访问 `/app`、`/admin` 会被前端路由守卫与后端 401 双重拦截。

### 3.2 用户与登录管理

**实体**

- `User`：`id, username, password_hash(PBKDF2-SHA256), role(admin|member), status(active|disabled), created_at, last_login_at`
- `Session`：`id, user_id, token_hash(SHA-256), ip, user_agent, created_at, last_seen_at, expires_at, revoked_at?`
- `LoginLog`（append-only）：`at, username, user_id?, ip, user_agent, success, fail_reason?`

**流程**

- 管理员在后台**手动新增用户**：给定用户名，密码可指定或由系统生成（仅创建响应中明文出现一次）。
- 登录：校验密码 → 创建会话 → 下发 HttpOnly Cookie → 记录登录 IP/UA 到会话与登录日志。
- **单点登录**（单会话模式，默认开启）：同一用户新登录时自动吊销其旧会话，保证同一账号同一时刻只有一处在线；关闭后允许多端并存。
- **强制下线**：管理员可吊销单个会话，或一键吊销某用户全部会话；被吊销会话的下一次请求立即 401。
- 登录防爆破：按 `用户名+IP` 维度内存限流（默认 5 次失败锁 15 分钟）。
- 会话滑动过期：默认 7 天不活动失效；每次请求刷新 `last_seen_at`。

### 3.3 权限校验与重置配额

**规则模型（管理员可配，通用化设计）**

```
Rule: { id, user_id?: (空=全局), reset_type: FIVE_HOUR|WEEK|ALL,
        period: DAY|WEEK, max_count, enabled }
```

- 匹配优先级（越具体越强）：`用户+类型 > 用户+ALL > 全局+类型 > 全局+ALL`。
- 计数键：`user_id|reset_type|period|period_key`，`period_key` 按服务器本地时间取 `YYYY-MM-DD`（DAY）或 ISO 周 `YYYY-Www`（WEEK）。
- 首次启动自动播种全局默认规则（`DAY=1`、`WEEK=2`，可用环境变量覆盖），管理员可随时改。
- **只统计上游成功的消耗**；执行采用「预占计数 → 上游成功提交，失败回滚」，杜绝并发点击穿透限额。

**重置编排（单次点击的完整服务端链路）**

```
点击「使用重置机会」
  → 网关：会话 / 用户状态 / 冷却 / 配额预占
  → ① GET status：该类型已有可用机会？ → 跳到 ③
  → ② POST opportunity（幂等键=UUID）：
        code=0   → granted，继续
        3301/429 → 写入冷却（透传 next_try_at / 退避），失败收尾
  → ③ POST use（幂等键=UUID, reset_type）
  → ④ 刷新 status 回传前端 + 记录重置日志（成功/失败均记，失败回滚计数）
```

冷却（cooldown）是平台级缓存：上游 3301 的 `next_try_at` 会被记住，冷却期内直接拒绝，避免持续撞上游限流。

### 3.4 上游账号与令牌托管（通用化的关键）

```
UpstreamAccount: { id, name, family: bigmodel|zai, base_url?, target_type: PERSONAL|TEAM,
                   org_id?, project_id?, zcode_jwt(enc), plan_token(enc),
                   enabled, is_default }
```

- 令牌以 `enc:v1:` + AES-256-GCM 落盘；主密钥来自 `MASTER_KEY` 环境变量，缺省时自动生成并保存到 `data/secret.key`（仅服务器可读）。
- 多账号是**方案内置能力**：v1 版全部成员使用默认账号；数据模型已含 `users.upstream_account_id` 预留位，后续可平滑演进为「每个成员绑定不同上游账号」的多租户形态。
- 管理后台提供「连接测试」按钮（即调用一次 status 验证令牌有效性）。

### 3.5 审计日志（append-only JSONL）

- `login.jsonl`：每次登录尝试（含失败与原因）。
- `reset.jsonl`：每次重置执行（用户、类型、幂等键、结果、上游错误码、IP）。
- `audit.jsonl`：所有管理操作（建用户、改规则、吊销会话、改账号……）。

## 4. 技术选型

| 层 | 选型 | 理由 |
| --- | --- | --- |
| 后端 | Go 1.24+，**仅标准库**（net/http 路由、crypto/pbkdf2、crypto/aes） | 单二进制部署、零第三方依赖、离线可构建；Windows/Linux 通用 |
| 存储 | `data/db.json`（RWMutex + 原子写）+ JSONL 追加日志 | 用户量级小（数十人），无引入 SQLite 的必要；Store 为接口化设计，可平滑换 SQLite |
| 前端 | Vite + React 18 + TypeScript + react-router 6 | 与既有技术栈一致；构建产物为静态文件，由 Go 直接托管（同源部署，天然免 CSRF 跨域问题） |
| 部署 | `go build` 单二进制 + `frontend/dist` 静态目录，单端口 | 可直接跑在本机/内网/VPS，建议前置 HTTPS 反代 |

## 5. API 契约（摘要）

统一错误形：`{"error":{"code":"...","message":"..."}}`

| 方法 & 路径 | 权限 | 说明 |
| --- | --- | --- |
| `POST /api/auth/login` | 公开 | 登录，Set-Cookie；限流 |
| `POST /api/auth/logout` | 会话 | 登出并吊销会话 |
| `GET /api/auth/me` | 会话 | 当前用户 + 会话信息（IP、登录时间） |
| `GET /api/reset/status` | 会话 | 上游机会快照 + 本人配额用量（含冷却） |
| `POST /api/reset/execute` | 会话 | 编排一次重置 `{reset_type}` |
| `GET/POST/PATCH/DELETE /api/admin/users[/{id}]` | 管理员 | 用户管理 |
| `POST /api/admin/users/{id}/revoke-sessions` | 管理员 | 强制下线该用户全部会话 |
| `GET/DELETE /api/admin/sessions[/{id}]` | 管理员 | 会话管理（含 IP） |
| `GET/POST/PATCH/DELETE /api/admin/rules[/{id}]` | 管理员 | 配额规则管理 |
| `GET/POST/PATCH/DELETE /api/admin/accounts[/{id}]`、`POST /{id}/test` | 管理员 | 上游账号管理（令牌仅写入，永不回显） |
| `GET /api/admin/logs/{login|reset|audit}` | 管理员 | 审计日志查询 |

## 6. 安全设计清单

- 上游令牌：仅服务端持有 + AES-256-GCM 静态加密 + API 掩码回显 + 日志脱敏。
- 密码：PBKDF2-SHA256（600k 迭代 + 16B 盐 + 常量时间比较）。
- 会话：256-bit 随机 token，库存 SHA-256；HttpOnly + SameSite=Lax（+ 可选 Secure）；滑动过期。
- 授权：后端中间件逐请求校验（会话 → 用户状态），前端路由守卫仅为体验层。
- 配额：预占/回滚模型，并发安全；上游冷却透传，避免放大限流。
- 登录防爆破、管理操作全量审计。

## 7. 扩展路线（不在 v1 范围）

1. **用量统计模块**：上游已备有 `model-usage` / `tool-usage` / `credit-usage/*` / `quota/limit` 等只读端点（`authorization` 头直接传业务 Key、不加 Bearer），可直接按同样模式接入为「用量看板」。
2. 存储 SQLite 化与多实例部署。
3. 成员自助改密、TOTP 二次验证、注册审批流。
4. 多上游账号绑定（数据模型已预留）与按账号分组配额。
