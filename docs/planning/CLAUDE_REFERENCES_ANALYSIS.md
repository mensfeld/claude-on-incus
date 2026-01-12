# Claude-Specific References Analysis

Analysis of Claude-specific references in the codebase for making the tool CLI-agnostic.

## Summary

Total Claude references found: ~200+ across the codebase

## Categories (Priority Order)

### 1. HIGH PRIORITY - Core Functionality (Hard Dependencies)

#### A. `.claude` Directory (State/Config Directory)
**Impact**: Used throughout for session state storage
**Locations**:
- `internal/session/setup.go` - 30+ references
- `internal/session/cleanup.go` - 15+ references
- `internal/cli/info.go` - 2 references
- `internal/cli/list.go` - 3 references
- `internal/cli/file.go` - 1 reference (example)
- `internal/cli/shell.go` - 5 references

**What it does**:
- Stores session history (`.claude/projects/-workspace/*.jsonl`)
- Stores credentials (`.claude/.credentials.json`)
- Stores settings (`.claude/settings.json`)
- Stores Claude-specific state directories

**Refactoring strategy**:
- Make state directory configurable per CLI tool
- Add abstraction layer for session state management
- Config: `state_dir = "~/.claude"` (default for Claude Code)
- Could be `~/.aider`, `~/.cursor`, etc. for other tools

#### B. Binary Name & Command Building
**Impact**: Hardcoded CLI binary name
**Locations**:
- `internal/cli/shell.go:286` - `claudeBinary := "claude"`
- `internal/cli/shell.go:360` - `claudeBinary := "claude"`

**Current logic**:
```go
claudeBinary := "claude"
if getEnvValue("COI_USE_DUMMY") == "1" {
    claudeBinary = "dummy"
}
```

**Refactoring strategy**:
- Add config option: `cli_binary = "claude"` (default)
- Support env var: `COI_CLI_BINARY=aider`
- Or profile-based: `[profiles.aider] cli_binary = "aider"`

#### C. Function/Variable Names
**Impact**: Semantic clarity, but not functional
**Examples**:
- `runClaude()` → `runCLI()` or `runTool()`
- `runClaudeInTmux()` → `runCLIInTmux()`
- `setupClaudeConfig()` → `setupCLIConfig()`
- `ClaudeConfigPath` → `CLIConfigPath` or `StateDir`
- `claudeBinary` → `cliBinary`
- `claudeDir` → `stateDir`
- `claudePath` → `statePath`
- `claudeJsonPath` → `stateConfigPath`

**Refactoring strategy**:
- Rename systematically in one pass
- Update all callers
- No breaking changes (internal only)

### 2. MEDIUM PRIORITY - User-Facing Content

#### A. Command Help Text & Descriptions
**Locations**:
- `internal/cli/shell.go:24` - "Start an interactive Claude session"
- `internal/cli/shell.go:25` - "Start an interactive Claude Code session..."
- `internal/cli/shell.go:209` - "Starting Claude session..."
- `internal/cli/attach.go:23` - "Attach to a running Claude session"
- `internal/cli/attach.go:91` - "No active Claude sessions"
- `internal/cli/attach.go:99` - "Active Claude sessions:"
- `internal/cli/tmux.go:18` - "Send commands to or capture output from Claude sessions..."
- `internal/cli/tmux.go:126` - "No active Claude sessions"
- `internal/cli/tmux.go:130` - "Active Claude sessions:"
- `internal/cli/build.go:19` - "Build Incus image for Claude sessions"
- `internal/cli/build.go:20` - "Build the coi Incus image for running Claude Code"
- `internal/cli/build.go:80` - "coi image (Docker + build tools + Claude CLI + GitHub CLI)"
- `internal/cli/root.go:29` - "Claude on Incus - Run Claude Code in isolated Incus containers"
- `internal/cli/root.go:30` - "claude-on-incus (coi) is a CLI tool for running Claude Code..."
- `internal/cli/root.go:34` - "# Start interactive Claude session (same as 'coi shell')"
- `internal/cli/images.go:249` - "coi image (Claude CLI, Node.js, Docker, GitHub CLI, tmux)"

**Refactoring strategy**:
- Make help text generic: "Start an interactive CLI session"
- Or configurable: Use tool name from config
- Template-based: "Run {cli_name} in isolated containers"

