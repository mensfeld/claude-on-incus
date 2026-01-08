package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mensfeld/claude-on-incus/internal/container"
	"github.com/mensfeld/claude-on-incus/internal/session"
	"github.com/spf13/cobra"
)

var (
	debugShell bool
	background bool
	useTmux    bool
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Start an interactive Claude session",
	Long: `Start an interactive Claude Code session in a container (always runs in tmux).

All sessions run in tmux for monitoring and detach/reattach support:
  - Interactive: Automatically attaches to tmux session
  - Background: Runs detached, use 'coi tmux capture' to view output
  - Detach anytime: Ctrl+B d (session keeps running)
  - Reattach: Run 'coi shell' again in same workspace

Examples:
  coi shell                         # Interactive session in tmux
  coi shell --background            # Run in background (detached)
  coi shell --resume                # Resume latest session (auto)
  coi shell --resume=<session-id>   # Resume specific session (note: = is required)
  coi shell --continue=<session-id> # Same as --resume (alias)
  coi shell --privileged            # Privileged mode with Git/SSH
  coi shell --slot 2                # Use specific slot
  coi shell --debug                 # Launch bash for debugging
`,
	RunE: shellCommand,
}

func init() {
	shellCmd.Flags().BoolVar(&debugShell, "debug", false, "Launch interactive bash instead of Claude (for debugging)")
	shellCmd.Flags().BoolVar(&background, "background", false, "Run Claude in background tmux session (detached)")
	shellCmd.Flags().BoolVar(&useTmux, "tmux", true, "Use tmux for session management (default true)")
}

