"""Test coi list --format=json with no containers"""

import json
import subprocess


def test_list_format_json_empty(coi_binary, cleanup_containers):
    """Test that coi list --format=json outputs valid JSON with no containers."""
    # cleanup_containers fixture ensures all test containers are cleaned up

    # First, forcefully clean ALL containers to ensure truly empty state
    subprocess.run(
        [coi_binary, "kill", "--all", "--force"],
        capture_output=True,
        timeout=30,
        check=False,
    )

    # Run list with JSON format (no containers running)
    result = subprocess.run(
        [coi_binary, "list", "--format=json"],
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert result.returncode == 0, f"List failed: {result.stderr}"

    # Parse and verify JSON
    data = json.loads(result.stdout)

    # Verify structure
    assert "active_containers" in data, "Missing 'active_containers' key"
    assert isinstance(data["active_containers"], list), "active_containers should be a list"
    assert len(data["active_containers"]) == 0, "Should have no containers when none are running"
