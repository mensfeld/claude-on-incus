#!/bin/bash
# Install fake Claude ALONGSIDE real Claude for testing
# Real Claude: /usr/local/bin/claude (normal)
# Fake Claude: /usr/local/bin/test-claude (for tests)
set -e

echo "Installing test-claude (fake Claude for testing)..."

# Install fake Claude as test-claude (alongside real claude)
cp /workspace/testdata/fake-claude/claude /usr/local/bin/test-claude
chmod +x /usr/local/bin/test-claude

# Create a wrapper script that chooses between them based on env var
cat > /usr/local/bin/claude-wrapper << 'EOF'
#!/bin/bash
# Wrapper that chooses real Claude or test-claude based on USE_TEST_CLAUDE env var

if [ "${USE_TEST_CLAUDE}" = "1" ]; then
    exec /usr/local/bin/test-claude "$@"
else
    exec /usr/local/bin/claude "$@"
fi
EOF

chmod +x /usr/local/bin/claude-wrapper

# Verify both are installed
echo "Checking installations..."
/usr/local/bin/test-claude --version
echo "✓ test-claude installed"

if [ -f "/usr/local/bin/claude" ]; then
    echo "✓ real claude already present"
else
    echo "⚠ real claude not found (will be installed separately)"
fi

echo "✓ Installation complete!"
echo ""
echo "Usage:"
echo "  USE_TEST_CLAUDE=1 coi shell  # Uses fake test-claude (fast)"
echo "  coi shell                     # Uses real claude (slow)"
