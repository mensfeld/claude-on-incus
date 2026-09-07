package container

import "testing"

func TestStatusIsRunning(t *testing.T) {
	for _, s := range []string{"Running", "RUNNING", "running"} {
		if !StatusIsRunning(s) {
			t.Errorf("StatusIsRunning(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"Stopped", "STOPPED", "Frozen", "", "run"} {
		if StatusIsRunning(s) {
			t.Errorf("StatusIsRunning(%q) = true, want false", s)
		}
	}
}

func TestStatusIsStopped(t *testing.T) {
	for _, s := range []string{"Stopped", "STOPPED", "stopped"} {
		if !StatusIsStopped(s) {
			t.Errorf("StatusIsStopped(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"Running", "RUNNING", "Frozen", ""} {
		if StatusIsStopped(s) {
			t.Errorf("StatusIsStopped(%q) = true, want false", s)
		}
	}
}
