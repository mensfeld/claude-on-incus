#!/bin/bash
# Build script for coi image
# This script runs INSIDE the container during image build
#
# It installs all dependencies needed for Claude Code execution:
# - Base development tools
# - Node.js LTS
# - Claude CLI
# - Docker
# - GitHub CLI
# - test-claude (fake Claude for testing)

set -euo pipefail

# Configuration
CODE_USER="code"
CODE_UID=1000

log() {
    echo "[coi] $*"
}

#######################################
# Install base dependencies
#######################################
install_base_dependencies() {
    log "Installing base dependencies..."

    apt-get update -qq

    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
        curl wget git ca-certificates gnupg jq unzip sudo \
        tmux \
        build-essential libssl-dev libreadline-dev zlib1g-dev \
        libffi-dev libyaml-dev libgmp-dev \
        libsqlite3-dev libpq-dev libmysqlclient-dev \
        libxml2-dev libxslt1-dev libcurl4-openssl-dev

    log "Base dependencies installed"
}

#######################################
# Install Node.js LTS
#######################################
install_nodejs() {
    log "Installing Node.js LTS..."

    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    apt-get install -y -qq nodejs

    log "Node.js $(node --version) installed"
}

#######################################
# Create code user with passwordless sudo
#######################################
create_code_user() {
    log "Creating code user..."

    # Rename ubuntu user to code
    usermod -l "$CODE_USER" -d "/home/$CODE_USER" -m ubuntu
    groupmod -n "$CODE_USER" ubuntu
    mkdir -p "/home/$CODE_USER/.claude"
    mkdir -p "/home/$CODE_USER/.ssh"
    chmod 700 "/home/$CODE_USER/.ssh"
    chown -R "$CODE_USER:$CODE_USER" "/home/$CODE_USER"

    # Setup passwordless sudo
    echo "$CODE_USER ALL=(ALL) NOPASSWD:ALL" > "/etc/sudoers.d/$CODE_USER"
    chown root:root "/etc/sudoers.d/$CODE_USER"
    chmod 440 "/etc/sudoers.d/$CODE_USER"
    usermod -aG sudo "$CODE_USER"

    log "User '$CODE_USER' created with passwordless sudo (uid: $CODE_UID)"
}

#######################################
# Install Claude CLI from npm
#######################################
install_claude_cli() {
    log "Installing Claude CLI..."

    npm install -g @anthropic-ai/claude-code

    log "Claude CLI $(claude --version 2>/dev/null || echo 'installed')"
}

#######################################
# Install test-claude (fake Claude for testing)
#######################################
install_test_claude() {
    log "Installing test-claude (fake Claude for testing)..."

    # test-claude MUST be pushed to /tmp/test-claude before running this script
    if [[ ! -f /tmp/test-claude ]]; then
        log "ERROR: /tmp/test-claude not found!"
        log "The test-claude script must be pushed to the container before building."
        log "Make sure you're running the build from the project root directory."
        exit 1
    fi

    cp /tmp/test-claude /usr/local/bin/test-claude
    chmod +x /usr/local/bin/test-claude
    rm /tmp/test-claude
    log "test-claude $(test-claude --version 2>/dev/null || echo 'installed')"
}

#######################################
# Install Docker CE
#######################################
install_docker() {
    log "Installing Docker..."

    # Add Docker GPG key
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    chmod a+r /etc/apt/keyrings/docker.gpg

    # Add Docker repository
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo $VERSION_CODENAME) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

    # Install Docker
    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
        docker-ce docker-ce-cli containerd.io \
        docker-buildx-plugin docker-compose-plugin

    # Add code user to docker group
    usermod -aG docker "$CODE_USER"

    log "Docker $(docker --version 2>/dev/null || echo 'installed')"
}

#######################################
# Install GitHub CLI
#######################################
install_github_cli() {
    log "Installing GitHub CLI..."

    # Add GitHub CLI GPG key
    curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
    chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg

    # Add GitHub CLI repository
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null

    # Install
    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq gh

    log "GitHub CLI $(gh --version 2>/dev/null | head -1 || echo 'installed')"
}

#######################################
# Cleanup
#######################################
cleanup() {
    log "Cleaning up..."
    apt-get clean
    rm -rf /var/lib/apt/lists/*
    log "Cleanup complete"
}

#######################################
# Main
#######################################
main() {
    log "Starting coi image build..."

    install_base_dependencies
    install_nodejs
    create_code_user
    install_claude_cli
    install_test_claude
    install_docker
    install_github_cli
    cleanup

    log "coi image build complete!"
}

main "$@"
