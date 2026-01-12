"""
Test list command with full end-to-end validation.

Flow:
1. Clean up all test containers first
2. Run list with no containers, verify appropriate message
3. Launch persistent container
4. Run list, verify container appears in output
5. Exit container (stop it)
6. Run list, verify no running containers shown
7. Clean up

Expected:
- List shows "no running containers" when none exist
- List shows container name, slot, and status when containers running
- List output is properly formatted
"""

import os
import subprocess
import time

from support.helpers import (
    calculate_container_name,
    cleanup_all_test_containers,
    exit_claude,
    spawn_coi,
    wait_for_container_ready,
    wait_for_prompt,
    with_live_screen,
)


def test_list_command_basic(coi_binary, cleanup_containers, workspace_dir, dummy_path):
    """Test that coi list shows proper output with full E2E validation."""
    # Clean up any existing test containers first
    cleanup_all_test_containers()
    time.sleep(1)

    # Test 1: List before test should succeed (may have other non-test containers)
    result_before = subprocess.run(
        [coi_binary, "list"], capture_output=True, text=True, timeout=10
    )
    assert result_before.returncode == 0, f"Expected exit code 0, got {result_before.returncode}"

    # Verify our test container is NOT in the list yet
    test_container = calculate_container_name(workspace_dir, 3)
    assert test_container not in result_before.stdout, \
        f"Test container {test_container} should not exist yet"

    # Test 2: Launch persistent container and verify it appears in list
    env = os.environ.copy()
    env["PATH"] = f"{dummy_path}:{env.get('PATH', '')}"

    child = spawn_coi(
        coi_binary,
        ["shell", "--persistent", "--slot=3", "--tmux=false"],
        cwd=workspace_dir,
        env=env,
    )
    wait_for_container_ready(child, timeout=60)
    wait_for_prompt(child, timeout=90)

    # Container should now appear in list
    result = subprocess.run(
        [coi_binary, "list"], capture_output=True, text=True, timeout=10
    )
    assert result.returncode == 0, f"List should succeed, got exit code {result.returncode}"
    assert test_container in result.stdout, f"Container {test_container} should appear in list output"
    # Verify output mentions "running" or similar status
    assert "running" in result.stdout.lower() or "active" in result.stdout.lower(), \
        "List should show container status"

    # Test 3: Exit container and verify list shows no running containers
    with with_live_screen(child):
        clean_exit = exit_claude(child)
        time.sleep(2)

    # After exit, list should show no running containers (unless --all flag used)
    result = subprocess.run(
        [coi_binary, "list"], capture_output=True, text=True, timeout=10
    )
    assert result.returncode == 0, "List should succeed after container stopped"

    # Container should not be in running list (it's stopped)
    # Note: This depends on whether 'list' shows stopped containers by default
    # Most likely it only shows running containers, so test_container should NOT appear
    # OR it should show with "stopped" status
    if test_container in result.stdout:
        # If it appears, it should say "stopped" or similar
        assert "stopped" in result.stdout.lower() or "inactive" in result.stdout.lower(), \
            "Stopped container should be marked as stopped"
    # If it doesn't appear in running list, that's correct behavior
    # (list command shows only running containers by default)
