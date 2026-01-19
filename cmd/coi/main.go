package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/thomas/claude-code-isolated/internal/cli"
)

func main() {
	// Detect if called as 'coi' or 'claude-code-isolated'
	progName := filepath.Base(os.Args[0])
	isCoi := progName == "coi"

	if err := cli.Execute(isCoi); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
