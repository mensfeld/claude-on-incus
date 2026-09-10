package session

import (
	"fmt"
	"strings"

	"github.com/mensfeld/code-on-incus/internal/container"
)

// AI coding agents auto-inject attribution into commit messages — a
// `Co-Authored-By: <tool bot>` trailer and/or a "Generated with …" footer —
// polluting authorship in the repos they work on (#788). Git has no setting
// that forbids a trailer, and per-tool opt-outs are easy to miss, so the
// tool-agnostic enforcement point is a global commit-msg hook: COI writes a
// root-owned hook directory at /etc/coi/git-hooks inside the container and
// points core.hooksPath at it. The hook STRIPS matching lines (never rejects —
// a rejected commit would break autonomous sessions over a cosmetic issue) and
// then delegates to the repository's own hook so repo hooks keep working.
//
// Known limitations (documented, accepted):
//   - A repo whose LOCAL git config sets core.hooksPath (husky writes
//     `core.hooksPath = .husky` into .git/config, which COI mounts read-only
//     from the host) overrides the global hooksPath — the strip does not run
//     in such repos.
//   - `git commit --no-verify` skips commit-msg hooks entirely.
//
// For Claude Code both gaps are additionally covered at the source by
// includeCoAuthoredBy=false in the managed-settings policy (setup_helpers.go).

// GitHooksDir is the root-owned in-container directory core.hooksPath points at.
const GitHooksDir = "/etc/coi/git-hooks"

// gitAttributionPatternsPath is the ERE pattern file the commit-msg hook feeds
// to `grep -Ev -f`; kept separate from the script so operators can inspect what
// is stripped without reading shell.
const gitAttributionPatternsPath = GitHooksDir + "/attribution-patterns"

// DefaultAttributionPatterns are the grep -E line patterns stripped by default:
// co-author trailers naming known AI tools or bot-style identities, and
// tool-generated "Generated with/by" footers. Overridden wholesale by
// [git] strip_attribution_patterns.
var DefaultAttributionPatterns = []string{
	`^[[:space:]]*Co-[Aa]uthored-[Bb]y:.*(noreply|no-reply|\[bot\]|[Cc]laude|[Cc]odex|[Cc]opilot|[Gg]emini|[Aa]ider|[Cc]ursor)`,
	`^[[:space:]]*(🤖[[:space:]]*)?Generated (with|by) `,
	`^[[:space:]]*(🤖[[:space:]]*)?Assisted-[Bb]y:`,
}

// delegatedHookNames are the client-side hooks that get a pure-delegation
// wrapper: core.hooksPath REPLACES the repository hooks directory (git looks
// only there), so without these the repo's own pre-commit/pre-push/... would
// silently stop running in-container. Deliberately excludes high-frequency
// plumbing hooks (reference-transaction, post-index-change) — a fork+exec on
// every ref update is not worth delegating hooks repos rarely use.
var delegatedHookNames = []string{
	"applypatch-msg", "pre-applypatch", "post-applypatch",
	"pre-commit", "pre-merge-commit", "prepare-commit-msg", "post-commit",
	"pre-rebase", "post-checkout", "post-merge", "pre-push",
	"post-rewrite", "pre-auto-gc",
}

// gitDelegateHookScript forwards to the repository's own hook of the same
// name, preserving arguments and exit code. `git rev-parse --git-dir` is used
// (NOT --git-path hooks, which would resolve back to core.hooksPath — i.e.
// this directory — and recurse).
const gitDelegateHookScript = `#!/bin/sh
# Managed by coi: core.hooksPath points at this directory, which would
# otherwise hide the repository's own hooks — forward to them.
hook="$(git rev-parse --git-dir 2>/dev/null)/hooks/$(basename "$0")"
if [ -x "$hook" ]; then
	exec "$hook" "$@"
fi
exit 0
`

// gitCommitMsgHookScript strips attribution lines from the commit message,
// then delegates to the repository's own commit-msg hook. Strip-don't-reject:
// every failure path keeps the original message and exits 0.
const gitCommitMsgHookScript = `#!/bin/sh
# Managed by coi ([git] strip_attribution): removes auto-injected AI
# attribution trailers/footers from every commit message, then runs the
# repository's own commit-msg hook. Never blocks a commit.
msg="$1"
patterns="` + gitAttributionPatternsPath + `"
if [ -f "$msg" ] && [ -s "$patterns" ]; then
	tmp="$msg.coi-strip"
	# grep -v exits 1 when NO line survives (message was attribution-only);
	# keep the original in that case rather than committing an empty message.
	if grep -Ev -f "$patterns" "$msg" > "$tmp" 2>/dev/null; then
		# Drop the trailing blank lines left where trailers were removed.
		awk 'NF{last=NR} {line[NR]=$0} END{for(i=1;i<=last;i++) print line[i]}' "$tmp" > "$msg" || :
	fi
	rm -f "$tmp"
fi
hook="$(git rev-parse --git-dir 2>/dev/null)/hooks/commit-msg"
if [ -x "$hook" ]; then
	exec "$hook" "$@"
fi
exit 0
`

