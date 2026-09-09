package session

import (
	"testing"

	"github.com/mensfeld/code-on-incus/internal/container"
)

// The SetupOptions -> HardeningPolicy mapping must resolve the docker vs
// reduce_kernel_surface conflict the same way config.EffectiveDockerEnabled
// does: hardening wins.
func TestHardeningPolicyFrom(t *testing.T) {
	tests := []struct {
		docker, reduce bool
		want           container.HardeningPolicy
	}{
		{true, false, container.HardeningPolicy{Docker: true}},
		{false, false, container.HardeningPolicy{}},
		{true, true, container.HardeningPolicy{Docker: false, ReduceKernelSurface: true}},
		{false, true, container.HardeningPolicy{Docker: false, ReduceKernelSurface: true}},
	}
	for _, tt := range tests {
		opts := &SetupOptions{DockerSupport: tt.docker, ReduceKernelSurface: tt.reduce}
		if got := hardeningPolicyFrom(opts); got != tt.want {
			t.Errorf("docker=%v reduce=%v: got %+v, want %+v", tt.docker, tt.reduce, got, tt.want)
		}
	}
}
