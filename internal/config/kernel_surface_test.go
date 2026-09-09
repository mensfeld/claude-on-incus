package config

import (
	"testing"

	"github.com/BurntSushi/toml"
)

// Defaults: docker on, hardening off — the pre-flag behavior.
func TestKernelSurface_Defaults(t *testing.T) {
	cfg := GetDefaultConfig()
	if !cfg.Container.IsDockerEnabled() {
		t.Error("docker should default to enabled")
	}
	if cfg.Security.IsReduceKernelSurfaceEnabled() {
		t.Error("reduce_kernel_surface should default to disabled")
	}
	if !cfg.EffectiveDockerEnabled() {
		t.Error("effective docker should default to enabled")
	}
}

func TestKernelSurface_TOMLParse(t *testing.T) {
	const tomlSrc = `
[container]
docker = false

[security]
reduce_kernel_surface = true
`
	var cfg Config
	if _, err := toml.Decode(tomlSrc, &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.Container.IsDockerEnabled() {
		t.Error("docker=false not parsed")
	}
	if !cfg.Security.IsReduceKernelSurfaceEnabled() {
		t.Error("reduce_kernel_surface=true not parsed")
	}
}

// reduce_kernel_surface wins over an explicit docker=true.
func TestKernelSurface_HardeningWinsOverDocker(t *testing.T) {
	yes := true
	cfg := GetDefaultConfig()
	cfg.Container.Docker = &yes
	cfg.Security.ReduceKernelSurface = &yes
	if cfg.EffectiveDockerEnabled() {
		t.Error("reduce_kernel_surface=true must disable docker even when docker=true is explicit")
	}
}

// Later scopes override earlier ones via pointer-merge for both flags.
func TestKernelSurface_MergePrecedence(t *testing.T) {
	yes, no := true, false
	base := GetDefaultConfig()
	overlay := &Config{}
	overlay.Container.Docker = &no
	overlay.Security.ReduceKernelSurface = &yes
	base.Merge(overlay)
	if base.Container.IsDockerEnabled() {
		t.Error("overlay docker=false should win over unset base")
	}
	if !base.Security.IsReduceKernelSurfaceEnabled() {
		t.Error("overlay reduce_kernel_surface=true should win over unset base")
	}
	// And a further overlay flipping docker back on wins again.
	overlay2 := &Config{}
	overlay2.Container.Docker = &yes
	base.Merge(overlay2)
	if !base.Container.IsDockerEnabled() {
		t.Error("later overlay docker=true should win")
	}
}

// Untrusted repo config: docker=true (re-widening) is stripped, docker=false
// (tightening) survives, reduce_kernel_surface is stripped in both directions.
func TestSanitizeUntrustedConfig_KernelSurface(t *testing.T) {
	yes, no := true, false

	cfg := &Config{}
	cfg.Container.Docker = &yes
	cfg.Security.ReduceKernelSurface = &no
	sanitizeUntrustedConfig(cfg, "/ws/.coi/config.toml")
	if cfg.Container.Docker != nil {
		t.Error("untrusted docker=true must be stripped")
	}
	if cfg.Security.ReduceKernelSurface != nil {
		t.Error("untrusted reduce_kernel_surface=false must be stripped")
	}

	cfg = &Config{}
	cfg.Container.Docker = &no
	cfg.Security.ReduceKernelSurface = &yes
	sanitizeUntrustedConfig(cfg, "/ws/.coi/config.toml")
	if cfg.Container.Docker == nil || *cfg.Container.Docker {
		t.Error("untrusted docker=false (tightening) must survive")
	}
	if cfg.Security.ReduceKernelSurface != nil {
		t.Error("untrusted reduce_kernel_surface=true must be stripped (trusted scope only)")
	}
}
