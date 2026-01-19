package cli

import (
	"fmt"

	"github.com/thomas/claude-code-isolated/internal/config"
	"github.com/spf13/cobra"
)

// Version is the current version of cci (injected via ldflags at build time)
var Version = "dev"

var (
	// Global flags
	workspace       string
	slot            int
	imageName       string
	persistent      bool
	resume          string
	continueSession string // Alias for resume
	profile         string
	envVars         []string
	storage         string
	networkMode     string

	// Loaded config
	cfg *config.Config
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "cci",
	Short: "Claude Code Isolated - Run Claude Code in isolated Incus containers",
	Long: `claude-code-isolated (cci) is a CLI tool for running Claude Code in isolated Incus containers
with session persistence, workspace isolation, and multi-slot support.

Examples:
  cci                          # Start Claude Code session (same as 'cci shell')
  cci shell --slot 2           # Use specific slot
  cci run "npm test"           # Run command in container
  cci build                    # Build cci image
  cci images                   # List available images
  cci list                     # List active sessions
`,
	Version: Version,
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
		if !cmd.Flags().Changed("persistent") {
			persistent = cfg.Defaults.Persistent
		}

		return nil
	},
}

// Execute runs the root command
func Execute(isCci bool) error {
	if !isCci {
		rootCmd.Use = "claude-code-isolated"
	}
	return rootCmd.Execute()
}

func init() {
	// Global flags available to all commands
	rootCmd.PersistentFlags().StringVarP(&workspace, "workspace", "w", ".", "Workspace directory to mount")
	rootCmd.PersistentFlags().IntVar(&slot, "slot", 0, "Slot number for parallel sessions (0 = auto-allocate)")
	rootCmd.PersistentFlags().StringVar(&imageName, "image", "", "Custom image to use (default: cci)")
	rootCmd.PersistentFlags().BoolVar(&persistent, "persistent", false, "Reuse container across sessions")
	rootCmd.PersistentFlags().StringVar(&resume, "resume", "", "Resume from session ID (omit value to auto-detect)")
	rootCmd.PersistentFlags().Lookup("resume").NoOptDefVal = "auto"
	rootCmd.PersistentFlags().StringVar(&continueSession, "continue", "", "Alias for --resume")
	rootCmd.PersistentFlags().Lookup("continue").NoOptDefVal = "auto"
	rootCmd.PersistentFlags().StringVar(&profile, "profile", "", "Use named profile")
	rootCmd.PersistentFlags().StringSliceVarP(&envVars, "env", "e", []string{}, "Environment variables (KEY=VALUE)")
	rootCmd.PersistentFlags().StringVar(&storage, "storage", "", "Mount persistent storage")
	rootCmd.PersistentFlags().StringVar(&networkMode, "network", "", "Network mode: restricted (default), open")

	// Add subcommands
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(shellCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(imagesCmd)    // Legacy: cci images
	rootCmd.AddCommand(imageCmd)     // New: cci image <subcommand>
	rootCmd.AddCommand(containerCmd) // New: cci container <subcommand>
	rootCmd.AddCommand(fileCmd)      // New: cci file <subcommand>
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(killCmd)
	rootCmd.AddCommand(tmuxCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("claude-code-isolated (cci) v%s\n", Version)
		fmt.Println("https://github.com/thomas/claude-code-isolated")
	},
}
