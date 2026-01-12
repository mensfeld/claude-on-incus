# Building Test Images with Fake Claude

## Better Approach: Install Fake Claude in Container

Instead of modifying PATH on the host, we should build a test image with fake Claude pre-installed.

### Option 1: Build Custom Image with Fake Claude

```bash
# Create build script that installs fake Claude
cat > testdata/fake-claude/install.sh << 'EOF'
#!/bin/bash
set -e

# Install fake Claude as /usr/local/bin/claude
cp /workspace/testdata/fake-claude/claude /usr/local/bin/claude
chmod +x /usr/local/bin/claude

echo "Fake Claude installed successfully"
EOF

# Build custom image
coi build custom coi-test-fake-claude \
    --script testdata/fake-claude/install.sh

# Now tests can use this image
coi shell --image coi-test-fake-claude
```

### Option 2: Mount Fake Claude via .claude Directory

Create a `.claude` directory structure that includes the fake Claude binary:

```bash
# In test setup
test_claude_dir="${workspace}/.claude"
mkdir -p "${test_claude_dir}/bin"
cp testdata/fake-claude/claude "${test_claude_dir}/bin/claude"
chmod +x "${test_claude_dir}/bin/claude"

# COI will mount this and container will use it
coi shell --workspace "${workspace}"
```

### Option 3: Use Fixture to Prepare Test Image (RECOMMENDED)

```python
@pytest.fixture(scope="session")
def fake_claude_image(coi_binary):
    """Build a test image with fake Claude pre-installed."""

    image_name = "coi-test-fake-claude"

    # Check if image already exists
    result = subprocess.run(
        [coi_binary, "image", "exists", image_name],
        capture_output=True
    )

    if result.returncode == 0:
        return image_name  # Already built

    # Build image with fake Claude
    script_path = "testdata/fake-claude/install.sh"
    result = subprocess.run(
        [coi_binary, "build", "custom", image_name,
         "--script", script_path],
        capture_output=True,
        text=True,
        timeout=300
    )

    if result.returncode != 0:
        pytest.skip(f"Could not build fake Claude image: {result.stderr}")

    return image_name


# In tests
def test_with_fake_claude(coi_binary, fake_claude_image, workspace_dir):
    """Test using pre-built image with fake Claude."""

    child = spawn_coi(
        coi_binary,
        ["shell", "--image", fake_claude_image],
        cwd=workspace_dir
    )

    # Fake Claude is already in the container!
    wait_for_prompt(child)
    # ...
```

## Why This is Better

### Current Approach (PATH modification):
- ❌ Modifies host environment
- ❌ Doesn't test actual installation
- ❌ Different from production setup

### New Approach (Image with fake Claude):
- ✅ Fake Claude inside container (realistic)
- ✅ Tests actual mount/installation mechanisms
- ✅ Closer to production setup
- ✅ No PATH manipulation needed
- ✅ Reusable across tests (build once)

## Implementation

The recommended approach is **Option 3** with a session-scoped fixture that builds the image once and reuses it for all tests.
