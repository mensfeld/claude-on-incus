package session

import (
	"github.com/mensfeld/code-on-incus/internal/container"
)

// hardeningPolicyFrom builds the container-level kernel-surface policy from
// SetupOptions. It passes the raw flags through; the docker-vs-hardening
// precedence ("reduce_kernel_surface wins") is resolved in exactly one place,
// container.HardeningPolicy.DockerEnabled, so callers never re-encode the rule.
func hardeningPolicyFrom(opts *SetupOptions) container.HardeningPolicy {
	return container.HardeningPolicy{
		Docker:              opts.DockerSupport,
		ReduceKernelSurface: opts.ReduceKernelSurface,
	}
}

// warnHardeningMismatch compares the desired kernel-surface policy against a
// RUNNING container's EXPANDED config (instance + inherited profile values, in
// one query) and logs how to apply a change. Running containers can't be
// reconciled (security.nesting changes are rejected, seccomp changes are racy)
// — that happens on the next stopped->start cycle in restartStoppedContainer.
// Reading expanded config avoids a spurious warning for a value the container
// legitimately inherits from an attached Incus profile.
func warnHardeningMismatch(containerName string, p container.HardeningPolicy, log func(string)) {
	current, err := container.ReadKernelSurfaceConfig(containerName)
	if err != nil {
		return // best-effort: a probe failure is not worth a warning of its own
	}
	if !container.KernelSurfaceMatches(current, p) {
		log("Warning: this running container was created with different docker/kernel-hardening settings than the current config; restart it (coi shutdown, then relaunch) to apply them")
	}
}
