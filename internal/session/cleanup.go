package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mensfeld/claude-on-incus/internal/container"
)

// CleanupOptions contains options for cleaning up a session
type CleanupOptions struct {
	ContainerName string
	SessionID     string // Claude session ID for saving .claude data
	Privileged    bool
	Persistent    bool   // If true, stop but don't delete container
	SessionsDir   string // e.g., ~/.claude-on-incus/sessions
	SaveSession   bool   // Whether to save .claude directory
	Workspace     string // Workspace directory path
	Logger        func(string)
}

// Cleanup stops and deletes a container, optionally saving session data
func Cleanup(opts CleanupOptions) error {
	// Default logger
	if opts.Logger == nil {
		opts.Logger = func(msg string) {
			fmt.Fprintf(os.Stderr, "[cleanup] %s\n", msg)
		}
	}

	if opts.ContainerName == "" {
		opts.Logger("No container to clean up")
		return nil
	}

	mgr := container.NewManager(opts.ContainerName)

	// Check if container exists (it might have auto-deleted if ephemeral)
	exists, err := mgr.Exists()
	if err != nil {
		opts.Logger(fmt.Sprintf("Warning: Could not check container existence: %v", err))
	}

	// Save session data if requested and container exists
	// Note: We can pull files from stopped containers, so don't check if running
	if opts.SaveSession && exists && opts.SessionID != "" && opts.SessionsDir != "" {
		if err := saveSessionData(mgr, opts.SessionID, opts.Privileged, opts.Persistent, opts.Workspace, opts.SessionsDir, opts.Logger); err != nil {
			opts.Logger(fmt.Sprintf("Warning: Failed to save session data: %v", err))
		}
	}

	// Stop container
	opts.Logger(fmt.Sprintf("Stopping container %s...", opts.ContainerName))
	if err := mgr.Stop(true); err != nil {
		opts.Logger(fmt.Sprintf("Warning: Failed to stop container: %v", err))
	}

	// Delete container only if NOT persistent
	if !opts.Persistent {
		// Give container a moment to fully stop before deletion
		time.Sleep(500 * time.Millisecond)

		// Check if container still exists (ephemeral containers may auto-delete)
		exists, err := mgr.Exists()
		if err != nil {
			opts.Logger(fmt.Sprintf("Warning: Could not check container existence: %v", err))
		}

		if exists {
			opts.Logger(fmt.Sprintf("Deleting container %s...", opts.ContainerName))
			if err := mgr.Delete(true); err != nil {
				opts.Logger(fmt.Sprintf("Warning: Failed to delete container: %v", err))
			} else {
				opts.Logger("Container deleted successfully")
			}
		} else {
			opts.Logger("Container already removed")
		}
	} else {
		opts.Logger("Container stopped but kept (persistent mode)")
	}

	return nil
}

// saveSessionData saves the .claude directory from the container
func saveSessionData(mgr *container.Manager, sessionID string, privileged bool, persistent bool, workspace string, sessionsDir string, logger func(string)) error {
	// Determine home directory
	// For coi images, we always use /home/claude
	// For other images, we use /root
	// Since we currently only support coi images, always use /home/claude
	homeDir := "/home/" + ClaudeUser

	claudeDir := filepath.Join(homeDir, ".claude")

	// Check if .claude exists in container
	exists, err := mgr.DirExists(claudeDir)
	if err != nil {
		return fmt.Errorf("failed to check .claude directory: %w", err)
	}
	if !exists {
		logger("No .claude directory found in container")
		return nil
	}

	// Create local session directory
	localSessionDir := filepath.Join(sessionsDir, sessionID)
	if err := os.MkdirAll(localSessionDir, 0755); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	logger(fmt.Sprintf("Saving session data to %s", localSessionDir))

	// Remove old .claude directory if it exists (when resuming)
	localClaudeDir := filepath.Join(localSessionDir, ".claude")
	if _, err := os.Stat(localClaudeDir); err == nil {
		logger("Removing old session data before saving new state")
		if err := os.RemoveAll(localClaudeDir); err != nil {
			return fmt.Errorf("failed to remove old .claude directory: %w", err)
		}
	}

	// Pull .claude directory from container
	if err := mgr.PullDirectory(claudeDir, localSessionDir); err != nil {
		return fmt.Errorf("failed to pull .claude directory: %w", err)
	}

	// Save metadata
	metadata := SessionMetadata{
		SessionID:     sessionID,
		ContainerName: mgr.ContainerName,
		Privileged:    privileged,
		Persistent:    persistent,
		Workspace:     workspace,
		SavedAt:       getCurrentTime(),
	}

	metadataPath := filepath.Join(localSessionDir, "metadata.json")
	if err := saveMetadata(metadataPath, metadata); err != nil {
		// Non-fatal - session data is already saved
		logger(fmt.Sprintf("Warning: Failed to save metadata: %v", err))
	}

	logger("Session data saved successfully")
	return nil
}

// SessionMetadata contains information about a saved session
type SessionMetadata struct {
	SessionID     string `json:"session_id"`
	ContainerName string `json:"container_name"`
	Privileged    bool   `json:"privileged"`
	Persistent    bool   `json:"persistent"`
	Workspace     string `json:"workspace"`
	SavedAt       string `json:"saved_at"`
}

