package terminal

import "testing"

func TestSanitizeTerm(t *testing.T) {
	tests := []struct {
		name string
		term string
		want string
	}{
		// Empty TERM
		{
			name: "empty term",
			term: "",
			want: "xterm-256color",
		},
		// Modern terminals that need mapping
		{
			name: "ghostty terminal",
			term: "xterm-ghostty",
			want: "xterm-256color",
		},
		{
			name: "wezterm",
			term: "wezterm",
			want: "xterm-256color",
		},
		{
			name: "alacritty",
			term: "alacritty",
			want: "xterm-256color",
		},
		{
			name: "kitty",
			term: "kitty",
			want: "xterm-256color",
		},
		{
			name: "tmux-256color",
			term: "tmux-256color",
			want: "xterm-256color",
		},
		{
			name: "screen-256color",
			term: "screen-256color",
			want: "xterm-256color",
		},
		// Standard terminals that should pass through
		{
			name: "xterm",
			term: "xterm",
			want: "xterm",
		},
		{
			name: "xterm-256color",
			term: "xterm-256color",
			want: "xterm-256color",
		},
		{
			name: "vt100",
			term: "vt100",
			want: "vt100",
		},
		{
			name: "screen",
			term: "screen",
			want: "screen",
		},
		{
			name: "linux",
			term: "linux",
			want: "linux",
		},
		// Edge cases
		{
			name: "unknown custom term",
			term: "my-custom-term",
			want: "my-custom-term", // Pass through unknown terms
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeTerm(tt.term)
			if got != tt.want {
				t.Errorf("SanitizeTerm(%q) = %q, want %q", tt.term, got, tt.want)
			}
		})
	}
}

func TestSanitizeTermDeterministic(t *testing.T) {
	// Test that sanitization is deterministic
	term := "xterm-ghostty"
	result1 := SanitizeTerm(term)
	result2 := SanitizeTerm(term)

	if result1 != result2 {
		t.Errorf("SanitizeTerm() not deterministic: %s != %s", result1, result2)
	}
}
