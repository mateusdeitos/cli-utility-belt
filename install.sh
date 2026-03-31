#!/bin/sh
set -e

REPO="mateusdeitos/cli-utility-belt"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY="belt"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  linux)  ASSET="belt-linux-${ARCH}" ;;
  darwin) ASSET="belt-darwin-${ARCH}" ;;
  *)
    echo "Unsupported OS: $OS"
    echo "For Windows, use: irm https://raw.githubusercontent.com/${REPO}/main/install.ps1 | iex"
    exit 1
    ;;
esac

URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "Downloading ${ASSET}..."
curl -fsSL "$URL" -o "/tmp/${BINARY}"
chmod +x "/tmp/${BINARY}"

if [ ! -d "$INSTALL_DIR" ]; then
  mkdir -p "$INSTALL_DIR"
fi

if [ -w "$INSTALL_DIR" ]; then
  mv "/tmp/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Moving binary to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "/tmp/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

echo "Installed to ${INSTALL_DIR}/${BINARY}"
echo "Run 'belt --help' to get started."
