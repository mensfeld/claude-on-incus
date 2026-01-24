"""
Test for coi persist - basic persist operation.

Tests that:
1. Launch an ephemeral container
2. Verify metadata shows persistent: false
3. Run coi persist <container-name>
4. Verify metadata updated to persistent: true
5. Verify container still exists
"""

import json
import subprocess
import time
from pathlib import Path

from support.helpers import calculate_container_name


def test_persist_basic(coi_binary, cleanup_containers, workspace_dir):
    """
    Test basic persist operation on a single container.

    Flow:
    1. Launch ephemeral container
    2. Verify metadata shows persistent: false
    3. Persist the container
    4. Verify metadata shows persistent: true
    5. Verify container still exists
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

    # === Phase 2: Find and verify metadata shows persistent: false ===

    # Find sessions directory
    sessions_dir = Path.home() / ".coi" / "sessions-claude"
    assert sessions_dir.exists(), f"Sessions directory should exist: {sessions_dir}"

    # Find metadata for this container
    metadata_path = None
    for session_dir in sessions_dir.iterdir():
        if not session_dir.is_dir():
            continue

        candidate_path = session_dir / "metadata.json"
        if not candidate_path.exists():
            continue

        try:
            with open(candidate_path) as f:
                metadata = json.load(f)

            if metadata.get("container_name") == container_name:
                metadata_path = candidate_path
                break
        except (json.JSONDecodeError, KeyError):
            continue

    assert metadata_path is not None, f"Should find metadata for container {container_name}"

    # Verify persistent is false
    with open(metadata_path) as f:
        metadata = json.load(f)

    assert metadata["persistent"] is False, "Container should initially be ephemeral"

    # === Phase 3: Persist the container ===

    result = subprocess.run(
        [coi_binary, "persist", container_name],
        capture_output=True,
        text=True,
        timeout=60,
    )
    assert result.returncode == 0, f"Persist should succeed. stderr: {result.stderr}"

    combined_output = result.stdout + result.stderr
    assert "Persisted" in combined_output or "persisted" in combined_output.lower(), (
        f"Should show persisted confirmation. Got:\n{combined_output}"
    )

    # === Phase 4: Verify metadata updated to persistent: true ===

    with open(metadata_path) as f:
        metadata = json.load(f)

    assert metadata["persistent"] is True, "Container should now be persistent"

    # === Phase 5: Verify container still exists ===

    result = subprocess.run(
        [coi_binary, "container", "exists", container_name],
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert result.returncode == 0, f"Container should still exist. stdout: {result.stdout}"
