package container

import (
	"fmt"
	"strings"
)

// HardeningPolicy selects how much kernel-facing surface a container gets.
// The zero value is maximally hardened; DefaultHardeningPolicy() preserves the
// historical behavior (Docker support on).
type HardeningPolicy struct {
	// Docker configures the container for Docker/nested containers:
	// security.nesting, mknod/setxattr syscall interception, and the
	// unprivileged low-port sysctl. Off, those four knobs are unset,
	// shrinking the shared-kernel attack surface.
	Docker bool
	// ReduceKernelSurface additionally denies the syscall families behind
	// most recent kernel escape chains via security.syscalls.deny. It wins
	// over Docker: nesting is part of the surface being reduced, and dockerd
	// itself needs bpf.
	ReduceKernelSurface bool
}

// DefaultHardeningPolicy returns the pre-flag behavior: Docker support on,
// no syscall denies.
func DefaultHardeningPolicy() HardeningPolicy { return HardeningPolicy{Docker: true} }

// DockerEnabled resolves the docker/hardening conflict: ReduceKernelSurface
// wins over Docker (nesting is part of the surface being reduced).
func (p HardeningPolicy) DockerEnabled() bool { return p.Docker && !p.ReduceKernelSurface }

// KernelSurfaceDenySyscalls is the security.syscalls.deny value applied by
// ReduceKernelSurface: io_uring, bpf, userfaultfd, and the kernel keyring —
// the syscall families behind most recent container-escape/LPE chains, none
// required by a typical agent workflow. Denied syscalls fail with EPERM;
// libuv (Node >= 20.3) probes io_uring at startup and silently falls back to
// its thread pool, so Node/npm keep working. Incus >= 6.1 (COI's hard floor)
// always supports the key.
const KernelSurfaceDenySyscalls = "io_uring_setup io_uring_enter io_uring_register bpf userfaultfd keyctl add_key request_key"

// kernelSurfaceKeys are the instance config keys ApplyKernelSurfacePolicy owns.
// Every launch/reconcile writes ALL of them (a wanted key to its value, an
// unwanted key to "") so re-applying a changed policy to an existing stopped
// container converges instead of leaking the previous policy's settings. An
// empty value removes the key (Incus treats `config set key=` as unset), so
// the whole policy applies in ONE `incus config set` — atomic and fail-closed.
var kernelSurfaceKeys = []string{
	"security.nesting",
	"security.syscalls.intercept.mknod",
	"security.syscalls.intercept.setxattr",
	"linux.sysctl.net.ipv4.ip_unprivileged_port_start",
	"security.syscalls.deny",
}

// hardeningConfigArgs returns the `key=value` pairs realizing the policy: the
// four Docker-support keys set to their values (or "" to unset) per
// DockerEnabled, and security.syscalls.deny set to the deny list (or "") per
// ReduceKernelSurface. Every key in kernelSurfaceKeys appears exactly once so
// the result fully specifies the policy.
func hardeningConfigArgs(p HardeningPolicy) []string {
	docker := "" // "" unsets the key
	if p.DockerEnabled() {
		docker = "true"
	}
	lowPort := ""
	if p.DockerEnabled() {
		lowPort = "0"
	}
	deny := ""
	if p.ReduceKernelSurface {
		deny = KernelSurfaceDenySyscalls
	}
	return []string{
		"security.nesting=" + docker,
		"security.syscalls.intercept.mknod=" + docker,
		"security.syscalls.intercept.setxattr=" + docker,
		"linux.sysctl.net.ipv4.ip_unprivileged_port_start=" + lowPort,
		"security.syscalls.deny=" + deny,
	}
}

// ApplyKernelSurfacePolicy applies (or re-applies) the hardening policy to a
// container in a single `incus config set`. Like the Docker flags it replaces,
// this must run before the container's first boot so the kernel loads the
// correct seccomp profile — setting security.nesting or security.syscalls.deny
// on a running container is rejected or racy. It is idempotent: call it again
// on a STOPPED persistent container to converge it to a changed policy. Unlike
// the previous per-key implementation it is FAIL-CLOSED — a failed write
// returns an error (the caller must decide, e.g. abort the launch) rather than
// silently leaving a hardened config partly applied.
func ApplyKernelSurfacePolicy(containerName string, p HardeningPolicy) error {
	args := append([]string{"config", "set", containerName}, hardeningConfigArgs(p)...)
	return IncusExec(args...)
}

// ReadKernelSurfaceConfig returns the EXPANDED (profile-inherited plus
// instance-local) values of the keys ApplyKernelSurfacePolicy owns. Expanded
// config is what actually takes effect, so a mismatch check against it never
// spuriously fires for a value a container inherits from an attached Incus
// profile. A key absent from the config maps to "" in the result. Uses `incus
// config get --expanded`, which — unlike `incus query` — accepts the --project
// flag COI injects into every invocation.
func ReadKernelSurfaceConfig(containerName string) (map[string]string, error) {
	result := make(map[string]string, len(kernelSurfaceKeys))
	for _, key := range kernelSurfaceKeys {
		v, err := IncusOutput("config", "get", "--expanded", containerName, key)
		if err != nil {
			return nil, fmt.Errorf("read %s from %s: %w", key, containerName, err)
		}
		result[key] = v
	}
	return result, nil
}

// ReconcileKernelSurfacePolicy converges an existing STOPPED container to the
// policy, but only when its current (expanded) config does not already match —
// so a reuse that changes nothing performs no writes and, crucially, cannot
// clobber a deny list or nesting value the container already carries. Returns
// whether a change was made. Fail-closed: a read or write failure is returned
// so the caller can abort rather than start a container whose kernel-surface
// config is unknown or half-applied.
func ReconcileKernelSurfacePolicy(containerName string, p HardeningPolicy) (changed bool, err error) {
	current, err := ReadKernelSurfaceConfig(containerName)
	if err != nil {
		return false, err
	}
	if KernelSurfaceMatches(current, p) {
		return false, nil
	}
	return true, ApplyKernelSurfacePolicy(containerName, p)
}

// KernelSurfaceMatches reports whether current (as returned by
// ReadKernelSurfaceConfig) already realizes the policy, so a reconcile can
// skip the write — avoiding both wasted `config set` calls and clobbering a
// container whose config already matches. A missing key reads as "".
func KernelSurfaceMatches(current map[string]string, p HardeningPolicy) bool {
	for _, kv := range hardeningConfigArgs(p) {
		key, want, _ := strings.Cut(kv, "=")
		if strings.TrimSpace(current[key]) != want {
			return false
		}
	}
	return true
}
