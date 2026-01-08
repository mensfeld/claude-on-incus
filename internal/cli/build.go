package cli

import (
	"fmt"

	"github.com/mensfeld/claude-on-incus/internal/container"
	"github.com/mensfeld/claude-on-incus/internal/image"
	"github.com/spf13/cobra"
)

var (
	buildForce bool
)

var buildCmd = &cobra.Command{
	Use:   "build [sandbox|privileged]",
	Short: "Build Incus images for Claude sessions",
	Long: `Build opinionated Incus images for running Claude Code.

Available images:
  sandbox     - Sandbox image (Docker + build tools + Claude CLI)
  privileged  - Privileged image (sandbox + GitHub CLI + SSH)

Examples:
  coi build sandbox
  coi build privileged
  coi build sandbox --force
`,
	Args: cobra.ExactArgs(1),
	RunE: buildCommand,
}

func init() {
	buildCmd.Flags().BoolVar(&buildForce, "force", false, "Force rebuild even if image exists")
}

func buildCommand(cmd *cobra.Command, args []string) error {
	imageType := args[0]

	// Validate image type
	if imageType != "sandbox" && imageType != "privileged" {
		return fmt.Errorf("invalid image type: %s (must be 'sandbox' or 'privileged')", imageType)
	}

	// Check if Incus is available
	if !container.Available() {
		return fmt.Errorf("incus is not available - please install Incus and ensure you're in the incus-admin group")
	}

	// Configure build options
	var opts image.BuildOptions
	opts.Force = buildForce
	opts.ImageType = imageType
	opts.BaseImage = image.BaseImage

	switch imageType {
	case "sandbox":
		opts.AliasName = image.SandboxAlias
		opts.Description = "coi sandbox image (Docker + build tools + sudo)"
	case "privileged":
		opts.AliasName = image.PrivilegedAlias
		opts.Description = "coi privileged image (sandbox + GitHub CLI + SSH)"
	}

	// Logger function
	opts.Logger = func(msg string) {
		fmt.Println(msg)
	}

	// Build the image
	fmt.Printf("Building %s image...\n", imageType)
	builder := image.NewBuilder(opts)
	result := builder.Build()

	if result.Error != nil {
		return fmt.Errorf("build failed: %w", result.Error)
	}

	if result.Skipped {
		fmt.Printf("\nImage already exists. Use --force to rebuild.\n")
		return nil
	}

	fmt.Printf("\n✓ Image '%s' built successfully!\n", opts.AliasName)
	fmt.Printf("  Version: %s\n", result.VersionAlias)
	fmt.Printf("  Fingerprint: %s\n", result.Fingerprint)
	return nil
}