#### B. Comments & Documentation
**Locations**: Throughout (50+ comments)
- `internal/cli/shell.go:208` - "// Run Claude CLI"
- `internal/cli/shell.go:216` - "// - Persistent: container is reused, .claude stays..."
- `internal/cli/shell.go:283` - "// runClaude executes the Claude CLI..."
- `internal/cli/shell.go:292` - "// Build command - either bash for debugging or Claude CLI"
- `internal/session/cleanup.go:16` - "// Claude session ID for saving .claude data"
- `internal/session/cleanup.go:377` - "// Claude stores sessions in .claude/projects/..."
- `internal/session/setup.go:39` - "// Setup initializes a container for a Claude session"

**Refactoring strategy**:
- Update comments during function renames
- Make generic: "CLI tool" instead of "Claude"

### 3. LOW PRIORITY - Configuration & Metadata

#### A. Config File Paths
**Locations**:
- `internal/config/loader.go:14-16` - Config file paths
  - `/etc/claude-on-incus/config.toml`
  - `~/.config/claude-on-incus/config.toml`
  - `./.claude-on-incus.toml`

**Refactoring strategy**:
- Keep current paths (already uses `coi` as primary)
- `.claude-on-incus.toml` → `.coi.toml` (breaking change)
- Document as legacy: support both for backwards compatibility

#### B. Default Config Values
**Locations**:
- `internal/config/config.go:57` - `Model: "claude-sonnet-4-5"`
- `internal/config/loader.go:113` - `model = "claude-sonnet-4-5"`

**What it is**: Default AI model name (Claude-specific)

**Refactoring strategy**:
- Keep as default for Claude Code
- Make configurable per tool profile
- Example:
  ```toml
  [profiles.claude]
  cli_binary = "claude"
  model = "claude-sonnet-4-5"

  [profiles.aider]
  cli_binary = "aider"
  model = "gpt-4"
  ```

#### C. Package Import Paths
**Locations**: All files
- `github.com/mensfeld/claude-on-incus/internal/...`

**Refactoring strategy**:
- Keep as-is (rename would break everything)
- Or rename repo to more generic name (major breaking change)
- Consider: Tool branding vs. functionality

### 4. VERY LOW PRIORITY - Examples & Tests

#### A. Example Container Names
**Locations**:
- `internal/cli/shutdown.go:29-30` - `claude-abc12345-1` (examples)
- `internal/cli/kill.go:27-28` - `claude-abc12345-1` (examples)
- `internal/cli/attach.go:31` - `claude-abc123-1` (example)
- `internal/session/naming_test.go:183,188,193` - Test cases

**Refactoring strategy**:
- Update examples to use actual prefix: `coi-abc12345-1`
- Tests already use `coi-` prefix in most places

#### B. Legacy Test References
**Locations**:
- `internal/cli/list.go:51` - "claude-on-incus containers" (comment)
- `internal/cli/clean.go:51` - "claude-on-incus containers" (output)

**Refactoring strategy**:
- Change to "coi containers" for consistency

## Recommended Refactoring Order

### Phase 1: Core Abstraction (Breaking Changes)
1. **Add CLI Tool Configuration**
   ```toml
   [cli]
   binary = "claude"          # Which binary to run
   state_dir = "~/.claude"    # Where to store state
   config_file = ".claude.json"  # Config file name
   ```

2. **Rename Internal Functions** (non-breaking)
   - `runClaude()` → `runCLI()`
   - `setupClaudeConfig()` → `setupCLIConfig()`
   - Variables: `claudeBinary` → `cliBinary`

3. **Abstract State Directory Handling**
   - Create `StateManager` interface
   - Make `.claude` path configurable
   - Support tool-specific state locations

### Phase 2: User-Facing Polish
4. **Update Help Text** - Make generic or configurable
5. **Fix Examples** - Use correct `coi-` prefix
6. **Update Comments** - Generic terminology

### Phase 3: Advanced (Optional)
7. **Profile System Enhancement** - Per-tool profiles
8. **Auto-Detection** - Detect which CLI tool is available
9. **Config Migration** - Handle `.claude-on-incus.toml` → `.coi.toml`

## Impact Assessment

### Breaking Changes Required
- **State directory path** (if changed from `.claude`)
- **Config file names** (if changed from `.claude-on-incus.toml`)

### Non-Breaking Changes
- Function/variable renames (internal)
- Help text updates
- Comment updates
- Example updates

## Next Steps

**Recommended approach**: Start with Phase 1, Step 1
- Add `cli_binary` and `state_dir` config options
- Keep backwards compatibility with defaults
- Test with both `claude` and `dummy`
- Document the new configuration options

**Benefits**:
- Minimal breaking changes
- Clear path to multi-tool support
- Maintains current user experience
- Easy to extend for new tools
