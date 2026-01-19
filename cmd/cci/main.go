package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/thomas/claude-code-isolated/internal/cli"
)

func main() {
	// Detect if called as 'cci' or 'claude-code-isolated'
	progName := filepath.Base(os.Args[0])
	isCci := progName == "cci"

	if err := cli.Execute(isCci); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
