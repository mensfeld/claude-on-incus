"""
A profile's env_command_timeout is honored end-to-end (parity fix).

env_command_timeout used to be settable only under top-level [defaults]; a
profile could set env_commands but not the timeout that bounds them. This test
puts both in a TRUSTED profile (~/.coi/profiles, seeded via a fake HOME so
env_commands are not stripped as untrusted) and runs a command that sleeps far
longer than the profile's 1s timeout. coi aborts the launch with the profile's
timeout in the message — proving the profile field reached the resolver.

The message carries the exact duration, so it also distinguishes a regression:
if the profile timeout were ignored, the fallback would be the 30s default and
the message would read "timed out after 30s", not "1s".
"""

import os
import subprocess
from pathlib import Path

import pytest


def _env_with_home(fake_home: Path) -> dict:
    env = os.environ.copy()
    env["HOME"] = str(fake_home)
    env.pop("COI_CONFIG", None)
    return env


def test_profile_env_command_timeout_applied(coi_binary, cleanup_containers, tmp_path):
    # Base image is needed to reach the run configure phase where env_commands
    # resolve; skip cleanly if it isn't present.
    if (
        subprocess.run(
            [coi_binary, "image", "exists", "coi-default"], capture_output=True
        ).returncode
        != 0
    ):
        pytest.skip("coi-default base image not present")

    fake_home = tmp_path / "home"
    fake_home.mkdir()
    prof_dir = fake_home / ".coi" / "profiles" / "slow"
    prof_dir.mkdir(parents=True)
    # env_command_timeout is a profile-ROOT key: it must appear before any
    # [table] header, or TOML scopes it into that table (e.g. container.*) and
    # the profile fails schema validation instead of setting the timeout.
    (prof_dir / "config.toml").write_text(
        'env_command_timeout = "1s"\n\n'
        "[container]\n"
        'image = "coi-default"\n\n'
        "[env_commands]\n"
        'SLOW = "sleep 60"\n'
    )

    workspace = tmp_path / "workspace"
    workspace.mkdir()

    result = subprocess.run(
        [coi_binary, "run", "--profile", "slow", "--workspace", str(workspace), "--", "true"],
        capture_output=True,
        text=True,
        timeout=120,
        cwd=str(workspace),
        env=_env_with_home(fake_home),
    )

    combined = result.stdout + result.stderr
    assert result.returncode != 0, f"launch should abort on the slow env_command. Got:\n{combined}"
    assert "timed out after 1s" in combined, (
        "the profile's env_command_timeout (1s) must be applied — a regression would "
        f"fall back to the 30s default. Got:\n{combined}"
    )
