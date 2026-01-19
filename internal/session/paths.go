package session

import (
	"path/filepath"

	"github.com/thomas/claude-code-isolated/internal/tool"
)

// GetSessionsDir returns the sessions directory path for a given tool.
// For example: ~/.cci/sessions-claude, ~/.cci/sessions-aider, etc.
func GetSessionsDir(baseDir string, t tool.Tool) string {
	return filepath.Join(baseDir, t.SessionsDirName())
}
