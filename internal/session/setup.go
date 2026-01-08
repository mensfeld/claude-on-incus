package session

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mensfeld/claude-on-incus/internal/container"
)

const (
	DefaultImage      = "images:ubuntu/22.04"
	SandboxImage      = "coi-sandbox"
	PrivilegedImage   = "coi-privileged"
	ClaudeUser        = "claude"
	ClaudeUID         = 1000
)

// SetupOptions contains options for setting up a session
type SetupOptions struct {
	WorkspacePath     string
	Image             string
	Privileged        bool
	Persistent        bool   // Keep container between sessions (don't delete on cleanup)
	ResumeFromID      string
	Slot              int
	StoragePath       string
	SessionsDir       string // e.g., ~/.claude-on-incus/sessions
	SSHKeyPath        string // e.g., ~/.ssh/id_coi
	GitConfigPath     string // e.g., ~/.gitconfig
	ClaudeConfigPath  string // e.g., ~/.claude (mount host Claude config)
	MountClaudeConfig bool   // Whether to mount host Claude config (default true)
	Logger            func(string)
}

// SetupResult contains the result of setup
type SetupResult struct {
	ContainerName string
	Manager       *container.Manager
	HomeDir       string
	RunAsRoot     bool
	Image         string
}

// Setup initializes a container for a Claude session
// This configures the container with workspace mounting and user setup
func Setup(opts SetupOptions) (*SetupResult, error) {
	result := &SetupResult{}

	// Default logger
	if opts.Logger == nil {
		opts.Logger = func(msg string) {
			fmt.Fprintf(os.Stderr, "[setup] %s\n", msg)
		}
	}

	// 1. Generate container name
	containerName := ContainerName(opts.WorkspacePath, opts.Slot)
	result.ContainerName = containerName
	result.Manager = container.NewManager(containerName)
	opts.Logger(fmt.Sprintf("Container name: %s", containerName))

	// 2. Determine image
	image := opts.Image
	if image == "" {
		image = SandboxImage
		if opts.Privileged {
			image = PrivilegedImage
		}
	}
	result.Image = image

	// Check if image exists
	exists, err := container.ImageExists(image)
	if err != nil {
		return nil, fmt.Errorf("failed to check image: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("image '%s' not found - run 'coi build' first", image)
	}

	// 3. Determine execution context
	usingCoiImage := image == SandboxImage || image == PrivilegedImage
	// coi images have the claude user pre-configured, so run as that user
	// Other images don't have this setup, so run as root
	result.RunAsRoot = !usingCoiImage
	if result.RunAsRoot {
		result.HomeDir = "/root"
	} else {
		result.HomeDir = "/home/" + ClaudeUser
	}

	// 4. Check if container already exists
	var skipLaunch bool
	exists, err = result.Manager.Exists()
	if err != nil {
		return nil, fmt.Errorf("failed to check if container exists: %w", err)
	}

	if exists {
		// Check if container is currently running
		running, err := result.Manager.Running()
		if err != nil {
			return nil, fmt.Errorf("failed to check if container is running: %w", err)
		}

		if running {
			// Container is running - this is an active session!
			if opts.Persistent {
				opts.Logger("Container already running, reusing...")
				skipLaunch = true
			} else {
				// ERROR: A running container exists for this slot, but we're not in persistent mode
				// This means AllocateSlot() gave us a slot that's already in use!
				return nil, fmt.Errorf("slot %d is already in use by a running container %s - this should not happen (bug in slot allocation)", opts.Slot, containerName)
			}
		} else {
			// Container exists but is stopped
			if opts.Persistent {
				// Restart the stopped persistent container
				opts.Logger("Restarting existing persistent container...")
				if err := result.Manager.Start(); err != nil {
					return nil, fmt.Errorf("failed to start container: %w", err)
				}
				skipLaunch = true
			} else {
				// Delete the stopped leftover container
				opts.Logger("Found stopped leftover container from previous session, deleting...")
				if err := result.Manager.Delete(true); err != nil {
					return nil, fmt.Errorf("failed to delete leftover container: %w", err)
				}
				// Brief pause to let Incus fully delete
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	// 5. Launch container if needed
	if !skipLaunch {
		opts.Logger(fmt.Sprintf("Launching container from %s...", image))
		ephemeral := !opts.Persistent
		if err := result.Manager.Launch(image, ephemeral); err != nil {
			return nil, fmt.Errorf("failed to launch container: %w", err)
		}
	}
	// 6. Wait for ready
	opts.Logger("Waiting for container to be ready...")
	if err := waitForReady(result.Manager, 30, opts.Logger); err != nil {
		return nil, err
	}

	// 7. When resuming: restore session data if container was recreated, then inject credentials
	if opts.ResumeFromID != "" {
		// If we launched a new container (not reusing persistent one), restore .claude from saved session
		if !skipLaunch && opts.SessionsDir != "" {
			if err := restoreSessionData(result.Manager, opts.ResumeFromID, result.HomeDir, opts.SessionsDir, opts.Logger); err != nil {
				opts.Logger(fmt.Sprintf("Warning: Could not restore session data: %v", err))
			}
		}

		// Always inject fresh credentials when resuming (whether persistent container or restored session)
		if opts.ClaudeConfigPath != "" {
			if err := injectCredentials(result.Manager, opts.ClaudeConfigPath, result.HomeDir, opts.Logger); err != nil {
				opts.Logger(fmt.Sprintf("Warning: Could not inject credentials: %v", err))
			}
		}
	}

	// 8. Mount workspace and storage
	if !skipLaunch {
		opts.Logger(fmt.Sprintf("Mounting workspace: %s", opts.WorkspacePath))
		if err := result.Manager.MountDisk("workspace", opts.WorkspacePath, "/workspace", true); err != nil {
			return nil, fmt.Errorf("failed to mount workspace: %w", err)
		}

		// 9. Mount storage if specified
		if opts.StoragePath != "" {
			if err := os.MkdirAll(opts.StoragePath, 0755); err != nil {
				return nil, fmt.Errorf("failed to create storage directory: %w", err)
			}
			opts.Logger(fmt.Sprintf("Mounting storage: %s", opts.StoragePath))
			if err := result.Manager.MountDisk("storage", opts.StoragePath, "/storage", true); err != nil {
				return nil, fmt.Errorf("failed to mount storage: %w", err)
			}
		}
	} else {
		opts.Logger("Reusing existing workspace and storage mounts")
	}

	// 9b. Setup Claude config (skip if resuming - .claude already restored)
	if opts.MountClaudeConfig && opts.ClaudeConfigPath != "" && opts.ResumeFromID == "" {
		// Check if host .claude directory exists
		if _, err := os.Stat(opts.ClaudeConfigPath); err == nil {
			if opts.Privileged {
				// PRIVILEGED MODE: Mount host .claude directory (shared credentials)
				if !skipLaunch {
					opts.Logger(fmt.Sprintf("Mounting Claude config: %s", opts.ClaudeConfigPath))
					claudeConfigDest := filepath.Join(result.HomeDir, ".claude")

					// Mount with shift=true for proper read/write access
					if err := result.Manager.MountDisk("claude-config", opts.ClaudeConfigPath, claudeConfigDest, true); err != nil {
						opts.Logger(fmt.Sprintf("Warning: Failed to mount Claude config: %v", err))
					}

					// Also mount .claude.json if it exists
					claudeJsonPath := filepath.Join(filepath.Dir(opts.ClaudeConfigPath), ".claude.json")
					if _, err := os.Stat(claudeJsonPath); err == nil {
						opts.Logger(fmt.Sprintf("Mounting .claude.json: %s", claudeJsonPath))
						claudeJsonDest := filepath.Join(result.HomeDir, ".claude.json")
						if err := result.Manager.MountDisk("claude-json", claudeJsonPath, claudeJsonDest, true); err != nil {
							opts.Logger(fmt.Sprintf("Warning: Failed to mount .claude.json: %v", err))
						}
					}
				}
			} else {
				// SANDBOX MODE: Copy and inject settings (but only if NOT resuming)
				// Only run on first launch, not when restarting persistent container
				if !skipLaunch {
					opts.Logger("Setting up Claude config for sandbox mode...")
					if err := setupClaudeConfigSandbox(result.Manager, opts.ClaudeConfigPath, result.HomeDir, opts.Logger); err != nil {
						opts.Logger(fmt.Sprintf("Warning: Failed to setup Claude config: %v", err))
					}
				} else {
					opts.Logger("Reusing existing Claude config (persistent container)")
				}
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to check Claude config directory: %w", err)
		}
	} else if opts.ResumeFromID != "" {
		opts.Logger("Resuming session - using restored .claude config")
	}

	// 10. Setup SSH keys (privileged only)
	if opts.Privileged && opts.SSHKeyPath != "" {
		if err := setupSSHKeys(result.Manager, result.HomeDir, opts.SSHKeyPath, opts.Logger); err != nil {
			opts.Logger(fmt.Sprintf("Warning: Failed to setup SSH keys: %v", err))
		}
	}

	// 11. Setup git config (privileged only)
	if opts.Privileged && opts.GitConfigPath != "" {
		if err := setupGitConfig(result.Manager, result.HomeDir, opts.GitConfigPath, opts.Logger); err != nil {
			opts.Logger(fmt.Sprintf("Warning: Failed to setup git config: %v", err))
		}
	}

	opts.Logger("Container setup complete!")
	return result, nil
}

// waitForReady waits for container to be ready
func waitForReady(mgr *container.Manager, maxRetries int, logger func(string)) error {
	for i := 0; i < maxRetries; i++ {
		running, err := mgr.Running()
		if err != nil {
			return fmt.Errorf("failed to check container status: %w", err)
		}

		if running {
			// Additional check: try to execute a simple command
			_, err := mgr.ExecCommand("echo ready", container.ExecCommandOptions{Capture: true})
			if err == nil {
				return nil
			}
		}

		time.Sleep(1 * time.Second)
		if i%5 == 0 && i > 0 {
			logger(fmt.Sprintf("Still waiting... (%ds)", i))
		}
	}

	return fmt.Errorf("container failed to become ready after %d seconds", maxRetries)
}

// restoreSessionData restores .claude directory from a saved session
// Used when resuming a non-persistent session (container was deleted and recreated)
func restoreSessionData(mgr *container.Manager, resumeID, homeDir, sessionsDir string, logger func(string)) error {
	sourceClaudeDir := filepath.Join(sessionsDir, resumeID, ".claude")

	// Check if directory exists
	if info, err := os.Stat(sourceClaudeDir); err != nil || !info.IsDir() {
		return fmt.Errorf("no saved session data found for %s", resumeID)
	}

	logger(fmt.Sprintf("Restoring session data from %s", resumeID))

	// Push .claude directory to container
	if err := mgr.PushDirectory(sourceClaudeDir, homeDir); err != nil {
		return fmt.Errorf("failed to push .claude directory: %w", err)
	}

	// Fix ownership if running as claude user
	if homeDir != "/root" {
		claudePath := filepath.Join(homeDir, ".claude")
		if err := mgr.Chown(claudePath, ClaudeUID, ClaudeUID); err != nil {
			return fmt.Errorf("failed to set ownership: %w", err)
		}
	}

	logger("Session data restored successfully")
	return nil
}

// injectCredentials copies credentials and essential config from host to container when resuming
// This ensures fresh authentication while preserving the session conversation history
func injectCredentials(mgr *container.Manager, hostClaudeConfigPath, homeDir string, logger func(string)) error {
	logger("Injecting fresh credentials and config for session resume...")

	// Copy .credentials.json from host to container
	credentialsPath := filepath.Join(hostClaudeConfigPath, ".credentials.json")
	if _, err := os.Stat(credentialsPath); err != nil {
		return fmt.Errorf("credentials file not found: %w", err)
	}

	destCredentials := filepath.Join(homeDir, ".claude", ".credentials.json")
	if err := mgr.PushFile(credentialsPath, destCredentials); err != nil {
		return fmt.Errorf("failed to push credentials: %w", err)
	}

	// Fix ownership if running as claude user
	if homeDir != "/root" {
		if err := mgr.Chown(destCredentials, ClaudeUID, ClaudeUID); err != nil {
			return fmt.Errorf("failed to set credentials ownership: %w", err)
		}
	}

	// Also copy .claude.json (sibling to .claude directory) if it exists
	// This file contains important config like theme, startup count, etc.
	claudeJsonPath := filepath.Join(filepath.Dir(hostClaudeConfigPath), ".claude.json")
	if _, err := os.Stat(claudeJsonPath); err == nil {
		logger("Copying .claude.json for session resume...")
		claudeJsonDest := filepath.Join(homeDir, ".claude.json")
		if err := mgr.PushFile(claudeJsonPath, claudeJsonDest); err != nil {
			logger(fmt.Sprintf("Warning: Failed to copy .claude.json: %v", err))
		} else {
			// Inject sandbox settings into .claude.json
			logger("Injecting sandbox settings into .claude.json...")
			injectCmd := fmt.Sprintf(
				`python3 -c 'import json; f=open("%s","r+"); d=json.load(f); d["allowDangerouslySkipPermissions"]=True; d["bypassPermissionsModeAccepted"]=True; d["permissions"]={"defaultMode":"bypassPermissions"}; f.seek(0); json.dump(d,f,indent=2); f.truncate()'`,
				claudeJsonDest,
			)
			if _, err := mgr.ExecCommand(injectCmd, container.ExecCommandOptions{Capture: true}); err != nil {
				logger(fmt.Sprintf("Warning: Failed to inject settings into .claude.json: %v", err))
			}

			// Fix ownership if running as claude user
			if homeDir != "/root" {
				if err := mgr.Chown(claudeJsonDest, ClaudeUID, ClaudeUID); err != nil {
					logger(fmt.Sprintf("Warning: Failed to set .claude.json ownership: %v", err))
				}
			}
		}
	}

	logger("Credentials and config injected successfully")
	return nil
}

// setupSSHKeys configures SSH keys for privileged containers
func setupSSHKeys(mgr *container.Manager, homeDir, sshKeyPath string, logger func(string)) error {
	// Check if SSH key exists
	if _, err := os.Stat(sshKeyPath); err != nil {
		return fmt.Errorf("SSH key not found: %s", sshKeyPath)
	}

	logger("Configuring SSH keys...")

	// Create .ssh directory
	sshDir := filepath.Join(homeDir, ".ssh")
	if _, err := mgr.ExecCommand(fmt.Sprintf("mkdir -p %s", sshDir), container.ExecCommandOptions{}); err != nil {
		return err
	}

	// Push SSH key
	destKey := filepath.Join(sshDir, "id_coi")
	if err := mgr.PushFile(sshKeyPath, destKey); err != nil {
		return err
	}

	// Set permissions
	if _, err := mgr.ExecCommand(fmt.Sprintf("chmod 600 %s", destKey), container.ExecCommandOptions{}); err != nil {
		return err
	}

	// Create SSH config
	sshConfig := fmt.Sprintf(`Host github.com
  IdentityFile %s
  StrictHostKeyChecking accept-new
  IdentitiesOnly yes
`, destKey)

	configPath := filepath.Join(sshDir, "config")
	if err := mgr.CreateFile(configPath, sshConfig); err != nil {
		return err
	}

	// Set permissions
	if _, err := mgr.ExecCommand(fmt.Sprintf("chmod 600 %s", configPath), container.ExecCommandOptions{}); err != nil {
		return err
	}

	// Fix ownership
	if err := mgr.Chown(sshDir, ClaudeUID, ClaudeUID); err != nil {
		return err
	}

	logger("SSH keys configured")
	return nil
}

// setupClaudeConfigSandbox copies .claude directory and injects sandbox settings
func setupClaudeConfigSandbox(mgr *container.Manager, hostClaudePath, homeDir string, logger func(string)) error {
	claudeDir := filepath.Join(homeDir, ".claude")

	// Create .claude directory in container
	logger("Creating .claude directory in container...")
	mkdirCmd := fmt.Sprintf("mkdir -p %s", claudeDir)
	if _, err := mgr.ExecCommand(mkdirCmd, container.ExecCommandOptions{Capture: true}); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	// Copy only essential files from .claude directory (skip debug logs with permission issues)
	essentialFiles := []string{
		".credentials.json",
		"config.yml",
		"settings.json",
	}

	logger(fmt.Sprintf("Copying essential Claude config files from %s", hostClaudePath))
	for _, filename := range essentialFiles {
		srcPath := filepath.Join(hostClaudePath, filename)
		if _, err := os.Stat(srcPath); err == nil {
			destPath := filepath.Join(claudeDir, filename)
			logger(fmt.Sprintf("  - Copying %s", filename))
			if err := mgr.PushFile(srcPath, destPath); err != nil {
				logger(fmt.Sprintf("  - Warning: Failed to copy %s: %v", filename, err))
			}
		} else {
			logger(fmt.Sprintf("  - Skipping %s (not found)", filename))
		}
	}

	// Create or update settings.json with sandbox settings
	settingsPath := filepath.Join(claudeDir, "settings.json")
	sandboxSettings := `{
  "includeCoAuthoredBy": false,
  "allowDangerouslySkipPermissions": true,
  "bypassPermissionsModeAccepted": true,
  "permissions": {
    "defaultMode": "bypassPermissions"
  }
}
`
	if err := mgr.CreateFile(settingsPath, sandboxSettings); err != nil {
		return fmt.Errorf("failed to create settings.json: %w", err)
	}

	logger("Claude config copied and sandbox settings injected in settings.json")

	// Copy and modify .claude.json (sibling to .claude directory)
	claudeJsonPath := filepath.Join(filepath.Dir(hostClaudePath), ".claude.json")
	logger(fmt.Sprintf("Checking for .claude.json at: %s", claudeJsonPath))

	if info, err := os.Stat(claudeJsonPath); err == nil {
		logger(fmt.Sprintf("Found .claude.json (size: %d bytes), copying to container...", info.Size()))
		claudeJsonDest := filepath.Join(homeDir, ".claude.json")

		// Push the file to container
		if err := mgr.PushFile(claudeJsonPath, claudeJsonDest); err != nil {
			return fmt.Errorf("failed to copy .claude.json: %w", err)
		}
		logger(fmt.Sprintf(".claude.json copied to %s", claudeJsonDest))

		// Inject sandbox settings into .claude.json
		logger("Injecting sandbox settings into .claude.json...")
		injectCmd := fmt.Sprintf(
			`python3 -c 'import json; f=open("%s","r+"); d=json.load(f); d["allowDangerouslySkipPermissions"]=True; d["bypassPermissionsModeAccepted"]=True; d["permissions"]={"defaultMode":"bypassPermissions"}; f.seek(0); json.dump(d,f,indent=2); f.truncate()'`,
			claudeJsonDest,
		)
		if _, err := mgr.ExecCommand(injectCmd, container.ExecCommandOptions{Capture: true}); err != nil {
			logger(fmt.Sprintf("Warning: Failed to inject settings into .claude.json: %v", err))
		} else {
			logger("Successfully injected sandbox settings into .claude.json")
		}

		// Fix ownership if running as claude user
		if homeDir != "/root" {
			logger(fmt.Sprintf("Fixing ownership of .claude.json to %d:%d", ClaudeUID, ClaudeUID))
			if err := mgr.Chown(claudeJsonDest, ClaudeUID, ClaudeUID); err != nil {
				return fmt.Errorf("failed to set .claude.json ownership: %w", err)
			}
		}

		// Fix ownership of entire .claude directory recursively
		if homeDir != "/root" {
			logger(fmt.Sprintf("Fixing ownership of entire .claude directory to %d:%d", ClaudeUID, ClaudeUID))
			chownCmd := fmt.Sprintf("chown -R %d:%d %s", ClaudeUID, ClaudeUID, claudeDir)
			if _, err := mgr.ExecCommand(chownCmd, container.ExecCommandOptions{Capture: true}); err != nil {
				return fmt.Errorf("failed to set .claude directory ownership: %w", err)
			}
		}

		logger(".claude.json setup complete")
	} else if os.IsNotExist(err) {
		logger(fmt.Sprintf("Warning: .claude.json not found at %s, skipping", claudeJsonPath))
	} else {
		return fmt.Errorf("failed to check .claude.json: %w", err)
	}

	return nil
}

// setupGitConfig configures git for privileged containers
func setupGitConfig(mgr *container.Manager, homeDir, gitConfigPath string, logger func(string)) error {
	// Check if gitconfig exists
	if _, err := os.Stat(gitConfigPath); err != nil {
		return fmt.Errorf("gitconfig not found: %s", gitConfigPath)
	}

	logger("Configuring git...")

	// Mount gitconfig
	gitconfigDest := filepath.Join(homeDir, ".gitconfig")
	if err := mgr.MountDisk("gitconfig", gitConfigPath, gitconfigDest, true); err != nil {
		return err
	}

	// Set GIT_SSH_COMMAND in bashrc
	bashrc := filepath.Join(homeDir, ".bashrc")
	sshKey := filepath.Join(homeDir, ".ssh", "id_coi")
	gitSSHCmd := fmt.Sprintf("export GIT_SSH_COMMAND=\"ssh -i %s -o StrictHostKeyChecking=accept-new\"", sshKey)

	cmd := fmt.Sprintf("echo '%s' >> %s", gitSSHCmd, bashrc)
	if _, err := mgr.ExecCommand(cmd, container.ExecCommandOptions{}); err != nil {
		return err
	}

	logger("Git configuration set up")
	return nil
}
