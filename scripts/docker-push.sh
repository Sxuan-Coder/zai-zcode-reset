#!/usr/bin/env bash
# 本地构建多架构镜像（linux/amd64 + linux/arm64）并推送到 Docker Hub。
#
# 前置条件：
#   1. 已 docker login（或至少已配置好登录凭据）
#   2. Docker 带 buildx（Docker Desktop / docker-ce 均自带）
#
# 用法：
#   DOCKER_USER=<你的DockerHub用户名> ./scripts/docker-push.sh            # 版本号默认为当天日期
#   DOCKER_USER=<你的DockerHub用户名> ./scripts/docker-push.sh v0.1.0     # 指定版本号
set -euo pipefail
cd "$(dirname "$0")/.."

DOCKER_USER="${DOCKER_USER:-}"
if [[ -z "$DOCKER_USER" ]]; then
  echo "错误：请先指定 Docker Hub 用户名，例如：DOCKER_USER=yourname $0 v0.1.0" >&2
  exit 1
fi
IMAGE="${DOCKER_USER}/zai-zcode-reset"
VERSION="${1:-$(date +%Y%m%d)}"

# 复用（或创建）一个支持多架构的构建器
if ! docker buildx ls | grep -q '^zsr-builder'; then
  docker buildx create --name zsr-builder --driver docker-container --use >/dev/null
fi
docker buildx use zsr-builder >/dev/null

echo "==> 构建 ${IMAGE}:{latest,${VERSION}}（linux/amd64, linux/arm64）并推送……"
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t "${IMAGE}:latest" \
  -t "${IMAGE}:${VERSION}" \
  --push \
  .

echo "==> 完成。其他人即可使用："
echo "      docker pull ${IMAGE}:${VERSION}"
echo "    或在 docker-compose.yml 中将镜像名替换为 ${IMAGE}:latest"
