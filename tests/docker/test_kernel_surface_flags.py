"""
Tests for the kernel attack-surface hardening flags:
[container] docker and [security] reduce_kernel_surface.

Tests that:
1. [container] docker = false in a project config (a tightening, honored from
   untrusted scope) launches without any of the four Docker-support keys.
2. [security] reduce_kernel_surface = true in TRUSTED config disables Docker
   support AND sets security.syscalls.deny — while git keeps working inside
   the container (the io_uring/keyring smoke gate).
3. An untrusted project config cannot re-enable docker that trusted config
   disabled.
4. A persistent container reconciles a changed policy on its next
   stopped -> start cycle.
"""

import subprocess

from support.helpers import (
    calculate_container_name,
    write_trusted_coi_config,
)

DENY_SYSCALLS = [
    "io_uring_setup",
    "io_uring_enter",
    "io_uring_register",
    "bpf",
    "userfaultfd",
    "keyctl",
    "add_key",
    "request_key",
]

DOCKER_KEYS = [
    "security.nesting",
    "security.syscalls.intercept.mknod",
    "security.syscalls.intercept.setxattr",
    "linux.sysctl.net.ipv4.ip_unprivileged_port_start",
]


def incus_config_get(container_name, key):
    result = subprocess.run(
        ["incus", "--project", "default", "config", "get", container_name, key],
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert result.returncode == 0, f"incus config get {key} failed: {result.stderr}"
    return result.stdout.strip()


def write_project_config(workspace_dir, content):
    import os

    coi_dir = os.path.join(workspace_dir, ".coi")
    os.makedirs(coi_dir, exist_ok=True)
    with open(os.path.join(coi_dir, "config.toml"), "w") as f:
        f.write(content)


def coi_run(coi_binary, workspace_dir, argv, env=None, timeout=240):
    return subprocess.run(
        [coi_binary, "run", "--workspace", workspace_dir, *argv],
        capture_output=True,
        text=True,
        timeout=timeout,
        env=env,
    )


def test_docker_disabled_via_project_config(coi_binary, cleanup_containers, workspace_dir):
    """[container] docker = false is a tightening, so a project config may set
    it; the container must then carry none of the Docker-support keys and no
    syscall deny list."""
    # persistent = true keeps the container around (stopped) after the run so
    # its host-side config can be inspected.
    write_project_config(workspace_dir, "[container]\npersistent = true\ndocker = false\n")
    container_name = calculate_container_name(workspace_dir, 1)

    result = coi_run(coi_binary, workspace_dir, ["true"])
    assert result.returncode == 0, f"coi run should succeed. stderr: {result.stderr}"

    for key in DOCKER_KEYS:
        value = incus_config_get(container_name, key)
        assert value in ("", "false"), f"{key} should be unset with docker=false, got {value!r}"
    assert incus_config_get(container_name, "security.syscalls.deny") == "", (
        "security.syscalls.deny should stay unset without reduce_kernel_surface"
    )


def test_reduce_kernel_surface_hardening(coi_binary, cleanup_containers, workspace_dir):
    """Trusted [security] reduce_kernel_surface = true must disable Docker
    support and install the syscall deny list — and the container must still
    boot and run real work (git) with those syscalls denied."""
    env = write_trusted_coi_config("[security]\nreduce_kernel_surface = true\n")
    write_project_config(workspace_dir, "[container]\npersistent = true\n")
    container_name = calculate_container_name(workspace_dir, 1)

    # The command doubles as the io_uring/keyring smoke gate: a boot or exec
    # failure under the deny list fails the run itself.
    result = coi_run(
        coi_binary,
        workspace_dir,
        ["sh", "-c", "git --version && echo SMOKE_OK"],
        env=env,
    )
    assert result.returncode == 0, (
        f"coi run should succeed under hardening. stderr: {result.stderr}"
    )
    assert "SMOKE_OK" in result.stdout + result.stderr

    for key in DOCKER_KEYS:
        value = incus_config_get(container_name, key)
        assert value in ("", "false"), f"{key} should be unset under reduce_kernel_surface"
    deny = incus_config_get(container_name, "security.syscalls.deny")
    for syscall in DENY_SYSCALLS:
        assert syscall in deny.split(), f"{syscall} missing from security.syscalls.deny: {deny!r}"


def test_untrusted_docker_reenable_ignored(coi_binary, cleanup_containers, workspace_dir):
    """A project config's docker = true must not re-enable nesting that
    trusted config disabled (an untrusted repo must not widen the kernel
    surface)."""
    env = write_trusted_coi_config("[container]\ndocker = false\n")
    write_project_config(workspace_dir, "[container]\npersistent = true\ndocker = true\n")
    container_name = calculate_container_name(workspace_dir, 1)

    result = coi_run(coi_binary, workspace_dir, ["true"], env=env)
    assert result.returncode == 0, f"coi run should succeed. stderr: {result.stderr}"

    assert "container.docker" in result.stderr, (
        "expected the untrusted-downgrade warning naming container.docker"
    )
    assert incus_config_get(container_name, "security.nesting") in ("", "false"), (
        "untrusted docker=true must not re-enable security.nesting"
    )


def test_persistent_reuse_reconciles_policy(coi_binary, cleanup_containers, workspace_dir):
    """A persistent container created with the default policy must converge to
    reduce_kernel_surface on its next stopped -> start cycle (and back)."""
    write_project_config(workspace_dir, "[container]\npersistent = true\n")
    container_name = calculate_container_name(workspace_dir, 1)

    # First run: defaults — Docker support on.
    result = coi_run(coi_binary, workspace_dir, ["true"])
    assert result.returncode == 0, f"first run should succeed. stderr: {result.stderr}"
    assert incus_config_get(container_name, "security.nesting") == "true"

    # Second run: hardening from trusted scope — the stopped-window reconcile
    # must flip nesting off and install the deny list on the SAME container.
    env = write_trusted_coi_config("[security]\nreduce_kernel_surface = true\n")
    result = coi_run(coi_binary, workspace_dir, ["true"], env=env)
    assert result.returncode == 0, f"hardened reuse should succeed. stderr: {result.stderr}"
    assert incus_config_get(container_name, "security.nesting") in ("", "false"), (
        "reuse must unset security.nesting when hardening is enabled"
    )
    assert "bpf" in incus_config_get(container_name, "security.syscalls.deny").split()

    # Third run: back to defaults — the deny list must be removed again.
    result = coi_run(coi_binary, workspace_dir, ["true"])
    assert result.returncode == 0, f"default reuse should succeed. stderr: {result.stderr}"
    assert incus_config_get(container_name, "security.nesting") == "true"
    assert incus_config_get(container_name, "security.syscalls.deny") == "", (
        "reuse must unset security.syscalls.deny when hardening is disabled again"
    )
