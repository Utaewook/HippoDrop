#!/usr/bin/env bash
set -euo pipefail

# Get the directory of the script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Repository root is two levels up
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

echo "🧹 Stopping and removing old tardis-dev container if exists..."
docker stop tardis-dev 2>/dev/null || true
docker rm tardis-dev 2>/dev/null || true

echo "🔨 Building tardis-dev-image..."
docker build -t tardis-dev-image -f "${SCRIPT_DIR}/Dockerfile.dev" "${REPO_ROOT}"

echo "🚀 Starting new tardis-dev container with workspace mount..."
docker run -d \
  --name tardis-dev \
  -v "${REPO_ROOT}":/app \
  -w /app \
  tardis-dev-image

echo "✨ tardis-dev container is running!"
echo "   Go Version: $(docker exec tardis-dev go version)"
echo "   Workspace mounted at: /app"
