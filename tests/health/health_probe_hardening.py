"""
E2E test: `coi health` probe containers honor the kernel-surface policy.

The four runtime-isolation probes (connectivity, network restriction, secret
masking, host-credential isolation) launch short-lived real containers. Under
[security] reduce_kernel_surface those probes must NOT silently boot with the
full docker/nesting surface — the exact class of widening the policy exists
to prevent.

The probes only live a few seconds, so the test runs `coi health` in the
background and polls `incus list` fast, snapshotting each probe container's
expanded config the moment it appears.
"""

import json
import os
import subprocess
import tempfile
import time

PROBE_PREFIXES = (
    "coi-health-check-",
    "coi-restriction-check-",
    "coi-secret-check-",
    "coi-hostcred-check-",
)


def _write_hardened_config():
    fd, path = tempfile.mkstemp(suffix=".toml", prefix="coi-hardened-")
    with os.fdopen(fd, "w") as f:
        f.write("[security]\nreduce_kernel_surface = true\n")
    return {**os.environ, "COI_CONFIG": path}


def _list_probe_containers():
    result = subprocess.run(
        ["incus", "list", "--format", "csv", "-c", "n", "coi-"],
        capture_output=True,
        text=True,
        timeout=10,
    )
    names = []
    for row in result.stdout.strip().splitlines():
        name = row.strip()
        if name.startswith(PROBE_PREFIXES):
            names.append(name)
    return names


def _expanded_get(name, key):
    result = subprocess.run(
        ["incus", "config", "get", "--expanded", name, key],
        capture_output=True,
        text=True,
        timeout=10,
    )
    return result.returncode, result.stdout.strip()


def test_health_probes_honor_reduce_kernel_surface(coi_binary, cleanup_containers):
    """While a hardened `coi health` runs, every probe container observed must
    carry the hardened policy: nesting off and the syscall deny list set."""
    env = _write_hardened_config()

    proc = subprocess.Popen(
        [coi_binary, "health", "--format", "json"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        env=env,
    )

    observed = {}  # probe name -> (nesting, deny)
    try:
        # Poll fast while the health run is alive; each probe lives seconds.
        while proc.poll() is None:
            for name in _list_probe_containers():
                if name in observed:
                    continue
                rc_n, nesting = _expanded_get(name, "security.nesting")
                rc_d, deny = _expanded_get(name, "security.syscalls.deny")
                # The container may vanish between list and get; only record
                # complete reads.
                if rc_n == 0 and rc_d == 0:
                    observed[name] = (nesting, deny)
            time.sleep(0.1)
        stdout, stderr = proc.communicate(timeout=30)
    finally:
        if proc.poll() is None:
            proc.kill()

    # The health run itself must have completed and produced JSON — hardened
    # probes must still boot and function (the deny list breaks nothing).
    assert proc.returncode in (0, 1, 2), f"coi health crashed: rc={proc.returncode}\n{stderr}"
    data = json.loads(stdout)
    assert "checks" in data

    assert observed, (
        "no probe container was observed while coi health ran — the probes "
        "may have been skipped (no Incus?) or the polling missed them; "
        f"stderr:\n{stderr[-2000:]}"
    )
    for name, (nesting, deny) in observed.items():
        assert nesting.lower() not in ("true", "1", "yes", "on"), (
            f"probe {name} booted with security.nesting={nesting!r} despite "
            "reduce_kernel_surface — probes must honor the kernel-surface policy"
        )
        assert "bpf" in deny.split(), f"probe {name} is missing the syscall deny list, got {deny!r}"
