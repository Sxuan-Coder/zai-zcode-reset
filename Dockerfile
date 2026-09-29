# zai-zcode-reset 多阶段构建
#   阶段 1：Node 构建前端静态资源（Vite）
#   阶段 2：Go 交叉编译纯静态后端二进制（CGO 关闭，仅标准库）
#   阶段 3：scratch 空镜像运行——不含 libc，兼容 CentOS 7（内核 3.10）等老环境
# 构建上下文为仓库根目录：docker build -t zai-zcode-reset .

# ---------- 阶段 1：前端 ----------
FROM node:20-alpine AS frontend-builder
RUN corepack enable pnpm && corepack prepare pnpm@9 --activate
WORKDIR /build
# 先只拷贝依赖清单，充分利用层缓存
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build   # 产物输出到 /build/dist

# ---------- 阶段 2：后端 ----------
FROM golang:1.24-alpine AS server-builder
# ca-certificates：供运行时调用上游 HTTPS API；tzdata：日志本地时间
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /src
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/zsr .

# ---------- 阶段 3：运行时（scratch，约 30MB） ----------
FROM scratch
LABEL org.opencontainers.image.title="zai-zcode-reset" \
      org.opencontainers.image.description="ZAI Coding Plan 额度重置多租户平台（前端 + 后端单容器）" \
      org.opencontainers.image.source="https://github.com/Sxuan-Coder/zai-zcode-reset" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=server-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=server-builder /usr/share/zoneinfo /usr/share/zoneinfo
ENV TZ=Asia/Shanghai \
    LISTEN_ADDR=0.0.0.0:8787 \
    DATA_DIR=/app/data \
    FRONTEND_DIST=/app/frontend

COPY --from=server-builder /out/zsr /app/zsr
COPY --from=frontend-builder /build/dist /app/frontend

EXPOSE 8787
ENTRYPOINT ["/app/zsr"]
