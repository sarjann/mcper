package adapters

import (
	"context"
	"testing"

	"github.com/sarjann/mcper/internal/model"
)

func TestCodexAdapterUpsertAndRemove(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	a, err := NewCodexAdapter()
	if err != nil {
		t.Fatalf("NewCodexAdapter failed: %v", err)
	}

	servers := map[string]model.MCPServerSpec{
		"demo": {
			Transport:   model.ServerTransportSTDIO,
			Command:     "npx",
			Args:        []string{"-y", "demo-mcp"},
			EnvRequired: []string{"DEMO_KEY"},
		},
	}

	if err := a.UpsertServers(context.Background(), servers); err != nil {
		t.Fatalf("UpsertServers failed: %v", err)
	}

	listed, err := a.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers failed: %v", err)
	}
	if _, ok := listed["demo"]; !ok {
		t.Fatalf("expected demo server in codex config")
	}

	if err := a.RemoveServers(context.Background(), []string{"demo"}); err != nil {
		t.Fatalf("RemoveServers failed: %v", err)
	}
	listed, err = a.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers failed: %v", err)
	}
	if _, ok := listed["demo"]; ok {
		t.Fatalf("expected demo server to be removed")
	}
}
