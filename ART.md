Here's [claude-on-incus](https://github.com/mensfeld/claude-on-incus) (or `coi` for short) - a tool for running Claude Code freely in isolated Incus containers.

If it's useful to you, [a star helps](https://github.com/mensfeld/claude-on-incus).

*Note: I'm also working on "code-on-incus" - a generalized version for running any AI coding assistant in isolated containers.*

<script src="https://asciinema.org/a/cRpuwdB5PwYNtjYu.js" id="asciicast-cRpuwdB5PwYNtjYu" async="true" data-autoplay="1" data-poster="00:03"></script>

## Why?

Three reasons: security, a clean host, and full contextual environments.

### Security

Claude Code inherits your entire shell environment. Your SSH keys, git credentials, `.env` files with API tokens - everything. You either click "Allow" hundreds of times per session, or use `--dangerously-skip-permissions` and hope nothing goes wrong.

With `coi`, Claude runs in complete isolation. Your host credentials stay on the host. Claude can't leak what Claude can't see.

**What remains exposed:** The Claude API token must be present inside the container, and your mounted workspace files are accessible. A malicious or compromised model could theoretically exfiltrate these over the network. Network filtering to restrict outbound connections is under development.

### Clean host, full capabilities

Claude loves installing things. Different Node versions, Python packages, Docker images, random build tools. On bare metal, this clutters your system with dependencies you may actually not need.

With `coi`, Claude can install and run whatever the task requires - without any of it touching your host. Need a specific Ruby version for one project? A Rust toolchain for another? Let Claude set it up in the container. Keep it if useful, throw it away if not.

VM-like isolation, Docker-like speed. Containers start in ~2 seconds.

### Contextual environments

Each project can have its own persistent container with Claude's installed context and setup. Your web project has Node 20 and React tools. Your data project has Python 3.11 with pandas and jupyter. Your embedded project has cross-compilers and debugging tools.

Claude remembers what it installed and configured - per project, completely isolated from each other.

## Why Incus over Docker?

Claude often needs to run Docker itself. Docker-in-Docker is a mess - you either bind-mount the host socket (defeating isolation) or run privileged mode (no security). Incus runs system containers where Docker works natively without hacks.

Incus also handles UID mapping automatically. No more `chown` after every session.

## Quick start

```bash
# Install (or build from sources if you prefer)
curl -fsSL https://raw.githubusercontent.com/mensfeld/claude-on-incus/master/install.sh | bash

# Build image (first time only)
coi build

# Start coding
cd your-project
coi shell
```

## Features And Capabilities

- **Multi-slot sessions** - Run parallel Claude instances for different tasks. Each slot has its own isolated home directory, so files don't leak between sessions.

```bash
coi shell --slot 1  # Frontend work
coi shell --slot 2  # API debugging
```

- **Session resume** - Stop working, come back tomorrow, pick up where you left off with full conversation history. Sessions are workspace-scoped, so you'll never accidentally resume a conversation from a different project:

```bash
coi shell --resume
```

- **Persistent containers** - By default, containers are ephemeral but your workspace files always persist. Enable persistence to also keep your installed tools between sessions:

```bash
coi shell --persistent
```

- **Detachable sessions** - All sessions run in tmux, allowing you to detach from running work and reattach later without losing progress. Your code analysis or long-running task continues in the background:

```bash
# Detach: Press Ctrl+b d
# Reattach to running session
coi attach
```

## The "dangerous" flags are much safer now

Claude Code's `--dangerously-skip-permissions` flag has that name for good reason when running on bare metal. Inside a `coi` container, the threat model changes completely:

| Risk | Bare metal | Inside coi |
|------|-----------|------------|
| SSH key exposure | Yes | No - keys not mounted |
| Git credential theft | Yes | No - credentials not present |
| Environment variable leaks | Yes | No - host env not inherited |
| Docker socket access | Yes | No - separate Docker daemon |
| Host filesystem access | Full | Only mounted workspace |

The "dangerous" flags give Claude full autonomy to work efficiently. The container isolation ensures that autonomy can't be weaponized against you.

## Summary

`coi` gives you secure, isolated Claude Code sessions that don't pollute your host. Install anything, experiment freely, keep what works, discard what doesn't.

The project is MIT licensed on [GitHub](https://github.com/mensfeld/claude-on-incus).