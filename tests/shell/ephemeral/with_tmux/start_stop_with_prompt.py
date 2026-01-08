"""
Scenario: Claude responds to basic interaction.

Flow:
1. Start shell
2. Wait for Claude to be ready
3. Ask Claude a simple math question
4. Verify Claude responds with correct answer

Expected:
- Claude is ready and responding
- Can interact with Claude
- Claude executes simple requests
"""

import time

from support.helpers import (
    assert_clean_exit,
    exit_claude,
    send_prompt,
    spawn_coi,
    wait_for_container_deletion,
    wait_for_container_ready,
    wait_for_prompt,
    wait_for_text_in_monitor,
    with_live_screen,
)


def test_claude_responds_to_request(coi_binary, cleanup_containers, workspace_dir):
    child = spawn_coi(coi_binary, ["shell", "--tmux=true"], cwd=workspace_dir)

    wait_for_container_ready(child)
    wait_for_prompt(child)

    with with_live_screen(child) as monitor:
        time.sleep(2)
        send_prompt(child, "Print first 10 PI digits")
        responded = wait_for_text_in_monitor(monitor, "14159", timeout=30)

        # Exit Claude and wait for container cleanup while monitor is still running
        clean_exit = exit_claude(child)
        wait_for_container_deletion()  # Wait for Incus cleanup to complete

    # Now assert after monitor has stopped
    assert responded, "Claude did not respond with correct answer '14159'"
    assert_clean_exit(clean_exit, child)
