"""
Test for coi persist --all flag.

Tests that:
1. Launch 2 ephemeral containers
2. Run coi persist --all --force
3. Verify both containers' metadata updated
4. Verify success message shows "Persisted 2"
"""

import json
import subprocess
import time
from pathlib import Path

from support.helpers import calculate_container_name


def test_persist_all_flag(coi_binary, cleanup_containers, workspace_dir):
    """
    Test persist --all flag on multiple containers.

    Flow:
    1. Launch 2 ephemeral containers
    2. Persist all with --all --force
    3. Verify both metadata files updated
    4. Verify success output
    """
    container_name_1 = calculate_container_name(workspace_dir, 1)
    container_name_2 = calculate_container_name(workspace_dir, 2)

    # === Phase 1: Launch 2 containers ===

    result = subprocess.run(
        [coi_binary, "container", "launch", "coi", container_name_1],
        capture_output=True,
        text=True,
        timeout=120,
    )
    assert result.returncode == 0, f"Container 1 launch should succeed. stderr: {result.stderr}"

    result = subprocess.run(
        [coi_binary, "container", "launch", "coi", container_name_2],
        capture_output=True,
        text=True,
        timeout=120,
    )
    assert result.returncode == 0, f"Container 2 launch should succeed. stderr: {result.stderr}"

    time.sleep(3)

    # === Phase 2: Persist all containers ===

    result = subprocess.run(
        [coi_binary, "persist", "--all", "--force"],
        capture_output=True,
        text=True,
        timeout=60,
    )
    assert result.returncode == 0, f"Persist --all should succeed. stderr: {result.stderr}"

    combined_output = result.stdout + result.stderr
    assert "Persisted 2" in combined_output, (
        f"Should show 'Persisted 2' in output. Got:\n{combined_output}"
    )

    # === Phase 3: Verify both metadata files updated ===

    sessions_dir = Path.home() / ".coi" / "sessions-claude"
    assert sessions_dir.exists(), f"Sessions directory should exist: {sessions_dir}"

    # Helper to find metadata for a container
    def find_metadata(container_name):
        for session_dir in sessions_dir.iterdir():
            if not session_dir.is_dir():
                continue

            metadata_path = session_dir / "metadata.json"
            if not metadata_path.exists():
                continue

            try:
                with open(metadata_path) as f:
                    metadata = json.load(f)

                if metadata.get("container_name") == container_name:
                    return metadata
            except (json.JSONDecodeError, KeyError):
                continue

        return None

    # Check container 1 metadata
    metadata_1 = find_metadata(container_name_1)
    assert metadata_1 is not None, f"Should find metadata for container {container_name_1}"
    assert metadata_1["persistent"] is True, "Container 1 should be persistent"

    # Check container 2 metadata
    metadata_2 = find_metadata(container_name_2)
    assert metadata_2 is not None, f"Should find metadata for container {container_name_2}"
    assert metadata_2["persistent"] is True, "Container 2 should be persistent"
