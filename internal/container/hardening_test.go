package container

import (
	"strings"
	"testing"
)

// argsMap parses hardeningConfigArgs("key=value"...) into a map, asserting each
// key appears exactly once (a duplicate would make the applied value
// order-dependent).
func argsMap(t *testing.T, p HardeningPolicy) map[string]string {
	t.Helper()
	m := make(map[string]string)
	for _, kv := range hardeningConfigArgs(p) {
		key, val, _ := strings.Cut(kv, "=")
		if _, dup := m[key]; dup {
			t.Fatalf("key %q appears twice in args", key)
		}
		m[key] = val
	}
	return m
}

// Every kernel-surface key must be specified by every policy (set to a value,
// or "" to unset) so re-applying a changed policy converges — the same
// create/reconcile coupling idea as security devices (#610).
func TestHardeningConfigArgs_EveryKeyAlwaysSpecified(t *testing.T) {
	for _, p := range []HardeningPolicy{
		{Docker: true},
		{Docker: false},
		{Docker: false, ReduceKernelSurface: true},
		{Docker: true, ReduceKernelSurface: true},
	} {
		m := argsMap(t, p)
		for _, k := range kernelSurfaceKeys {
			if _, ok := m[k]; !ok {
				t.Errorf("policy %+v: key %q not specified", p, k)
			}
		}
		if len(m) != len(kernelSurfaceKeys) {
			t.Errorf("policy %+v: unexpected extra keys (%d != %d)", p, len(m), len(kernelSurfaceKeys))
		}
	}
}

func TestHardeningConfigArgs_DefaultPolicy(t *testing.T) {
	m := argsMap(t, DefaultHardeningPolicy())
	for k, want := range map[string]string{
		"security.nesting":                                 "true",
		"security.syscalls.intercept.mknod":                "true",
		"security.syscalls.intercept.setxattr":             "true",
		"linux.sysctl.net.ipv4.ip_unprivileged_port_start": "0",
	} {
		if m[k] != want {
			t.Errorf("default policy: %s = %q, want %q", k, m[k], want)
		}
	}
	if m["security.syscalls.deny"] != "" {
		t.Errorf("default policy must leave security.syscalls.deny empty, got %q", m["security.syscalls.deny"])
	}
}

func TestHardeningConfigArgs_DockerOff(t *testing.T) {
	m := argsMap(t, HardeningPolicy{Docker: false})
	for _, k := range kernelSurfaceKeys {
		if m[k] != "" {
			t.Errorf("docker-off policy must leave %s empty (unset), got %q", k, m[k])
		}
	}
}

func TestHardeningConfigArgs_ReduceKernelSurface(t *testing.T) {
	// ReduceKernelSurface wins even when Docker is (mis)set alongside it.
	m := argsMap(t, HardeningPolicy{Docker: true, ReduceKernelSurface: true})
	if m["security.nesting"] != "" {
		t.Error("reduce_kernel_surface must leave security.nesting empty even with Docker=true")
	}
	if m["security.syscalls.deny"] != KernelSurfaceDenySyscalls {
		t.Errorf("reduce_kernel_surface must set the deny list, got %q", m["security.syscalls.deny"])
	}
}

func TestHardeningPolicy_DockerEnabled(t *testing.T) {
	cases := []struct {
		p    HardeningPolicy
		want bool
	}{
		{HardeningPolicy{Docker: true}, true},
		{HardeningPolicy{Docker: false}, false},
		{HardeningPolicy{Docker: true, ReduceKernelSurface: true}, false},
		{HardeningPolicy{Docker: false, ReduceKernelSurface: true}, false},
	}
	for _, c := range cases {
		if got := c.p.DockerEnabled(); got != c.want {
			t.Errorf("%+v.DockerEnabled() = %v, want %v", c.p, got, c.want)
		}
	}
}

// KernelSurfaceMatches must accept exactly the config ApplyKernelSurfacePolicy
// would write, and reject any drift.
func TestKernelSurfaceMatches(t *testing.T) {
	p := HardeningPolicy{Docker: false, ReduceKernelSurface: true}
	match := map[string]string{
		"security.nesting":                                 "",
		"security.syscalls.intercept.mknod":                "",
		"security.syscalls.intercept.setxattr":             "",
		"linux.sysctl.net.ipv4.ip_unprivileged_port_start": "",
		"security.syscalls.deny":                           KernelSurfaceDenySyscalls,
	}
	if !KernelSurfaceMatches(match, p) {
		t.Error("expected match for the exact hardened config")
	}
	// Whitespace around a value must not defeat the match.
	match["security.syscalls.deny"] = " " + KernelSurfaceDenySyscalls + " "
	if !KernelSurfaceMatches(match, p) {
		t.Error("expected match despite surrounding whitespace")
	}
	// A stray nesting=true is drift.
	drift := map[string]string{"security.nesting": "true"}
	if KernelSurfaceMatches(drift, p) {
		t.Error("expected mismatch when nesting is still on")
	}
	// Default policy matches a docker-on container.
	dockerOn := map[string]string{
		"security.nesting":                                 "true",
		"security.syscalls.intercept.mknod":                "true",
		"security.syscalls.intercept.setxattr":             "true",
		"linux.sysctl.net.ipv4.ip_unprivileged_port_start": "0",
		"security.syscalls.deny":                           "",
	}
	if !KernelSurfaceMatches(dockerOn, DefaultHardeningPolicy()) {
		t.Error("default policy should match a docker-on container")
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
