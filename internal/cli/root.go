package cli

import (
	"fmt"

	"github.com/mensfeld/claude-on-incus/internal/config"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	workspace         string
	slot              int
	imageName         string
	privileged        bool
	persistent        bool
	resume            string
	continueSession   string // Alias for resume
	profile           string
	envVars           []string
	storage           string
	mountClaudeConfig bool

	// Loaded config
	cfg *config.Config
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "coi",
	Short: "Claude on Incus - Run Claude Code in isolated Incus containers",
	Long: `claude-on-incus (coi) is a CLI tool for running Claude Code in Incus containers
with session persistence, workspace isolation, and multi-slot support.

Examples:
  coi                          # Start interactive Claude session (same as 'coi shell')
  coi shell --slot 2           # Use specific slot
  coi shell --privileged       # Enable Git/SSH access
  coi run "npm test"           # Run command in container
  coi build sandbox            # Build sandbox image
  coi images                   # List available images
  coi list                     # List active sessions
`,
	Version: "0.1.0",
	// When called without subcommand, run shell command
	RunE: func(cmd *cobra.Command, args []string) error {
		// Execute shell command with the same args
		return shellCmd.RunE(cmd, args)
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Load config
		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Apply profile if specified
		if profile != "" {
			if !cfg.ApplyProfile(profile) {
				return fmt.Errorf("profile '%s' not found", profile)
			}
		}

		// Apply config defaults to flags that weren't explicitly set
		if !cmd.Flags().Changed("privileged") {
			privileged = cfg.Defaults.Privileged
		}
		if !cmd.Flags().Changed("persistent") {
			persistent = cfg.Defaults.Persistent
		}
		if !cmd.Flags().Changed("mount-claude-config") {
			mountClaudeConfig = cfg.Defaults.MountClaudeConfig
		}

		return nil
	},
}

// Execute runs the root command
func Execute(isCoi bool) error {
	if !isCoi {
		rootCmd.Use = "claude-on-incus"
	}
	return rootCmd.Execute()
}

func init() {
	// Global flags available to all commands
	rootCmd.PersistentFlags().StringVarP(&workspace, "workspace", "w", ".", "Workspace directory to mount")
	rootCmd.PersistentFlags().IntVar(&slot, "slot", 0, "Slot number for parallel sessions (0 = auto-allocate)")
	rootCmd.PersistentFlags().StringVar(&imageName, "image", "", "Custom image to use (default: coi-sandbox or coi-privileged)")
	rootCmd.PersistentFlags().BoolVar(&privileged, "privileged", false, "Use privileged image (Git/SSH/sudo)")
	rootCmd.PersistentFlags().BoolVar(&persistent, "persistent", false, "Reuse container across sessions")
	rootCmd.PersistentFlags().StringVar(&resume, "resume", "", "Resume from session ID (omit value to auto-detect)")
	rootCmd.PersistentFlags().Lookup("resume").NoOptDefVal = "auto"
	rootCmd.PersistentFlags().StringVar(&continueSession, "continue", "", "Alias for --resume")
	rootCmd.PersistentFlags().Lookup("continue").NoOptDefVal = "auto"
	rootCmd.PersistentFlags().StringVar(&profile, "profile", "", "Use named profile")
	rootCmd.PersistentFlags().StringSliceVarP(&envVars, "env", "e", []string{}, "Environment variables (KEY=VALUE)")
	rootCmd.PersistentFlags().StringVar(&storage, "storage", "", "Mount persistent storage")
	rootCmd.PersistentFlags().BoolVar(&mountClaudeConfig, "mount-claude-config", true, "Mount/copy host ~/.claude directory and .claude.json")

	// Add subcommands
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(shellCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(imagesCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(killCmd)
	rootCmd.AddCommand(tmuxCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("claude-on-incus (coi) v0.1.0")
		fmt.Println("https://github.com/mensfeld/claude-on-incus")
	},
}
