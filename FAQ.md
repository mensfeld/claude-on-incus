# Frequently Asked Questions

## General

### How is this different from Docker?

**Better file permissions** - Incus automatically maps container UIDs to host UIDs. No more `chown` after every operation.

**True isolation** - System containers, not application containers. Claude can run Docker, systemd services, etc.

**Native Docker support** - Run Docker inside containers without DinD hacks.

**Multi-user friendly** - Proper UID namespacing for better security.

### Can I run this on macOS or Windows?

**No.** Incus is Linux-only because it uses Linux kernel features (namespaces, cgroups).

For macOS/Windows, use:
- [claudebox](https://github.com/RchGrav/claudebox) (Docker-based)
- [run-claude-docker](https://github.com/icanhasjonas/run-claude-docker)

### Can I run multiple Claude sessions on the same project?

**Yes!** Use slots:

```bash
# Terminal 1
coi shell --slot 1

# Terminal 2 (same project)
coi shell --slot 2

# Terminal 3 (same project)
coi shell --slot 3
```

Each slot gets its own container but shares the workspace files.

### How much disk space do I need?

- **Incus itself:** ~100MB
- **coi image:** ~500MB
- **Per container (persistent):** ~200MB base + your tools

**Recommendation:** 5GB free space for comfortable usage.

### Is this production-ready?

**Yes!** All core features are implemented and tested:
- Comprehensive integration test suite
- Comprehensive error handling
- Stable API

Current version: **0.1.0** (see [CHANGELOG](CHANGELOG.md))

### How do I update?

```bash
# Re-run installer
curl -fsSL https://raw.githubusercontent.com/mensfeld/claude-on-incus/master/install.sh | bash

# Or build from source
cd claude-on-incus
git pull
make install
```

Containers and sessions are preserved during updates.

## Security

### Are the `--dangerous` flags actually safe?

**Yes**, inside containers they're completely safe:

- `--dangerously-disable-sandbox` disables Node.js sandbox in the **container**, not your host
- `--dangerously-allow-write-to-root` allows writes to **container root**, not your host filesystem
- Even if Claude tries malicious actions, they're contained within the isolated container
- Containers are ephemeral by default - any damage is wiped on exit

### What credentials does Claude have access to?

By default, **none of your host credentials**:

- SSH keys in `~/.ssh/` - NOT accessible
- Environment variables - NOT inherited (unless you pass `--env`)
- Git credentials - NOT accessible
- API keys in `.env` files - NOT accessible (unless in mounted workspace)

Only your workspace directory is mounted. Everything else is isolated.

### Can Claude access my Docker daemon?

**No.** Even though containers can run Docker internally, they run their own Docker daemon inside the container. Your host Docker daemon is not accessible.

## Usage

### What's the difference between ephemeral and persistent mode?

| Mode | Container | Workspace Files | Installed Packages |
|------|-----------|----------------|-------------------|
| **Ephemeral** (default) | Deleted on exit | Persisted | Deleted |
| **Persistent** | Kept running | Persisted | Persisted |

**Your workspace files always persist**, regardless of mode.

### When should I use persistent mode?

Use persistent mode when:
- You install system packages (`apt install`, `cargo install`)
- You have long-running build processes
- You want faster startup (no container recreation)
- You're doing iterative development with the same tools

### Can I attach to a running Claude session from another terminal?

**Yes!** Use:

```bash
coi attach                    # List sessions and select
coi attach claude-abc123-1    # Attach to specific container
```

This connects to the running tmux session inside the container.

### How do I clean up old containers and sessions?

```bash
coi clean              # Remove stopped containers (with confirmation)
coi clean --force      # Skip confirmation
coi clean --sessions   # Also remove saved session data
coi clean --all        # Remove everything
```

### Can I use a different base image?

**Yes**, you can specify any Incus image:

```bash
coi shell --image ubuntu:24.04
coi shell --image debian:12
```

Or configure in your project `.claude-on-incus.toml`:

```toml
[defaults]
image = "ubuntu:24.04"
```

## Troubleshooting

### Files created in container have wrong owner

This should **never** happen with `coi` - it's a core feature!

If it does:
1. Verify you're using the `coi` image
2. Check UID mapping: `incus config get <container> raw.idmap`
3. Report as a bug

### Container exits immediately

Check the logs:

```bash
coi list  # Get container name
incus info <container-name> --show-log
```

Common causes:
- Claude CLI not installed in image
- Workspace path doesn't exist
- Network connectivity issues

### "Device or resource busy" when cleaning up

The container may still be running:

```bash
incus list  # Find container
incus stop <container-name> --force
incus delete <container-name> --force
```

### Session resume doesn't work

Check if `.claude` directory was saved:

```bash
ls ~/.claude-on-incus/sessions/<session-id>/.claude/
```

If missing, the session may not have exited cleanly. Use Ctrl+C or `/exit` to exit Claude properly.

## Development

### How do I build a custom image?

```bash
# Create a build script
cat > setup.sh <<'EOF'
#!/bin/bash
apt-get update
apt-get install -y your-package
EOF

# Build custom image
coi build custom --script setup.sh --name my-custom-image

# Use it
coi shell --image my-custom-image
```

### Can I contribute?

Yes! See the [GitHub repository](https://github.com/mensfeld/claude-on-incus) for:
- Issue tracker
- Pull requests
- Discussions

### Where are the integration tests?

See [INTE.md](INTE.md) for comprehensive testing documentation.

## Getting Help

- Read the [README](README.md)
- Check [CHANGELOG](CHANGELOG.md) for recent changes
- Report issues at [GitHub Issues](https://github.com/mensfeld/claude-on-incus/issues)
- Join discussions at [GitHub Discussions](https://github.com/mensfeld/claude-on-incus/discussions)