func shellCommand(cmd *cobra.Command, args []string) error {
	// Validate no unexpected positional arguments
	if len(args) > 0 {
		return fmt.Errorf("unexpected argument '%s' - did you mean --resume=%s? (note: use = when specifying session ID)", args[0], args[0])
	}

	// Get absolute workspace path
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return fmt.Errorf("invalid workspace path: %w", err)
	}

	// Check if Incus is available
	if !container.Available() {
		return fmt.Errorf("incus is not available - please install Incus and ensure you're in the incus-admin group")
	}

	// Get sessions directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	sessionsDir := filepath.Join(homeDir, ".coi", "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Handle resume flag (--resume or --continue)
	resumeID := resume
	if continueSession != "" {
		resumeID = continueSession // --continue takes precedence if both are provided
	}

	// Check if resume/continue flag was explicitly set
	resumeFlagSet := cmd.Flags().Changed("resume") || cmd.Flags().Changed("continue")

	// Auto-detect if flag was set but value is empty or "auto"
	if resumeFlagSet && (resumeID == "" || resumeID == "auto") {
		// Auto-detect latest for workspace
		resumeID, err = session.GetLatestSessionForWorkspace(sessionsDir, absWorkspace)
		if err != nil {
			// Fallback to global latest if no workspace-specific session found
			resumeID, err = session.GetLatestSession(sessionsDir)
			if err != nil {
				return fmt.Errorf("no previous session to resume: %w", err)
			}
		}
		fmt.Fprintf(os.Stderr, "Auto-detected session: %s\n", resumeID)
	} else if resumeID != "" {
		// Validate that the explicitly provided session exists
		if !session.SessionExists(sessionsDir, resumeID) {
			return fmt.Errorf("session '%s' not found - check available sessions with: coi list --all", resumeID)
		}
		fmt.Fprintf(os.Stderr, "Resuming session: %s\n", resumeID)
	}

	// When resuming, inherit persistent and privileged flags from the original session
	// unless they were explicitly overridden by the user
	if resumeID != "" {
		metadataPath := filepath.Join(sessionsDir, resumeID, "metadata.json")
		if metadata, err := session.LoadSessionMetadata(metadataPath); err == nil {
			// Inherit persistent flag if not explicitly set by user
			if !cmd.Flags().Changed("persistent") {
				persistent = metadata.Persistent
				if persistent {
					fmt.Fprintf(os.Stderr, "Inherited persistent mode from session\n")
				}
			}
			// Inherit privileged flag if not explicitly set by user
			if !cmd.Flags().Changed("privileged") {
				privileged = metadata.Privileged
				if privileged {
					fmt.Fprintf(os.Stderr, "Inherited privileged mode from session\n")
				}
			}
		}
	}

	// Generate or use session ID
	var sessionID string
	if resumeID != "" {
		sessionID = resumeID // Reuse the same session ID when resuming
	} else {
		sessionID, err = session.GenerateSessionID()
		if err != nil {
			return err
		}
	}

	// Allocate slot - always check for availability and auto-increment if needed
	slotNum := slot
	if slotNum == 0 {
		// No slot specified, find first available
		slotNum, err = session.AllocateSlot(absWorkspace, 10)
		if err != nil {
			return fmt.Errorf("failed to allocate slot: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Auto-allocated slot %d\n", slotNum)
	} else {
		// Slot specified, but check if it's available
		// If not, find next available slot starting from the specified one
		available, err := session.IsSlotAvailable(absWorkspace, slotNum)
		if err != nil {
			return fmt.Errorf("failed to check slot availability: %w", err)
		}

		if !available {
			// Slot is occupied, find next available starting from slot+1
			originalSlot := slotNum
			slotNum, err = session.AllocateSlotFrom(absWorkspace, slotNum+1, 10)
			if err != nil {
				return fmt.Errorf("slot %d is occupied and failed to find next available slot: %w", originalSlot, err)
			}
			fmt.Fprintf(os.Stderr, "Slot %d is occupied, using slot %d instead\n", originalSlot, slotNum)
		}
	}

	// Setup session
	setupOpts := session.SetupOptions{
		WorkspacePath:     absWorkspace,
		Image:             imageName,
		Privileged:        privileged,
		Persistent:        persistent,
		ResumeFromID:      resumeID,
		Slot:              slotNum,
		SessionsDir:       sessionsDir,
		SSHKeyPath:        filepath.Join(homeDir, ".ssh", "id_coi"),
		GitConfigPath:     filepath.Join(homeDir, ".gitconfig"),
		ClaudeConfigPath:  filepath.Join(homeDir, ".claude"),
		MountClaudeConfig: mountClaudeConfig,
	}

	if storage != "" {
		setupOpts.StoragePath = storage
	}

	fmt.Fprintf(os.Stderr, "Setting up session %s...\n", sessionID)
	result, err := session.Setup(setupOpts)
	if err != nil {
		return fmt.Errorf("failed to setup session: %w", err)
	}

	// Setup cleanup on exit
	defer func() {
		fmt.Fprintf(os.Stderr, "\nCleaning up session...\n")
		cleanupOpts := session.CleanupOptions{
			ContainerName: result.ContainerName,
			SessionID:     sessionID,
			Privileged:    privileged,
			Persistent:    persistent,
			SessionsDir:   sessionsDir,
			SaveSession:   true, // Always save session data
			Workspace:     absWorkspace,
		}
		if err := session.Cleanup(cleanupOpts); err != nil {
			fmt.Fprintf(os.Stderr, "Cleanup error: %v\n", err)
		}
	}()

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintf(os.Stderr, "\nReceived interrupt signal, cleaning up...\n")
		os.Exit(0) // Defer will run
	}()

	// Run Claude CLI
	fmt.Fprintf(os.Stderr, "\nStarting Claude session...\n")
	fmt.Fprintf(os.Stderr, "Session ID: %s\n", sessionID)
	fmt.Fprintf(os.Stderr, "Container: %s\n", result.ContainerName)
	fmt.Fprintf(os.Stderr, "Workspace: %s\n", absWorkspace)

	// Determine resume mode
	// When resuming, always pass --resume flag to Claude CLI so it knows which session to resume
	// The difference is:
	// - Persistent: container is reused, session data stays in container
	// - Ephemeral: container is recreated, we restore .claude dir and pass --resume
	useResumeFlag := (resumeID != "")
	restoreOnly := false // No longer used - always use --resume flag

	// Choose execution mode
	if useTmux {
		if background {
			fmt.Fprintf(os.Stderr, "Mode: Background (tmux)\n")
		} else {
			fmt.Fprintf(os.Stderr, "Mode: Interactive (tmux)\n")
		}
		if restoreOnly {
			fmt.Fprintf(os.Stderr, "Resume mode: Restored conversation (auto-detect)\n")
		} else if useResumeFlag {
			fmt.Fprintf(os.Stderr, "Resume mode: Persistent session\n")
		}
		fmt.Fprintf(os.Stderr, "\n")
		err = runClaudeInTmux(result, sessionID, background, useResumeFlag, restoreOnly)
	} else {
		fmt.Fprintf(os.Stderr, "Mode: Direct (no tmux)\n")
		if restoreOnly {
			fmt.Fprintf(os.Stderr, "Resume mode: Restored conversation (auto-detect)\n")
		} else if useResumeFlag {
			fmt.Fprintf(os.Stderr, "Resume mode: Persistent session\n")
		}
		fmt.Fprintf(os.Stderr, "\n")
		err = runClaude(result, sessionID, useResumeFlag, restoreOnly)
	}

	// Exit status 130 means interrupted by SIGINT (Ctrl+C) - this is normal, not an error
	if err != nil && err.Error() == "exit status 130" {
		return nil
	}

	return err
}

