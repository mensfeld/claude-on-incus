#!/bin/bash
# Install fake Claude into the container for testing
set -e

echo "Installing fake Claude CLI for testing..."

# Install fake Claude as /usr/local/bin/claude
cp /workspace/testdata/fake-claude/claude /usr/local/bin/claude
chmod +x /usr/local/bin/claude

# Verify it works
/usr/local/bin/claude --version

echo "✓ Fake Claude installed successfully"
