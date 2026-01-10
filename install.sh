#!/usr/bin/env bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
REPO="mensfeld/claude-on-incus"
BINARY_NAME="coi"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${VERSION:-latest}"

# Detect OS and architecture
detect_platform() {
    local os
    local arch

    os="$(uname -s)"
    arch="$(uname -m)"

    case "$os" in
        Linux*)
            OS="linux"
            ;;
        *)
            echo -e "${RED}✗ Unsupported OS: $os${NC}"
            echo "  claude-on-incus only supports Linux (Incus is Linux-only)"
            exit 1
            ;;
    esac

    case "$arch" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            echo -e "${RED}✗ Unsupported architecture: $arch${NC}"
            exit 1
            ;;
    esac

    echo -e "${BLUE}→ Detected platform: ${OS}/${ARCH}${NC}"
}

# Check if Incus is installed
check_incus() {
    echo -e "${BLUE}→ Checking Incus installation...${NC}"

    if ! command -v incus &> /dev/null; then
        echo -e "${YELLOW}⚠ Incus not found${NC}"
        echo ""
        echo "  claude-on-incus requires Incus to be installed."
        echo "  Install Incus: https://linuxcontainers.org/incus/docs/main/installing/"
        echo ""
        echo "  Quick install (Ubuntu/Debian):"
        echo "    sudo apt update"
        echo "    sudo apt install -y incus"
        echo "    sudo incus admin init --auto"
        echo "    sudo usermod -aG incus-admin \$USER"
        echo ""
        read -p "Continue installation anyway? [y/N] " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    else
        echo -e "${GREEN}✓ Incus found: $(incus version)${NC}"
    fi
}

# Check if user is in incus-admin group
check_group() {
    if groups | grep -q incus-admin; then
        echo -e "${GREEN}✓ User is in incus-admin group${NC}"
    else
        echo -e "${YELLOW}⚠ User is not in incus-admin group${NC}"
        echo ""
        echo "  You need to be in the incus-admin group to use claude-on-incus."
        echo "  Run: sudo usermod -aG incus-admin \$USER"
        echo "  Then log out and back in for changes to take effect."
        echo ""
    fi
}

# Download binary from GitHub releases
download_binary() {
    local download_url
    local tmp_dir
    local binary_path

    echo -e "${BLUE}→ Downloading claude-on-incus...${NC}"

    tmp_dir="$(mktemp -d)"
    trap "rm -rf '$tmp_dir'" EXIT

    if [ "$VERSION" = "latest" ]; then
        download_url="https://github.com/${REPO}/releases/latest/download/coi-${OS}-${ARCH}"
    else
        download_url="https://github.com/${REPO}/releases/download/${VERSION}/coi-${OS}-${ARCH}"
    fi

    binary_path="${tmp_dir}/${BINARY_NAME}"

    if command -v curl &> /dev/null; then
        curl -fsSL "$download_url" -o "$binary_path"
    elif command -v wget &> /dev/null; then
        wget -q -O "$binary_path" "$download_url"
    else
        echo -e "${RED}✗ Neither curl nor wget found${NC}"
        echo "  Please install curl or wget and try again."
        exit 1
    fi

    chmod +x "$binary_path"

    # Install to system
    echo -e "${BLUE}→ Installing to ${INSTALL_DIR}...${NC}"

    if [ -w "$INSTALL_DIR" ]; then
        cp "$binary_path" "${INSTALL_DIR}/${BINARY_NAME}"
        ln -sf "${INSTALL_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/claude-on-incus"
    else
        sudo cp "$binary_path" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo ln -sf "${INSTALL_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/claude-on-incus"
    fi

    echo -e "${GREEN}✓ Installed to ${INSTALL_DIR}/${BINARY_NAME}${NC}"
}

# Build from source
build_from_source() {
    local tmp_dir

    echo -e "${BLUE}→ Building from source...${NC}"

    # Check for Go
    if ! command -v go &> /dev/null; then
        echo -e "${RED}✗ Go not found${NC}"
        echo "  Install Go: https://go.dev/doc/install"
        exit 1
    fi

    echo -e "${BLUE}→ Go version: $(go version)${NC}"

    tmp_dir="$(mktemp -d)"
    trap "rm -rf '$tmp_dir'" EXIT

    # Clone repository
    echo -e "${BLUE}→ Cloning repository...${NC}"
    git clone --depth 1 "https://github.com/${REPO}.git" "$tmp_dir"

    # Build
    cd "$tmp_dir"
    echo -e "${BLUE}→ Building binary...${NC}"
    make build

    # Install
    echo -e "${BLUE}→ Installing to ${INSTALL_DIR}...${NC}"
    if [ -w "$INSTALL_DIR" ]; then
        make install
    else
        sudo make install
    fi

    echo -e "${GREEN}✓ Built and installed${NC}"
}

# Post-install setup
post_install() {
    echo ""
    echo -e "${GREEN}✓ Installation complete!${NC}"
    echo ""
    echo "Next steps:"
    echo ""
    echo "  1. Build the COI image:"
    echo "     ${BLUE}coi build${NC}"
    echo ""
    echo "  2. (Optional) Set up ZFS for instant container creation:"
    echo "     ${BLUE}sudo apt install zfsutils-linux${NC}"
    echo "     ${BLUE}sudo incus storage create zfs-pool zfs size=50GiB${NC}"
    echo "     ${BLUE}incus profile device set default root pool=zfs-pool${NC}"
    echo ""
    echo "  3. Start your first session:"
    echo "     ${BLUE}coi shell${NC}"
    echo ""
    echo "  4. View available commands:"
    echo "     ${BLUE}coi --help${NC}"
    echo ""

    if ! groups | grep -q incus-admin; then
        echo -e "${YELLOW}⚠ Remember to add yourself to incus-admin group:${NC}"
        echo "   ${BLUE}sudo usermod -aG incus-admin \$USER${NC}"
        echo "   Then log out and back in."
        echo ""
    fi

    echo "Documentation: https://github.com/${REPO}"
    echo ""
}

# Main installation
main() {
    echo ""
    echo -e "${BLUE}════════════════════════════════════════${NC}"
    echo -e "${BLUE}  claude-on-incus (coi) installer${NC}"
    echo -e "${BLUE}════════════════════════════════════════${NC}"
    echo ""

    detect_platform
    check_incus
    check_group

    echo ""
    echo "Installation method:"
    echo "  1. Download pre-built binary (fastest)"
    echo "  2. Build from source"
    echo ""

    # Check if releases exist
    if curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" &> /dev/null; then
        read -p "Choose [1/2] (default: 1): " -n 1 -r
        echo ""

        case $REPLY in
            2)
                build_from_source
                ;;
            *)
                download_binary
                ;;
        esac
    else
        echo -e "${YELLOW}⚠ No pre-built binaries available, building from source...${NC}"
        build_from_source
    fi

    post_install
}

# Handle errors
error_handler() {
    echo ""
    echo -e "${RED}✗ Installation failed${NC}"
    echo ""
    echo "If you need help:"
    echo "  - Check the documentation: https://github.com/${REPO}"
    echo "  - File an issue: https://github.com/${REPO}/issues"
    exit 1
}

trap error_handler ERR

# Run main
main "$@"
