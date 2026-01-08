"""
Pytest configuration and fixtures for pexpect-based CLI tests.
"""

import os
import shutil
import subprocess
import uuid
from pathlib import Path

import pytest

# Set container prefix for all tests to avoid interfering with user's active sessions
os.environ["COI_CONTAINER_PREFIX"] = "coi-test-"


@pytest.fixture(scope="session")
def project_root():
    """Get the project root directory."""
    # __file__ = tests/conftest.py
    # .parent = tests/
    # .parent = claude-on-incus/
    return Path(__file__).parent.parent


@pytest.fixture(scope="session")
def coi_binary(project_root):
    """
    Build and return path to coi binary.
    Built once per test session and cached.
    """
    binary_path = project_root / "coi"

    # Build if not exists or is outdated
    print(f"\nBuilding coi binary at {binary_path}...")
    result = subprocess.run(["make", "build"], cwd=project_root, capture_output=True, text=True)

    if result.returncode != 0:
        pytest.fail(f"Failed to build binary:\n{result.stdout}\n{result.stderr}")

    if not binary_path.exists():
        pytest.fail(f"Binary not found at {binary_path} after build")

    print(f"Binary ready at {binary_path}")
    return str(binary_path)


@pytest.fixture
def cleanup_containers(request):
    """
    Cleanup fixture that runs after each test.
    Cleans up any containers created during the test.
    """
    from support.helpers import cleanup_all_test_containers

    # Run test
    yield

    # Cleanup after test (even if it failed)
    print("\nCleaning up test containers...")
    try:
        cleanup_all_test_containers()
    except Exception as e:
        print(f"Warning: Cleanup failed: {e}")


@pytest.fixture(scope="session")
def integrations_tmp_dir(project_root):
    """
    Create project-local tmp directory for integration tests.
    Cleaned up after all tests complete.
    """
    tmp_dir = project_root / "tmp" / "integrations"
    tmp_dir.mkdir(parents=True, exist_ok=True)

    yield tmp_dir

    # Cleanup after all tests complete
    print(f"\nCleaning up integration test directory: {tmp_dir}")
    try:
        shutil.rmtree(tmp_dir)
    except Exception as e:
        print(f"Warning: Failed to cleanup {tmp_dir}: {e}")


@pytest.fixture
def workspace_dir(integrations_tmp_dir, request):
    """
    Create a temporary workspace directory for the test.
    Uses UUID for unique directory per test run.
    Located in project_root/tmp/integrations/{uuid}/
    """
    # Use UUID for unique directory
    test_uuid = str(uuid.uuid4())[:8]  # Short UUID for readability
    test_name = request.node.name

    workspace = integrations_tmp_dir / f"{test_name}_{test_uuid}"
    workspace.mkdir(parents=True, exist_ok=True)
    return str(workspace)


@pytest.fixture
def sessions_dir():
    """Get the sessions directory."""
    home = Path.home()
    return str(home / ".coi" / "sessions")
