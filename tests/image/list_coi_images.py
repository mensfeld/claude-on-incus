"""
Test for cci image list - list COI images (default behavior).

Tests that:
1. Run cci image list
2. Verify it shows COI images section
3. Verify output format is correct
"""

import subprocess


def test_list_coi_images(coi_binary, cleanup_containers):
    """
    Test listing COI images (default behavior).

    Flow:
    1. Run cci image list
    2. Verify output contains COI Images section
    3. Verify cci image is shown (exists or not built)
    """
    # === Phase 1: Run image list ===

    result = subprocess.run(
        [coi_binary, "image", "list"],
        capture_output=True,
        text=True,
        timeout=30,
    )

    assert result.returncode == 0, f"Image list should succeed. stderr: {result.stderr}"

    # === Phase 2: Verify output format ===

    combined_output = result.stdout + result.stderr
    assert "COI Images:" in combined_output or "Available Images:" in combined_output, (
        f"Should show COI Images section. Got:\n{combined_output}"
    )

    # Should mention the cci image (either built or not)
    assert "cci" in combined_output.lower(), f"Should mention cci image. Got:\n{combined_output}"
