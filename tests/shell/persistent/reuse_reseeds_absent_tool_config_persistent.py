"""
#708: on persistent REUSE, coi seeds a tool's CLI config when that tool's config
dir is absent in the container — the fix that lets you re-enter a persistent box
(same code/packages/state) with a DIFFERENT tool and have it authenticate.

Directly exercises the new reuse branch in session.Setup (toolConfigDirPresent →
setupCLIConfig), which the unit test only covers in isolation. Uses a fake host
~/.codex (so there is something to seed) + COI_USE_DUMMY (so the codex binary
isn't needed), mirroring tests/config/test_codex_config_seeding.py.

Flow: create persistent container (codex config seeded) → delete ~/.codex inside
it → stop → fresh `coi shell` (no --resume) reuses the stopped box → assert
~/.codex was RE-seeded.
"""

import subprocess
import time

from support.helpers import (
    calculate_container_name,
    spawn_coi,
    wait_for_container_ready,
)

AUTH_JSON = '{"tokens": {"access_token": "reuse-reseed-token-708"}}'
CONFIG_TOML = 'model = "gpt-5-codex"\n'
AGENTS_MD = "# codex instructions\nUSER-CODEX-CONTENT\n"

CODEX_DIR = "/home/code/.codex"


def _incus(argv, timeout=30):
    return subprocess.run(
        ["sg", "incus-admin", "-c", "incus " + argv],
        capture_output=True,
        text=True,
        timeout=timeout,
    )


def _cat(container, path):
    r = _incus(f"exec {container} -- cat {path}")
    return r.returncode == 0, r.stdout


def _dir_present(container, path):
    return _incus(f"exec {container} -- test -d {path}").returncode == 0


def _close(child):
    try:
        child.send("exit")
        time.sleep(0.3)
        child.send("\x0d")
        time.sleep(1)
    except Exception:
        pass
    try:
        child.close(force=False)
    except Exception:
        child.close(force=True)


def test_reuse_reseeds_absent_tool_config(coi_binary, cleanup_containers, workspace_dir, tmp_path):
    container_name = calculate_container_name(workspace_dir, 1)

    # Select codex + persistent via project config.
    coi_dir = f"{workspace_dir}/.coi"
    subprocess.run(["mkdir", "-p", coi_dir], check=True)
    with open(f"{coi_dir}/config.toml", "w") as f:
        f.write('[tool]\nname = "codex"\n[container]\npersistent = true\n')

    # Fake host home with a populated ~/.codex to seed from.
    fake_home = tmp_path / "fake_home"
    codex_dir = fake_home / ".codex"
    codex_dir.mkdir(parents=True)
    (codex_dir / "auth.json").write_text(AUTH_JSON)
    (codex_dir / "config.toml").write_text(CONFIG_TOML)
    (codex_dir / "AGENTS.md").write_text(AGENTS_MD)

    env = {"COI_USE_DUMMY": "1", "HOME": str(fake_home)}

    seeded_at_create = reseeded_on_reuse = False
    absent_after_rm = False
    try:
        # === Phase 1: create — codex config seeded from host ===
        child = spawn_coi(coi_binary, ["shell"], cwd=workspace_dir, env=env, timeout=120)
        wait_for_container_ready(child, timeout=60)
        time.sleep(5)
        ok, content = _cat(container_name, f"{CODEX_DIR}/auth.json")
        seeded_at_create = ok and content == AUTH_JSON

        # Remove the tool's config dir, simulating a tool this container hasn't
        # seen; the container is still running here.
        _incus(f"exec {container_name} -- rm -rf {CODEX_DIR}")
        absent_after_rm = not _dir_present(container_name, CODEX_DIR)

        # Stop + keep the container so a fresh session reuses it.
        _close(child)
        time.sleep(2)
        _incus(f"stop {container_name} --force", timeout=60)
        time.sleep(2)

        # === Phase 2: fresh `coi shell` (no --resume) reuses the stopped box ===
        child2 = spawn_coi(coi_binary, ["shell"], cwd=workspace_dir, env=env, timeout=120)
        wait_for_container_ready(child2, timeout=60)
        time.sleep(5)
        ok2, content2 = _cat(container_name, f"{CODEX_DIR}/auth.json")
        reseeded_on_reuse = ok2 and content2 == AUTH_JSON
        _close(child2)
    finally:
        time.sleep(2)
        subprocess.run(
            [coi_binary, "container", "delete", container_name, "--force"],
            capture_output=True,
            timeout=30,
        )

    assert seeded_at_create, "codex config should be seeded from the host at container creation"
    assert absent_after_rm, "test setup failed: ~/.codex should be gone after rm"
    assert reseeded_on_reuse, (
        "~/.codex/auth.json should be RE-seeded when a fresh session reuses the "
        "persistent container and the tool's config dir is absent (#708)"
    )
