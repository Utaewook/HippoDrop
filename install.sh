#!/bin/sh
set -e

# Configuration
REPO="Utaewook/Tardis"
BIN_DIR="/usr/local/bin"
CONFIG_DIR="/etc/tardis"
BINARY_NAME="tardis"

# Determine if sudo is needed and available
SUDO=""
if [ "$(id -u)" -ne 0 ]; then
    if command -v sudo >/dev/null 2>&1; then
        SUDO="sudo"
    else
        echo "Warning: You are not running as root and 'sudo' is not installed. Installation may fail due to permissions."
    fi
fi

# Detect OS and Architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# GitHub Releases URL construction
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${BINARY_NAME}-${OS}-${ARCH}"

echo "Downloading Tardis for ${OS}/${ARCH}..."

# Download binary to a temporary location
TMP_FILE="/tmp/${BINARY_NAME}"
if command -v curl >/dev/null 2>&1; then
    curl -sSL -f -o "$TMP_FILE" "$DOWNLOAD_URL"
elif command -v wget >/dev/null 2>&1; then
    wget -qO "$TMP_FILE" "$DOWNLOAD_URL"
else
    echo "Error: curl or wget is required to download Tardis."
    exit 1
fi

chmod +x "$TMP_FILE"

# Move to bin directory (requires sudo if not root)
echo "Installing to ${BIN_DIR}..."
$SUDO mv "$TMP_FILE" "${BIN_DIR}/${BINARY_NAME}"

# Setup config directory
echo "Setting up config directory at ${CONFIG_DIR}..."
$SUDO mkdir -p "${CONFIG_DIR}"

if [ ! -f "${CONFIG_DIR}/tardis.yml" ]; then
    # Download example config directly from the main branch
    CONFIG_URL="https://raw.githubusercontent.com/${REPO}/main/tardis.example.yml"
    if command -v curl >/dev/null 2>&1; then
        $SUDO curl -sSL -o "${CONFIG_DIR}/tardis.yml" "$CONFIG_URL"
    else
        $SUDO wget -qO "${CONFIG_DIR}/tardis.yml" "$CONFIG_URL"
    fi
    echo "Default config installed at ${CONFIG_DIR}/tardis.yml"
else
    echo "Config already exists at ${CONFIG_DIR}/tardis.yml, skipping."
fi

echo "=========================================="
echo "Tardis installed successfully!"
echo "Run it with: tardis -c ${CONFIG_DIR}/tardis.yml"
echo "=========================================="
