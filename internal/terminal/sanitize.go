package terminal

import "strings"

// SanitizeTerm returns a TERM value compatible with most container environments.
// Modern terminals like Ghostty, WezTerm, etc. may not have terminfo entries
// in the container, so we map them to standard equivalents that preserve features.
func SanitizeTerm(term string) string {
	if term == "" {
		return "xterm-256color"
	}

	// Map modern/exotic terminals to standard equivalents
	// Preserve color support when possible (256color vs basic)
	switch {
	case strings.HasPrefix(term, "xterm-ghostty"):
		return "xterm-256color" // Ghostty supports 256 colors
	case strings.HasPrefix(term, "wezterm"):
		return "xterm-256color"
	case strings.HasPrefix(term, "alacritty"):
		return "xterm-256color"
	case strings.HasPrefix(term, "kitty"):
		return "xterm-256color"
	case strings.HasPrefix(term, "tmux-256color"):
		return "xterm-256color"
	case strings.HasPrefix(term, "screen-256color"):
		return "xterm-256color"
	default:
		// For standard terms (xterm, xterm-256color, vt100, etc.), pass through
		return term
	}
}
