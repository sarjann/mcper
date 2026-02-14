package service

import "testing"

func TestResolveTargets(t *testing.T) {
	m := &Manager{}
	targets, err := m.resolveTargets("codex,claude")
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}

	if _, err := m.resolveTargets("unknown"); err == nil {
		t.Fatalf("expected error for unknown target")
	}
}
