package service

import (
	"testing"

	"github.com/sarjann/mcper/internal/adapters"
	"github.com/sarjann/mcper/internal/model"
)

func TestResolveTargets(t *testing.T) {
	m := &Manager{
		adapters: map[string]adapters.Adapter{
			model.TargetCodex:  newStub("codex", nil),
			model.TargetClaude: newStub("claude", nil),
		},
	}
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

func TestResolveTargets_All(t *testing.T) {
	m := &Manager{
		adapters: map[string]adapters.Adapter{
			model.TargetCodex:   newStub("codex", nil),
			model.TargetClaude:  newStub("claude", nil),
			model.TargetCursor:  newStub("cursor", nil),
		},
	}
	targets, err := m.resolveTargets("all")
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
	// Should be sorted
	if targets[0] != "claude" || targets[1] != "codex" || targets[2] != "cursor" {
		t.Errorf("expected sorted [claude codex cursor], got %v", targets)
	}
}

func TestResolveTargets_Empty(t *testing.T) {
	m := &Manager{
		adapters: map[string]adapters.Adapter{},
	}
	_, err := m.resolveTargets("all")
	if err == nil {
		t.Fatal("expected error for empty adapters")
	}
}
