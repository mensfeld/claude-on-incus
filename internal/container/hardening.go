package container

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

// KernelSurfaceDenySyscalls is the security.syscalls.deny value applied by
// ReduceKernelSurface: io_uring, bpf, userfaultfd, and the kernel keyring —
// the syscall families behind most recent container-escape/LPE chains, none
// required by a typical agent workflow. Denied syscalls fail with EPERM;
// libuv (Node >= 20.3) probes io_uring at startup and silently falls back to
// its thread pool, so Node/npm keep working. Incus >= 6.1 (COI's hard floor)
// always supports the key.
const KernelSurfaceDenySyscalls = "io_uring_setup io_uring_enter io_uring_register bpf userfaultfd keyctl add_key request_key"

// configOp is one incus `config set`/`config unset` operation.
type configOp struct {
	set   bool
	key   string
	value string // set only
}

// dockerSupportKeys are the instance config keys that make up Docker/nested
// container support. Kept together so hardeningOps sets and unsets the exact
// same list and a policy flip converges on persistent-container reuse.
var dockerSupportKeys = []configOp{
	// Enable container nesting for Docker support.
	{set: true, key: "security.nesting", value: "true"},
	// Syscall interception: safe device node creation / filesystem attributes.
	{set: true, key: "security.syscalls.intercept.mknod", value: "true"},
	{set: true, key: "security.syscalls.intercept.setxattr", value: "true"},
	// Allow unprivileged port binding and prevent runc sysctl permission
	// errors: newer runc (1.3.x) writes this sysctl via a detached procfs
	// mount, which AppArmor blocks in nested containers (#187).
	{set: true, key: "linux.sysctl.net.ipv4.ip_unprivileged_port_start", value: "0"},
}

// hardeningOps returns the ordered config operations realizing the policy.
// Every key the policy does not want is explicitly unset (not merely left
// alone) so re-applying a changed policy to an existing (stopped) container
// converges instead of leaking the previous session's settings.
func hardeningOps(p HardeningPolicy) []configOp {
	docker := p.Docker && !p.ReduceKernelSurface
	ops := make([]configOp, 0, len(dockerSupportKeys)+1)
	for _, op := range dockerSupportKeys {
		op.set = docker
		ops = append(ops, op)
	}
	if p.ReduceKernelSurface {
		ops = append(ops, configOp{set: true, key: "security.syscalls.deny", value: KernelSurfaceDenySyscalls})
	} else {
		ops = append(ops, configOp{set: false, key: "security.syscalls.deny"})
	}
	return ops
}

// ApplyKernelSurfacePolicy applies (or re-applies) the hardening policy to a
// container. Like the Docker flags it replaces, this must run before the
// container's first boot so the kernel loads the correct seccomp profile —
// setting security.nesting or security.syscalls.deny on a running container
// is rejected or racy. It is idempotent: call it again on a STOPPED persistent
// container to converge it to a changed policy. Unsets of absent keys are
// tolerated (IncusExecQuiet).
func ApplyKernelSurfacePolicy(containerName string, p HardeningPolicy) error {
	for _, op := range hardeningOps(p) {
		if op.set {
			if err := IncusExec("config", "set", containerName, op.key+"="+op.value); err != nil {
				return err
			}
		} else {
			_ = IncusExecQuiet("config", "unset", containerName, op.key)
		}
	}
	return nil
}
