package session

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mensfeld/code-on-incus/internal/container"
	"github.com/mensfeld/code-on-incus/internal/tool"
)

// fakeDirProbe is a minimal containerCommandRunner: it records the command and
// returns success/failure to stand in for the populated-check's exit status.
type fakeDirProbe struct {
	populated bool
	gotCmd    string
}

func (f *fakeDirProbe) ExecCommand(cmd string, _ container.ExecCommandOptions) (string, error) {
	f.gotCmd = cmd
	if f.populated {
		return "", nil
	}
	return "", fmt.Errorf("exit status 1") // empty/missing dir
}

// toolConfigDirPopulated must check for CONTENT (via `ls -A`), not mere dir
// existence — the base image pre-creates the config dirs empty, so a `test -d`
// would wrongly report every tool already-configured and skip reuse-seeding
// (#708 follow-up).
func TestToolConfigDirPopulated(t *testing.T) {
	c, err := tool.Get("claude")
	if err != nil {
		t.Fatalf("tool.Get: %v", err)
	}
	tcf, ok := c.(tool.ToolWithConfigDirFiles)
	if !ok {
		t.Fatal("claude should implement ToolWithConfigDirFiles")
	}

	t.Run("populated", func(t *testing.T) {
		fp := &fakeDirProbe{populated: true}
		if !toolConfigDirPopulated(fp, "/home/code", tcf) {
			t.Error("want populated=true when the dir has content")
		}
		// It must inspect contents (ls -A), not just `test -d`.
		if !strings.Contains(fp.gotCmd, "ls -A /home/code/.claude") {
			t.Errorf("probe must check dir contents, got %q", fp.gotCmd)
		}
		if strings.HasPrefix(fp.gotCmd, "test -d") {
			t.Errorf("probe must not be a bare `test -d` (empty pre-created dirs would fool it): %q", fp.gotCmd)
		}
	})

	t.Run("empty or missing", func(t *testing.T) {
		fa := &fakeDirProbe{populated: false}
		if toolConfigDirPopulated(fa, "/home/code", tcf) {
			t.Error("want populated=false when the dir is empty/missing")
		}
	})
}