// runClaude executes the Claude CLI in the container interactively
func runClaude(result *session.SetupResult, sessionID string, useResumeFlag, restoreOnly bool) error {
	// Build command - either bash for debugging or Claude CLI
	var cmdToRun string
	if debugShell {
		// Debug mode: launch interactive bash
		cmdToRun = "bash"
	} else {
		// Interactive mode
		// In sandbox mode, use permission-mode bypassPermissions to skip all prompts
		// Including the initial acknowledgment warning
		permissionFlags := ""
		if !privileged {
			permissionFlags = "--permission-mode bypassPermissions "
		}
		// Build session flag:
		// - useResumeFlag: use --resume for persistent containers
		// - restoreOnly: no session flag, let Claude auto-detect from restored .claude
		// - neither: use --session-id for new sessions
		var sessionArg string
		if useResumeFlag {
			sessionArg = fmt.Sprintf(" --resume %s", sessionID)
		} else if !restoreOnly {
			sessionArg = fmt.Sprintf(" --session-id %s", sessionID)
		}

		cmdToRun = fmt.Sprintf("claude --verbose %s%s", permissionFlags, sessionArg)
	}

	// Execute in container
	user := container.ClaudeUID
	if result.RunAsRoot {
		user = 0
	}

	userPtr := &user

	// Build environment variables
	envVars := map[string]string{
		"HOME": result.HomeDir,
		"TERM": os.Getenv("TERM"), // Preserve terminal type
	}

	// Set IS_SANDBOX=1 in sandbox mode (non-privileged) so Claude knows it's sandboxed
	if !privileged {
		envVars["IS_SANDBOX"] = "1"
	}

	opts := container.ExecCommandOptions{
		User:        userPtr,
		Cwd:         "/workspace",
		Env:         envVars,
		Interactive: true, // Attach stdin/stdout/stderr for interactive session
	}

	_, err := result.Manager.ExecCommand(cmdToRun, opts)
	return err
}

