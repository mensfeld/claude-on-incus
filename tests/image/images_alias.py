"""
Test for cci images - alias for cci image list.

Tests that:
1. Run cci images
2. Verify it behaves like cci image list
"""

import subprocess


def test_images_alias(coi_binary, cleanup_containers):
    """
    Test that 'cci images' is an alias for 'cci image list'.

    Flow:
    1. Run cci images
    2. Verify output is similar to cci image list
    """
    # === Phase 1: Run cci images ===

    result = subprocess.run(
        [coi_binary, "images"],
        capture_output=True,
        text=True,
        timeout=30,
    )

    assert result.returncode == 0, f"cci images should succeed. stderr: {result.stderr}"

    # === Phase 2: Verify output format ===

    combined_output = result.stdout + result.stderr

    # Should show same content as image list
    assert "COI Images:" in combined_output or "Available Images:" in combined_output, (
        f"Should show COI Images section. Got:\n{combined_output}"
    )


def test_images_all_flag(coi_binary, cleanup_containers):
    """
    Test that 'cci images --all' works.

    Flow:
    1. Run cci images --all
    2. Verify it shows all local images
    """
    # === Phase 1: Run cci images --all ===

    result = subprocess.run(
        [coi_binary, "images", "--all"],
        capture_output=True,
        text=True,
        timeout=30,
    )

    assert result.returncode == 0, f"cci images --all should succeed. stderr: {result.stderr}"

    # === Phase 2: Verify output ===

    combined_output = result.stdout + result.stderr

    # Should show All Local Images section
    assert "All Local Images:" in combined_output or "ALIAS" in combined_output, (
        f"Should show All Local Images section. Got:\n{combined_output}"
    )
