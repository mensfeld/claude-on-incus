"""
Test for cci image exists - check existing image.

Tests that:
1. Check for cci image (should exist after build)
2. Verify exit code is 0
"""

import subprocess


def test_exists_coi_image(coi_binary, cleanup_containers):
    """
    Test checking if the cci image exists.

    Flow:
    1. Run cci image exists cci
    2. Verify exit code is 0 (image exists)

    Note: This test assumes the cci image has been built.
    """
    # === Phase 1: Check if cci image exists ===

    result = subprocess.run(
        [coi_binary, "image", "exists", "cci"],
        capture_output=True,
        text=True,
        timeout=30,
    )

    # === Phase 2: Verify success ===

    assert result.returncode == 0, f"cci image should exist. stderr: {result.stderr}"
