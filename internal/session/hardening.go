package session

import (
	"strings"

	"github.com/mensfeld/code-on-incus/internal/container"
)

// hardeningPolicyFrom resolves SetupOptions into the container-level
// kernel-surface policy. reduce_kernel_surface wins over docker (nesting is
// part of the surface being reduced); the CLI layer warns about an explicit
// conflict, this just resolves it the same way config.EffectiveDockerEnabled
// does.
func hardeningPolicyFrom(opts *SetupOptions) container.HardeningPolicy {
	return container.HardeningPolicy{
		Docker:              opts.DockerSupport && !opts.ReduceKernelSurface,
		ReduceKernelSurface: opts.ReduceKernelSurface,
	}
}

// warnHardeningMismatch compares the desired kernel-surface policy against a
// RUNNING container's actual config and logs how to apply a change. Running
// containers can't be reconciled (security.nesting changes are rejected,
// seccomp changes are racy) — that happens on the next stopped->start cycle
// in restartStoppedContainer.
func warnHardeningMismatch(containerName string, p container.HardeningPolicy, log func(string)) {
	nesting, err1 := container.IncusOutput("config", "get", containerName, "security.nesting")
	deny, err2 := container.IncusOutput("config", "get", containerName, "security.syscalls.deny")
	if err1 != nil || err2 != nil {
		return // best-effort: a probe failure is not worth a warning of its own
	}
	nestingOn := strings.EqualFold(strings.TrimSpace(nesting), "true")
	denySet := strings.TrimSpace(deny) != ""
	if nestingOn != p.Docker || denySet != p.ReduceKernelSurface {
		log("Warning: this running container was created with different docker/kernel-hardening settings than the current config; restart it (coi shutdown, then relaunch) to apply them")
	}
}
