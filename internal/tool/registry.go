package tool

import "fmt"

// registry maps tool names to their factory functions
var registry = map[string]func() Tool{
	"claude": NewClaude,
}

// Get returns a tool by name
func Get(name string) (Tool, error) {
	factory, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s (supported: claude)", name)
	}
	return factory(), nil
}

// GetDefault returns the default tool (Claude)
func GetDefault() Tool {
	return NewClaude()
}
