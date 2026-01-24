"""
Test for coi persist - verify container not deleted on stop.

Tests that:
1. Launch ephemeral container
2. Persist it
3. Stop container (not delete)
4. Verify container still exists after stop
"""

import subprocess
import time

from support.helpers import calculate_container_name


def test_persist_no_delete_on_stop(coi_binary, cleanup_containers, workspace_dir):
    """
    Test that persisted containers are not deleted when stopped.

    Flow:
    1. Launch ephemeral container
    2. Persist it
    3. Stop the container
    4. Verify container still exists (not deleted)
    """
    container_name = calculate_container_name(workspace_dir, 1)

    # === Phase 1: Launch ephemeral container ===

    result = subprocess.run(
        [coi_binary, "container", "launch", "coi", container_name],
        capture_output=True,
        text=True,
        timeout=120,
    )
    assert result.returncode == 0, f"Container launch should succeed. stderr: {result.stderr}"

    time.sleep(3)

    # Verify container is running
    result = subprocess.run(
        [coi_binary, "container", "running", container_name],
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert result.returncode == 0, f"Container should be running. stderr: {result.stderr}"

    # === Phase 2: Persist the container ===

    result = subprocess.run(
        [coi_binary, "persist", container_name],
        capture_output=True,
        text=True,
        timeout=60,
    )
    assert result.returncode == 0, f"Persist should succeed. stderr: {result.stderr}"

    # === Phase 3: Stop the container ===

    result = subprocess.run(
        [coi_binary, "container", "stop", container_name],
        capture_output=True,
        text=True,
        timeout=60,
    )
    assert result.returncode == 0, f"Container stop should succeed. stderr: {result.stderr}"

    time.sleep(5)  # Wait for stop to complete

    # === Phase 4: Verify container still exists ===

    result = subprocess.run(
        [coi_binary, "container", "exists", container_name],
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert result.returncode == 0, (
        f"Container should still exist after stop (persistent mode). stdout: {result.stdout}"
    )

    # Verify it's stopped (not running)
    result = subprocess.run(
        [coi_binary, "container", "running", container_name],
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert result.returncode != 0, "Container should not be running after stop"
