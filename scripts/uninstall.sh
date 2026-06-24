#!/bin/sh
set -e

# Configuration
BIN_DIR="/usr/local/bin"
BINARY_NAME="hippo"

# Determine if sudo is needed and available
SUDO=""
if [ "$(id -u)" -ne 0 ]; then
    if command -v sudo >/dev/null 2>&1; then
        SUDO="sudo"
    else
        echo "Warning: You are not running as root and 'sudo' is not installed. Uninstallation may fail due to permissions."
    fi
fi

echo "Stopping any running HippoDrop instances..."
# Try using the binary itself to stop the daemon if it exists
if command -v "$BINARY_NAME" >/dev/null 2>&1; then
    "$BINARY_NAME" stop >/dev/null 2>&1 || true
fi

# Fallback: kill processes if still running
if pgrep "$BINARY_NAME" >/dev/null 2>&1; then
    echo "Sending SIGTERM to remaining HippoDrop processes..."
    $SUDO pkill "$BINARY_NAME" >/dev/null 2>&1 || true
fi

# Remove binary
if [ -f "${BIN_DIR}/${BINARY_NAME}" ]; then
    echo "Removing binary from ${BIN_DIR}/${BINARY_NAME}..."
    $SUDO rm "${BIN_DIR}/${BINARY_NAME}"
fi

# Clean up user level configuration directory (~/.hippodrop)
USER_CONFIG_DIR="${HOME}/.hippodrop"
if [ -d "${USER_CONFIG_DIR}" ]; then
    echo "Removing user configuration and database at ${USER_CONFIG_DIR}..."
    rm -rf "${USER_CONFIG_DIR}"
fi

echo "=========================================="
# Verify removal
if [ ! -f "${BIN_DIR}/${BINARY_NAME}" ]; then
    echo "HippoDrop uninstalled."
else
    echo "Warning: HippoDrop binary might still be present at ${BIN_DIR}/${BINARY_NAME}."
fi
echo "=========================================="