// runClaudeInTmux executes Claude CLI in a tmux session for background/monitoring support
func runClaudeInTmux(result *session.SetupResult, sessionID string, detached bool, useResumeFlag, restoreOnly bool) error {
	tmuxSessionName := fmt.Sprintf("coi-%s", result.ContainerName)

	// Build Claude command
	var claudeCmd string
	if debugShell {
		// Debug mode: launch interactive bash
		claudeCmd = "bash"
	} else {
		// Interactive mode
		permissionFlags := ""
		if !privileged {
			permissionFlags = "--permission-mode bypassPermissions "
		}
		// Build session flag:
		// - useResumeFlag: use --resume for persistent containers
		// - restoreOnly: no session flag, let Claude auto-detect from restored .claude
		// - neither: use --session-id for new sessions
		var sessionArg string
		if useResumeFlag {
			sessionArg = fmt.Sprintf(" --resume %s", sessionID)
		} else if !restoreOnly {
			sessionArg = fmt.Sprintf(" --session-id %s", sessionID)
		}

		claudeCmd = fmt.Sprintf("claude --verbose %s%s", permissionFlags, sessionArg)
	}

	// Build environment variables
	user := container.ClaudeUID
	if result.RunAsRoot {
		user = 0
	}
	userPtr := &user

	// Get TERM with fallback
	termEnv := os.Getenv("TERM")
	if termEnv == "" {
		termEnv = "xterm-256color" // Fallback to widely compatible terminal
	}

	envVars := map[string]string{
		"HOME": result.HomeDir,
		"TERM": termEnv,
	}

	if !privileged {
		envVars["IS_SANDBOX"] = "1"
	}

	// Build environment export commands for tmux
	envExports := ""
	for k, v := range envVars {
		envExports += fmt.Sprintf("export %s=%q; ", k, v)
	}

	// Check if tmux session already exists
	checkSessionCmd := fmt.Sprintf("tmux has-session -t %s 2>/dev/null", tmuxSessionName)
	_, err := result.Manager.ExecCommand(checkSessionCmd, container.ExecCommandOptions{
		Capture: true,
		User:    userPtr,
	})

	if err == nil {
		// Session exists - attach or send command
		if detached {
			// Send command to existing session
			sendCmd := fmt.Sprintf("tmux send-keys -t %s %q Enter", tmuxSessionName, claudeCmd)
			_, err := result.Manager.ExecCommand(sendCmd, container.ExecCommandOptions{
				Capture: true,
				User:    userPtr,
			})
			if err != nil {
				return fmt.Errorf("failed to send command to existing tmux session: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Sent command to existing tmux session: %s\n", tmuxSessionName)
			fmt.Fprintf(os.Stderr, "Use 'coi tmux capture %s' to view output\n", result.ContainerName)
			return nil
		} else {
			// Attach to existing session
			fmt.Fprintf(os.Stderr, "Attaching to existing tmux session: %s\n", tmuxSessionName)
			attachCmd := fmt.Sprintf("tmux attach -t %s", tmuxSessionName)
			opts := container.ExecCommandOptions{
				User:        userPtr,
				Cwd:         "/workspace",
				Interactive: true,
			}
			_, err := result.Manager.ExecCommand(attachCmd, opts)
			return err
		}
	}

	// Create new tmux session
	if detached {
		// Background mode: create detached session
		createCmd := fmt.Sprintf(
			"tmux new-session -d -s %s -c /workspace 'cd /workspace && %s %s'",
			tmuxSessionName,
			envExports,
			claudeCmd,
		)
		opts := container.ExecCommandOptions{
			Capture: true,
			User:    userPtr,
		}
		_, err := result.Manager.ExecCommand(createCmd, opts)
		if err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Created background tmux session: %s\n", tmuxSessionName)
		fmt.Fprintf(os.Stderr, "Use 'coi tmux capture %s' to view output\n", result.ContainerName)
		fmt.Fprintf(os.Stderr, "Use 'coi tmux send %s \"<command>\"' to send commands\n", result.ContainerName)
		return nil
	} else {
		// Interactive mode: create session and attach
		createCmd := fmt.Sprintf(
			"tmux new-session -s %s -c /workspace '%s %s'",
			tmuxSessionName,
			envExports,
			claudeCmd,
		)
		opts := container.ExecCommandOptions{
			User:        userPtr,
			Cwd:         "/workspace",
			Interactive: true,
			Env:         envVars,
		}
		_, err := result.Manager.ExecCommand(createCmd, opts)
		return err
	}
}

