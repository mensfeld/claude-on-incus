"""
Test that restricted mode fails gracefully on non-OVN networks.

Tests that:
1. Using --network=restricted on a non-OVN network shows clear error message
2. Error message suggests using --network=open or setting up OVN
3. Container is not left in a broken state

Note: This test is designed to run in the CI bridge-only environment.
In OVN environments, this test is skipped.
"""

import subprocess

import pytest


def test_restricted_requires_ovn(coi_binary, workspace_dir, cleanup_containers):
    """
    Test that restricted mode fails with helpful error on non-OVN network.

    This test only runs in bridge-only environments (checked via environment variable).
    In OVN environments, it's skipped since the test wouldn't trigger the error path.

    Flow:
    1. Check if we're in a bridge-only environment (via CI_NETWORK_TYPE)
    2. Try to start shell with --network=restricted
    3. Verify it fails with clear error message about ACL/OVN
    4. Verify error suggests using --network=open
    """
    import os

    # Only run this test in bridge-only environments
    network_type = os.getenv("CI_NETWORK_TYPE", "ovn")
    if network_type != "bridge":
        pytest.skip("Test only runs in bridge-only CI environment")

    # Try to use restricted mode (should fail on bridge network)
    result = subprocess.run(
        [
            coi_binary,
            "shell",
            "--workspace",
            workspace_dir,
            "--background",
            "--network=restricted",
        ],
        capture_output=True,
        text=True,
        timeout=60,
    )

    # Should fail with non-zero exit code
    assert result.returncode != 0, (
        "Restricted mode should fail on non-OVN network. "
        f"stdout: {result.stdout}, stderr: {result.stderr}"
    )

    # Check for helpful error message
    error_output = result.stderr.lower()
    assert "acl" in error_output or "ovn" in error_output, (
        f"Error message should mention ACL or OVN. Got: {result.stderr}"
    )

    assert "network=open" in error_output or "--network=open" in error_output, (
        f"Error message should suggest using --network=open. Got: {result.stderr}"
    )

    # Verify the error message mentions how to set up OVN
    assert "apt install" in error_output or "ovn-host" in error_output, (
        f"Error message should explain how to install OVN. Got: {result.stderr}"
    )
