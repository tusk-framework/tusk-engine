#!/bin/bash
# Tusk Engine - Unix Auto-Installer (Ubuntu/macOS)
# Usage: curl -fsSL https://tusk.sh/install.sh | bash

set -e

TUSK_HOME="$HOME/.tusk"
BIN_DIR="$TUSK_HOME/bin"
PHP_DIR="$TUSK_HOME/php"

echo "--- Tusk Engine Auto-Installer ---"

# 1. Create Directories
mkdir -p "$BIN_DIR"
mkdir -p "$PHP_DIR"

# 2. Detect OS & Arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "$ARCH" == "x86_64" ]; then ARCH="amd64"; fi
if [ "$ARCH" == "aarch64" ] || [ "$ARCH" == "arm64" ]; then ARCH="arm64"; fi

# 3. Download Tusk Engine
# Mapping for GoReleaser names
G_OS="$(uname -s | sed 's/Darwin/Darwin/;s/Linux/Linux/')"
G_ARCH=$(uname -m)
if [ "$G_ARCH" == "x86_64" ]; then G_ARCH="x86_64"; fi
if [ "$G_ARCH" == "aarch64" ] || [ "$G_ARCH" == "arm64" ]; then G_ARCH="arm64"; fi

TUSK_VERSION_NAME="tusk_${G_OS}_${G_ARCH}"
TUSK_URL="https://github.com/tusk-framework/tusk-engine/releases/latest/download/${TUSK_VERSION_NAME}.tar.gz"

echo "Downloading Tusk Engine ($TUSK_VERSION_NAME)..."
curl -L -o "$TUSK_HOME/tusk.tar.gz" "$TUSK_URL"
tar -xzf "$TUSK_HOME/tusk.tar.gz" -C "$BIN_DIR"
rm "$TUSK_HOME/tusk.tar.gz"
chmod +x "$BIN_DIR/tusk"

# 4. Download & Setup PHP (Static Binary)
# Using a placeholder static PHP provider
PHP_URL="https://github.com/crazywhalecc/static-php-cli/releases/latest/download/php-8.3.3-static-${OS}-${ARCH}.tar.gz"

if [ ! -f "$PHP_DIR/php" ]; then
    echo "Downloading Static PHP..."
    curl -L -o "$TUSK_HOME/php.tar.gz" "$PHP_URL"
    tar -xzf "$TUSK_HOME/php.tar.gz" -C "$PHP_DIR"
    rm "$TUSK_HOME/php.tar.gz"
else
    echo "PHP already installed in $PHP_DIR"
fi

# 5. Update PATH
SHELL_TYPE=$(basename "$SHELL")
RC_FILE=""

case "$SHELL_TYPE" in
    bash) RC_FILE="$HOME/.bashrc" ;;
    zsh)  RC_FILE="$HOME/.zshrc" ;;
    *)    RC_FILE="$HOME/.profile" ;;
esac

if ! grep -q "$BIN_DIR" "$RC_FILE"; then
    echo "Adding Tusk to PATH in $RC_FILE..."
    echo "export PATH=\"\$PATH:$BIN_DIR:$PHP_DIR\"" >> "$RC_FILE"
fi

echo -e "\nInstallation Complete!"
echo "Please run: source $RC_FILE"
echo "Try: tusk --help"
