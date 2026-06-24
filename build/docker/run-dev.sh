#!/usr/bin/env bash
set -euo pipefail

# Get the directory of the script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Repository root is two levels up
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

echo "🧹 Stopping and removing old hippodrop-dev container if exists..."
docker stop hippodrop-dev 2>/dev/null || true
docker rm hippodrop-dev 2>/dev/null || true

echo "🔨 Building hippodrop-dev-image..."
docker build -t hippodrop-dev-image -f "${SCRIPT_DIR}/Dockerfile.dev" "${REPO_ROOT}"

echo "🚀 Starting new hippodrop-dev container with workspace mount..."
docker run -d \
  --name hippodrop-dev \
  -v "${REPO_ROOT}":/app \
  -w /app \
  hippodrop-dev-image

echo "✨ hippodrop-dev container is running!"
echo "   Go Version: $(docker exec hippodrop-dev go version)"
echo "   Workspace mounted at: /app"
