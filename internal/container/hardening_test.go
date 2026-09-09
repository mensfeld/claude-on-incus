package container

import (
	"strings"
	"testing"
)

// opsByKey indexes the operation list for assertion convenience and asserts no
// key appears twice (set+unset of the same key would be order-dependent).
func opsByKey(t *testing.T, ops []configOp) map[string]configOp {
	t.Helper()
	m := make(map[string]configOp, len(ops))
	for _, op := range ops {
		if _, dup := m[op.key]; dup {
			t.Fatalf("key %q appears twice in ops", op.key)
		}
		m[op.key] = op
	}
	return m
}

// Every docker-support key plus the deny list must be covered by every policy
// (set OR unset) so a policy flip re-applied to a stopped persistent container
// converges — the same create/strip coupling idea as security devices (#610).
func TestHardeningOps_EveryKeyAlwaysReconciled(t *testing.T) {
	wantKeys := []string{
		"security.nesting",
		"security.syscalls.intercept.mknod",
		"security.syscalls.intercept.setxattr",
		"linux.sysctl.net.ipv4.ip_unprivileged_port_start",
		"security.syscalls.deny",
	}
	for _, p := range []HardeningPolicy{
		{Docker: true},
		{Docker: false},
		{Docker: false, ReduceKernelSurface: true},
		{Docker: true, ReduceKernelSurface: true},
	} {
		m := opsByKey(t, hardeningOps(p))
		for _, k := range wantKeys {
			if _, ok := m[k]; !ok {
				t.Errorf("policy %+v: key %q not covered", p, k)
			}
		}
		if len(m) != len(wantKeys) {
			t.Errorf("policy %+v: unexpected extra keys in ops (%d != %d)", p, len(m), len(wantKeys))
		}
	}
}

func TestHardeningOps_DefaultPolicy(t *testing.T) {
	m := opsByKey(t, hardeningOps(DefaultHardeningPolicy()))
	for k, want := range map[string]string{
		"security.nesting":                                 "true",
		"security.syscalls.intercept.mknod":                "true",
		"security.syscalls.intercept.setxattr":             "true",
		"linux.sysctl.net.ipv4.ip_unprivileged_port_start": "0",
	} {
		if op := m[k]; !op.set || op.value != want {
			t.Errorf("default policy: %s should be set to %q, got %+v", k, want, op)
		}
	}
	if m["security.syscalls.deny"].set {
		t.Error("default policy must unset security.syscalls.deny")
	}
}

func TestHardeningOps_DockerOff(t *testing.T) {
	m := opsByKey(t, hardeningOps(HardeningPolicy{Docker: false}))
	for _, k := range []string{
		"security.nesting",
		"security.syscalls.intercept.mknod",
		"security.syscalls.intercept.setxattr",
		"linux.sysctl.net.ipv4.ip_unprivileged_port_start",
		"security.syscalls.deny",
	} {
		if m[k].set {
			t.Errorf("docker-off policy must unset %s", k)
		}
	}
}

func TestHardeningOps_ReduceKernelSurface(t *testing.T) {
	// ReduceKernelSurface wins even when Docker is (mis)set alongside it.
	m := opsByKey(t, hardeningOps(HardeningPolicy{Docker: true, ReduceKernelSurface: true}))
	if m["security.nesting"].set {
		t.Error("reduce_kernel_surface must unset security.nesting even with Docker=true")
	}
	deny := m["security.syscalls.deny"]
	if !deny.set || deny.value != KernelSurfaceDenySyscalls {
		t.Errorf("reduce_kernel_surface must set the deny list, got %+v", deny)
	}
}

// The deny list itself: each syscall exactly once, no accidental edits.
func TestKernelSurfaceDenySyscalls(t *testing.T) {
	want := []string{
		"io_uring_setup", "io_uring_enter", "io_uring_register",
		"bpf", "userfaultfd", "keyctl", "add_key", "request_key",
	}
	got := strings.Fields(KernelSurfaceDenySyscalls)
	seen := make(map[string]bool)
	for _, s := range got {
		if seen[s] {
			t.Errorf("syscall %q listed twice", s)
		}
		seen[s] = true
	}
	for _, s := range want {
		if !seen[s] {
			t.Errorf("syscall %q missing from deny list", s)
		}
	}
	if len(got) != len(want) {
		t.Errorf("deny list has %d entries, want %d", len(got), len(want))
	}
}