// saveMetadata saves session metadata to a JSON file
func saveMetadata(path string, metadata SessionMetadata) error {
	// Simple JSON marshaling
	content := fmt.Sprintf(`{
  "session_id": "%s",
  "container_name": "%s",
  "privileged": %t,
  "persistent": %t,
  "workspace": "%s",
  "saved_at": "%s"
}
`, metadata.SessionID, metadata.ContainerName, metadata.Privileged, metadata.Persistent, metadata.Workspace, metadata.SavedAt)

	return os.WriteFile(path, []byte(content), 0644)
}

// getCurrentTime returns current time in RFC3339 format
func getCurrentTime() string {
	return time.Now().Format(time.RFC3339)
}

// SessionExists checks if a session with the given ID exists and is valid
func SessionExists(sessionsDir, sessionID string) bool {
	claudePath := filepath.Join(sessionsDir, sessionID, ".claude")
	info, err := os.Stat(claudePath)
	return err == nil && info.IsDir()
}

// ListSavedSessions lists all saved sessions in the sessions directory
func ListSavedSessions(sessionsDir string) ([]string, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var sessions []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if it contains a .claude directory
			claudePath := filepath.Join(sessionsDir, entry.Name(), ".claude")
			if info, err := os.Stat(claudePath); err == nil && info.IsDir() {
				sessions = append(sessions, entry.Name())
			}
		}
	}

	return sessions, nil
}

// GetLatestSession returns the most recently saved session ID
func GetLatestSession(sessionsDir string) (string, error) {
	sessions, err := ListSavedSessions(sessionsDir)
	if err != nil {
		return "", err
	}

	if len(sessions) == 0 {
		return "", fmt.Errorf("no saved sessions found")
	}

	// Find the most recent session by reading metadata
	var latestSession string
	var latestTime time.Time

	for _, sessionID := range sessions {
		metadataPath := filepath.Join(sessionsDir, sessionID, "metadata.json")
		metadata, err := LoadSessionMetadata(metadataPath)
		if err != nil {
			continue // Skip sessions without valid metadata
		}

		savedTime, err := time.Parse(time.RFC3339, metadata.SavedAt)
		if err != nil {
			continue
		}

		if latestSession == "" || savedTime.After(latestTime) {
			latestSession = sessionID
			latestTime = savedTime
		}
	}

	if latestSession == "" {
		return "", fmt.Errorf("no valid sessions found")
	}

	return latestSession, nil
}

// GetLatestSessionForWorkspace returns the most recent session ID for a specific workspace
func GetLatestSessionForWorkspace(sessionsDir, workspacePath string) (string, error) {
	sessions, err := ListSavedSessions(sessionsDir)
	if err != nil {
		return "", err
	}

	if len(sessions) == 0 {
		return "", fmt.Errorf("no saved sessions found")
	}

	// Get the workspace hash to match against
	workspaceHash := WorkspaceHash(workspacePath)

	// Find the most recent session for this workspace
	var latestSession string
	var latestTime time.Time

	for _, sessionID := range sessions {
		metadataPath := filepath.Join(sessionsDir, sessionID, "metadata.json")
		metadata, err := LoadSessionMetadata(metadataPath)
		if err != nil {
			continue // Skip sessions without valid metadata
		}

		// Extract workspace hash from container name (format: claude-<hash>-<slot>)
		sessionHash, _, err := ParseContainerName(metadata.ContainerName)
		if err != nil {
			continue
		}

		// Only consider sessions from the same workspace
		if sessionHash != workspaceHash {
			continue
		}

		savedTime, err := time.Parse(time.RFC3339, metadata.SavedAt)
		if err != nil {
			continue
		}

		if latestSession == "" || savedTime.After(latestTime) {
			latestSession = sessionID
			latestTime = savedTime
		}
	}

	if latestSession == "" {
		return "", fmt.Errorf("no saved sessions found for workspace %s", workspacePath)
	}

	return latestSession, nil
}

// LoadSessionMetadata loads session metadata from a JSON file
func LoadSessionMetadata(path string) (*SessionMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var metadata SessionMetadata
	// Simple JSON parsing
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "\"session_id\"") {
			metadata.SessionID = extractJSONValue(line)
		} else if strings.Contains(line, "\"container_name\"") {
			metadata.ContainerName = extractJSONValue(line)
		} else if strings.Contains(line, "\"privileged\"") {
			metadata.Privileged = strings.Contains(line, "true")
		} else if strings.Contains(line, "\"persistent\"") {
			metadata.Persistent = strings.Contains(line, "true")
		} else if strings.Contains(line, "\"workspace\"") {
			metadata.Workspace = extractJSONValue(line)
		} else if strings.Contains(line, "\"saved_at\"") {
			metadata.SavedAt = extractJSONValue(line)
		}
	}

	if metadata.SessionID == "" {
		return nil, fmt.Errorf("invalid metadata: missing session_id")
	}

	return &metadata, nil
}

// extractJSONValue extracts the value from a JSON line like `"key": "value",`
func extractJSONValue(line string) string {
	// Find the value between quotes after the colon
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return ""
	}

	value := strings.TrimSpace(parts[1])
	value = strings.Trim(value, `",`)
	return value
}