// renderAttributionPatternsFile joins the pattern list into the grep -f file
// content. Blank/whitespace-only entries are dropped: an empty pattern line
// matches EVERY line, which with grep -v would delete the whole message.
func renderAttributionPatternsFile(patterns []string) string {
	var b strings.Builder
	for _, p := range patterns {
		if strings.TrimSpace(p) == "" {
			continue
		}
		b.WriteString(p)
		b.WriteString("\n")
	}
	return b.String()
}

// effectiveAttributionPatterns resolves the pattern list: config override when
// non-empty, built-in defaults otherwise.
func effectiveAttributionPatterns(configured []string) []string {
	if len(configured) > 0 {
		return configured
	}
	return DefaultAttributionPatterns
}

// SetupGitAttributionHook installs the strip hook inside the container: the
// root-owned hook directory (commit-msg + delegation wrappers + pattern file)
// and, for a writable global gitconfig, core.hooksPath pointing at it. With
// [git] readonly the caller instead bakes core.hooksPath into the mounted
// gitconfig (renderReadonlyGitConfig) and passes setHooksPath=false here.
// Root ownership keeps the sandboxed agent from editing its own policy.
// Non-fatal: logs a warning on failure (cosmetic feature, never blocks a
// session), mirroring SetupGitIdentity.
func SetupGitAttributionHook(mgr container.ContainerManager, homeDir string, patterns []string, setHooksPath bool, logger func(string)) {
	files := []struct {
		path, content, mode string
	}{
		{GitHooksDir + "/commit-msg", gitCommitMsgHookScript, "0755"},
		{GitHooksDir + "/delegate", gitDelegateHookScript, "0755"},
		{gitAttributionPatternsPath, renderAttributionPatternsFile(effectiveAttributionPatterns(patterns)), "0644"},
	}
	if _, err := mgr.ExecCommand("mkdir -p "+GitHooksDir, container.ExecCommandOptions{Capture: true}); err != nil {
		logger(fmt.Sprintf("Warning: failed to create %s: %v", GitHooksDir, err))
		return
	}
	for _, f := range files {
		if err := mgr.CreateFileWithOwner(f.path, f.content, 0, 0, f.mode); err != nil {
			logger(fmt.Sprintf("Warning: failed to write %s: %v", f.path, err))
			return
		}
	}
	// Delegation wrappers as symlinks to the single delegate script (ln -sf is
	// idempotent for persistent reuse).
	var links strings.Builder
	for _, name := range delegatedHookNames {
		fmt.Fprintf(&links, "ln -sf delegate %s/%s && ", GitHooksDir, name)
	}
	if _, err := mgr.ExecCommand(links.String()+"true", container.ExecCommandOptions{Capture: true}); err != nil {
		logger(fmt.Sprintf("Warning: failed to link delegation hooks: %v", err))
		return
	}
	if setHooksPath {
		cmd := fmt.Sprintf(`HOME=%s git config --global core.hooksPath %s`,
			shellEscape(homeDir), shellEscape(GitHooksDir))
		if _, err := mgr.ExecCommand(cmd, container.ExecCommandOptions{Capture: true}); err != nil {
			logger(fmt.Sprintf("Warning: failed to set core.hooksPath: %v", err))
			return
		}
	}
	logger("Installed AI-attribution strip hook (git commit messages keep only the configured author)")
}

// RemoveGitAttributionHookConfig best-effort unsets core.hooksPath so a
// persistent container reused after [git] strip_attribution was turned off
// converges instead of keeping the hook active. The hook directory itself is
// left in place (inert without the config).
func RemoveGitAttributionHookConfig(mgr container.ContainerExecution, homeDir string) {
	cmd := fmt.Sprintf(`HOME=%s git config --global --unset core.hooksPath 2>/dev/null || true`,
		shellEscape(homeDir))
	_, _ = mgr.ExecCommand(cmd, container.ExecCommandOptions{Capture: true})
}
