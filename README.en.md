# zai-zcode-reset — ZAI Coding Plan Quota Reset, Multi-Tenant Platform

[简体中文](./README.md) | English

A shared platform for Coding Plan quota resets: turns "quota resets are only possible on the official website or inside the ZCode client" into a general-purpose multi-tenant system — public landing page + controlled member accounts + admin console + configurable quota rules. **Upstream tokens live only on the server** (encrypted at rest with AES-256-GCM); the browser never touches them.

See [DESIGN.md](./DESIGN.md) for the general solution design.

## Screenshot

The reset panel members see after signing in: 5-hour / weekly reset opportunities (available counts, expiry, one-click use), plus real Coding Plan usage rings (5-hour window, weekly window, monthly tool calls).

![Reset panel: 5-hour/weekly reset opportunities and Coding Plan usage rings](docs/images/dashboard.png)

## Features

- **Landing page**: public, no login required
- **Member system**: admins create accounts manually (password auto-generated, shown once); single sign-on (a new login revokes the old session); login IP/UA fully recorded; instant forced logout at any time
- **Reset panel**: shows 5-hour-limit / weekly-limit reset opportunities — if available, shows the count and expiry and a "Use" button; otherwise shows "No reset". Clicking triggers the server-orchestrated two-step flow: claim (opportunity) → consume (use)
- **Coding Plan usage panel**: real upstream usage for the bound account — 5-hour window and weekly window shown as percentage rings (red at ≥80%), plus plan level and monthly tool-call usage
- **Quota rules**: global defaults + per-user overrides (daily/weekly, split by reset type), nearest-match specificity, reserve-and-rollback, concurrency-safe; upstream 3301/429 cooldown boundaries are passed through automatically
- **Admin console**: overview / user management / session management / reset rules / upstream accounts (token masking + connection test) / operation logs (login · reset · audit)
- **Audit**: every login attempt, reset execution, and admin operation is persisted

## Tech Stack

| Layer | Technology |
| --- | --- |
| Backend | Go (standard library only, zero third-party deps, single binary) |
| Frontend | React 18 + TypeScript + Vite + react-router (build output served directly by Go, single-port deployment) |
| Storage | `data/db.json` (atomic writes) + JSONL append-only logs; Store is interface-based, swappable to SQLite |

## Quick Start

```bash
# 1. Build the frontend
cd frontend
pnpm install
pnpm build            # output in frontend/dist

# 2. Build and start the backend (first start bootstraps the admin and seeds default rules)
cd ../backend
go build -o zsr .
ADMIN_PASSWORD=your-admin-password ./zsr

# 3. Visit http://127.0.0.1:8787
```

On first start the log prints the admin username and initial password (a random password is generated — and shown once — if `ADMIN_PASSWORD` is not set). Default seeded global rules: 1 per day / 2 per week.

After logging in as admin: enter the ZCode JWT and Coding Plan Token on the **Upstream Accounts** page → (optionally) tune limits under **Reset Rules** → create member accounts under **User Management** and hand them out.

## Installation (Docker Compose, Recommended)

The image bundles frontend + backend in a single container on a single port (8787), multi-arch (`linux/amd64` and `linux/arm64`); the runtime layer is a scratch image (pure static Go binary, no libc), so it runs directly on old environments such as CentOS 7 (kernel 3.10).

### One-Command Setup

```bash
# ① Install Docker (CentOS 7: docker-ce ≥ 20.10 with the compose plugin recommended)
#    https://docs.docker.com/engine/install/centos/

# ② Bring the service up
mkdir -p /opt/zsr && cd /opt/zsr
wget -O docker-compose.yml https://raw.githubusercontent.com/Sxuan-Coder/zai-zcode-reset/main/docker-compose.yml
docker compose up -d            # older versions: docker-compose up -d

# ③ Grab the admin password generated on first start (printed once — save it now)
docker compose logs
```

Configuration is passed via `environment` in `docker-compose.yml` (see comments in the file); all data lives in `./data/` (db.json, secret.key, audit logs) — **back that directory up regularly**. To upgrade: `docker compose pull && docker compose up -d`.

## Frontend Dev Mode

```bash
cd frontend && pnpm dev   # Vite on port 5173, /api proxied to 127.0.0.1:8787
```

## Configuration

Environment variables with same-name flags (flags take precedence):

| Variable | Default | Description |
| --- | --- | --- |
| `LISTEN_ADDR` / `-addr` | `127.0.0.1:8787` | Listen address |
| `DATA_DIR` / `-data` | `data` | Data directory (db.json, logs, keys) |
| `FRONTEND_DIST` / `-frontend` | `frontend/dist` | Frontend static assets; empty string disables |
| `MASTER_KEY` | auto-generated | Master key for token encryption (64 hex chars); defaults to `data/secret.key` |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | `admin` / random | Bootstrap admin on first start |
| `SINGLE_SESSION` | `true` | Single sign-on: a new login revokes the old session |
| `SESSION_TTL_HOURS` | `168` | Session lifetime in hours (sliding renewal) |
| `TRUST_PROXY` | `false` | Set true behind a reverse proxy so real IPs are read from X-Forwarded-For |
| `COOKIE_SECURE` | `false` | Set true for HTTPS deployments |
| `DEFAULT_DAY_LIMIT` / `DEFAULT_WEEK_LIMIT` | `1` / `2` | Global default quotas seeded on first start |
| `MOCK_UPSTREAM` | `false` | Dev only: upstream returns mock data — **never enable in production** |

## Deployment Notes

1. Put Nginx/Caddy in front for HTTPS and proxy to `127.0.0.1:8787`; set `TRUST_PROXY=true` and `COOKIE_SECURE=true`
2. Inject `MASTER_KEY` via environment variable and back it up; if lost, encrypted tokens become undecryptable (just re-enter them in the admin console)
3. Back up the `data/` directory regularly
4. Upstream tokens are long-lived but revocable: if you suspect a leak, re-login upstream to invalidate the old JWT and update the token in the admin console

## Upstream API Integration (Verified Semantics)

- Endpoint family: `/api/v1/coding-plan/reset/{status|opportunity|use|history/read}`
- Dual auth: `Authorization: <ZCode JWT>` + `X-Bigmodel-Authorization: <Plan Token>` (Team adds `Bigmodel-Target-Type/Organization/Project`)
- Envelope `{code,msg,data}`: **code=0 means success**; `3301` = opportunity claim denied (`data.next_try_at` is the cooldown boundary in ms); HTTP 429 = throttled
- All timestamps are in milliseconds
- Usage monitoring (read-only): `GET /api/monitor/usage/quota/limit` on `bigmodel.cn` / `api.z.ai`, `authorization` header passes the business key verbatim; `TOKENS_LIMIT` unit=3 is the 5-hour window, unit=6 the weekly window, `TIME_LIMIT` monthly tool calls

## License

Released under the [Apache License 2.0](./LICENSE). See [NOTICE](./NOTICE) for attribution details: forks and derivative works MUST retain the upstream repository address (https://github.com/Sxuan-Coder/zai-zcode-reset) and the original copyright notice in their README, NOTICE, or equivalent documentation, as required by Section 4 of the Apache License 2.0.

## Disclaimer

This project is for learning and controlled internal sharing. Sharing account quotas may violate upstream terms of service; abuse may get the account restricted — use at your own risk.
