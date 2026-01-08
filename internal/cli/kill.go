package cli

import (
	"fmt"
	"os"

	"github.com/mensfeld/claude-on-incus/internal/container"
	"github.com/spf13/cobra"
)

var (
	killForce bool
	killAll   bool
)

var killCmd = &cobra.Command{
	Use:   "kill [container-name...]",
	Short: "Stop and delete containers",
	Long: `Stop and delete one or more containers by name.

This is useful for cleaning up dangling or stuck containers.
Use 'coi list' to see active containers.

Examples:
  coi kill claude-abc12345-1           # Kill specific container
  coi kill claude-abc12345-1 claude-xyz78901-2  # Kill multiple containers
  coi kill --all                       # Kill all containers (with confirmation)
  coi kill --all --force               # Kill all without confirmation
`,
	RunE: killCommand,
}

func init() {
	killCmd.Flags().BoolVar(&killForce, "force", false, "Skip confirmation prompts")
	killCmd.Flags().BoolVar(&killAll, "all", false, "Kill all containers")
}

func killCommand(cmd *cobra.Command, args []string) error {
	// Get container names to kill
	var containerNames []string

	if killAll {
		// Get all containers
		containers, err := listActiveContainers()
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}

		if len(containers) == 0 {
			fmt.Println("No containers to kill")
			return nil
		}

		for _, c := range containers {
			containerNames = append(containerNames, c.Name)
		}

		// Show what will be killed
		fmt.Printf("Found %d container(s):\n", len(containerNames))
		for _, name := range containerNames {
			fmt.Printf("  - %s\n", name)
		}

		// Confirm unless --force
		if !killForce {
			fmt.Print("\nKill all these containers? [y/N]: ")
			var response string
			_, _ = fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				fmt.Println("Cancelled.")
				return nil
			}
		}
	} else {
		// Use containers from args
		if len(args) == 0 {
			return fmt.Errorf("no container names provided - use 'coi list' to see active containers")
		}
		containerNames = args

		// Confirm unless --force
		if !killForce && len(containerNames) > 1 {
			fmt.Printf("Kill %d container(s)? [y/N]: ", len(containerNames))
			var response string
			_, _ = fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				fmt.Println("Cancelled.")
				return nil
			}
		}
	}

	// Kill each container
	killed := 0
	for _, name := range containerNames {
		fmt.Printf("Killing container %s...\n", name)
		mgr := container.NewManager(name)

		// Stop container
		if err := mgr.Stop(true); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: Failed to stop %s: %v\n", name, err)
		}

		// Delete container
		if err := mgr.Delete(true); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: Failed to delete %s: %v\n", name, err)
		} else {
			killed++
			fmt.Printf("  ✓ Killed %s\n", name)
		}
	}

	if killed > 0 {
		fmt.Printf("\nKilled %d container(s)\n", killed)
	} else {
		fmt.Println("\nNo containers were killed")
	}

	return nil
}
