# Generalizing claude-on-incus for Multiple AI Providers

This document outlines a plan to abstract the Claude-specific parts of this project to support other AI CLI tools (Aider, Cursor, Continue, etc.).

## Current State

The project has excellent separation of concerns:

- **Claude-Specific Code**: ~5% (~200 lines)
- **AI-Agnostic Infrastructure**: ~95% (~4000+ lines)

The container management, session lifecycle, slot allocation, and configuration systems are already provider-agnostic.

## Claude-Specific Touchpoints

### 1. CLI Invocation (`internal/cli/shell.go`)

```go
// Lines 275-292, 329-351
claudeCmd = fmt.Sprintf("claude --verbose %s%s", permissionFlags, sessionArg)
```

**What's specific:**
- Command name: `claude`
- Permission flag: `--permission-mode bypassPermissions`
- Resume flag: `--resume <session-id>`
- Session flag: `--session-id <id>`

### 2. Configuration Directory (`internal/session/setup.go`)

```go
// Lines 191-233, 307-360, 414-514
// Handles ~/.claude directory:
// - .credentials.json
// - settings.json
// - .claude.json
```

**What's specific:**
- Directory name: `.claude`
- Settings file format (JSON with specific keys)
- Credentials file structure

### 3. User Constants (`internal/image/sandbox.go`)

```go
// Lines 53-87
ClaudeUser = "claude"
ClaudeUID  = 1000
```

### 4. Package Installation (`internal/image/sandbox.go`)

```go
// Lines 89-104
npm install -g @anthropic-ai/claude-code
```

### 5. Default Model (`internal/config/config.go`)

```go
// Line 61
Default Model: "claude-sonnet-4-5"
```

## Proposed Abstraction

### Option A: Provider Interface (Recommended)

Create a provider interface that each AI tool implements:

```go
// internal/provider/provider.go

type Provider interface {
    // Identity
    Name() string        // "claude", "aider", "cursor"
    User() string        // Container username
    UID() int            // Container user ID

    // Configuration
    ConfigDir() string                      // ".claude", ".aider"
    InjectSettings(containerPath string, sandbox bool) error
    CredentialsFiles() []string             // Files to copy/mount

    // CLI
    BuildCommand(opts CommandOpts) string
    InstallCommands() []string              // Installation steps

    // Session
    SupportsResume() bool
    ResumeFlag(sessionID string) string
    SessionFlag(sessionID string) string
}

type CommandOpts struct {
    Sandbox      bool
    SessionID    string
    Resume       bool
    ResumeID     string
    ExtraArgs    []string
}
```

### Option B: Configuration-Driven

Make provider details fully configurable:

```toml
# ~/.config/coi/providers/claude.toml
[provider]
name = "claude"
user = "claude"
uid = 1000
config_dir = ".claude"

[provider.cli]
command = "claude"
verbose_flag = "--verbose"
permission_flag = "--permission-mode bypassPermissions"
session_flag = "--session-id"
resume_flag = "--resume"

[provider.install]
commands = ["npm install -g @anthropic-ai/claude-code"]

[provider.settings]
# JSON to inject into settings file
sandbox_settings = '''
{
  "allowDangerouslySkipPermissions": true,
  "bypassPermissionsModeAccepted": true,
  "permissions": {"defaultMode": "bypassPermissions"}
}
'''
```

### Recommendation

Use **Option A** (Provider Interface) with sensible defaults. This provides:
- Type safety and compile-time checks
- Easy testing with mock providers
- Clear contract for adding new providers
- Flexibility for complex providers

## Implementation Steps

### Phase 1: Extract Provider Interface

1. Create `internal/provider/provider.go` with the interface
2. Implement `internal/provider/claude/claude.go` for current behavior
3. Update `internal/cli/shell.go` to use provider interface
4. Update `internal/session/setup.go` to use provider methods

### Phase 2: Refactor Constants

1. Move hardcoded "claude" strings to provider methods
2. Update image building to use provider install commands
3. Make container user/UID configurable via provider

### Phase 3: Configuration Integration

1. Add `provider` field to config struct
2. Support provider selection via CLI flag: `--provider aider`
3. Allow provider-specific config sections in TOML

### Phase 4: Add Alternative Providers

Example providers to implement:

```go
// internal/provider/aider/aider.go
type AiderProvider struct{}

func (p *AiderProvider) Name() string { return "aider" }
func (p *AiderProvider) ConfigDir() string { return ".aider" }
func (p *AiderProvider) InstallCommands() []string {
    return []string{"pip install aider-chat"}
}
func (p *AiderProvider) BuildCommand(opts CommandOpts) string {
    return fmt.Sprintf("aider %s", strings.Join(opts.ExtraArgs, " "))
}
```

### Phase 5: Update Tests

1. Parameterize tests to run against multiple providers
2. Add provider-specific test fixtures
3. Create mock provider for unit tests

## Files to Modify

| File | Changes | Effort |
|------|---------|--------|
| `internal/provider/provider.go` | New - interface definition | Low |
| `internal/provider/claude/claude.go` | New - Claude implementation | Low |
| `internal/cli/shell.go` | Use provider interface | Medium |
| `internal/cli/root.go` | Add `--provider` flag | Low |
| `internal/session/setup.go` | Use provider for config injection | Medium |
| `internal/session/cleanup.go` | Use provider for config dir | Low |
| `internal/image/sandbox.go` | Use provider for install/user | Low |
| `internal/config/config.go` | Add provider field | Low |

## Naming Considerations

If generalizing, consider renaming:
- `claude-on-incus` → `ai-on-incus` or `coi` (already the binary name)
- `ClaudeUser` → `ContainerUser`
- `coi-sandbox` → `ai-sandbox` or keep as-is

## Backwards Compatibility

Maintain backwards compatibility by:
1. Defaulting to Claude provider if none specified
2. Keeping existing config format working
3. Supporting existing image names

## Estimated Effort

- **Phase 1-2**: 2-3 days
- **Phase 3**: 1 day
- **Phase 4** (per provider): 0.5-1 day each
- **Phase 5**: 1-2 days

**Total**: ~5-7 days for full abstraction with one alternative provider

## Example Usage After Generalization

```bash
# Current (unchanged)
coi shell

# With explicit provider
coi shell --provider claude
coi shell --provider aider

# Using config profile
# ~/.config/coi/config.toml
# [profiles.aider]
# provider = "aider"
# image = "coi-aider"

coi shell --profile aider
```

## Notes

- The container infrastructure (Incus management, UID shifting, mounts) requires zero changes
- Session resume semantics may differ per provider (some may not support it)
- Credential handling will be provider-specific
- Image building could be provider-aware (different base dependencies)
