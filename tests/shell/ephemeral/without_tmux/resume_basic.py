"""
Test basic session resume functionality.

Flow:
1. Start first session
2. Exit session (session data should be saved)
3. Resume session with --resume flag
4. Verify no credential prompts appear
5. Verify Claude responds to new prompts

Expected:
- Session data is saved after first session
- Resume works without credential prompts
- Can interact with Claude after resume
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


def test_basic_resume_works(coi_binary, cleanup_containers, workspace_dir):
    """Test that basic resume works without credential prompts."""

    # First session - just start and exit to create a session
    child = spawn_coi(coi_binary, ["shell", "--tmux=false"], cwd=workspace_dir)

    wait_for_container_ready(child)
    wait_for_prompt(child)

    with with_live_screen(child) as monitor:
        time.sleep(2)
        # Exit quickly - we just need to create a session
        clean_exit = exit_claude(child)
        wait_for_container_deletion()

    assert_clean_exit(clean_exit, child)

    # Give a moment for session to be fully saved
    time.sleep(3)

    # Second session - resume with --resume flag (auto-detect latest)
    child2 = spawn_coi(coi_binary, ["shell", "--tmux=false", "--resume"], cwd=workspace_dir)

    wait_for_container_ready(child2)
    # Give extra time for Claude to load from restored session
    time.sleep(5)
    wait_for_prompt(child2)

    with with_live_screen(child2) as monitor2:
        time.sleep(2)

        # Capture initial screen - should NOT have credential prompts
        initial_output = monitor2.get_current_screen()

        # Ask Claude a simple question to verify it's working
        send_prompt(child2, "Say OK")
        responded = wait_for_text_in_monitor(monitor2, "OK", timeout=30)

        # Exit Claude
        clean_exit2 = exit_claude(child2)
        wait_for_container_deletion()

    # Verify resume worked - no credential prompts
    assert "Choose the text style" not in initial_output, (
        "Resume failed: 'Choose the text style' prompt appeared"
    )
    assert "No, exit" not in initial_output and "Yes, I accept" not in initial_output, (
        "Resume failed: Bypass permissions acceptance prompt appeared"
    )
    assert responded, "Resume failed: Claude did not respond to prompt"
    assert_clean_exit(clean_exit2, child2)
