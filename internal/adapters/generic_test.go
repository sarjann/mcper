package adapters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarjann/mcper/internal/model"
)

func TestGenericJSONAdapter_UpsertAndList(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	adapter := NewGenericJSONAdapter("test", configPath, dir, []string{"mcpServers"}, nil, nil)

	ctx := context.Background()
	servers := map[string]model.MCPServerSpec{
		"vercel": {
			Transport: model.ServerTransportSTDIO,
			Command:   "npx",
			Args:      []string{"-y", "@vercel/mcp"},
		},
	}

	if err := adapter.UpsertServers(ctx, servers); err != nil {
		t.Fatalf("UpsertServers: %v", err)
	}

	// Verify file was created with correct structure
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	mcpServers, ok := raw["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers key, got: %v", raw)
	}
	if _, ok := mcpServers["vercel"]; !ok {
		t.Fatal("expected vercel server in config")
	}

	// List servers
	listed, err := adapter.ListServers(ctx)
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 server, got %d", len(listed))
	}
	spec := listed["vercel"]
	if spec.Command != "npx" {
		t.Errorf("expected command 'npx', got %q", spec.Command)
	}
}

func TestGenericJSONAdapter_NestedKeys(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "settings.json")

	// Test nested key path support (e.g., {"mcp": {"servers": {...}}})
	adapter := NewGenericJSONAdapter("nested", configPath, dir, []string{"mcp", "servers"}, nil, nil)

	ctx := context.Background()
	servers := map[string]model.MCPServerSpec{
		"test-server": {
			Transport: model.ServerTransportSTDIO,
			Command:   "myserver",
		},
	}

	if err := adapter.UpsertServers(ctx, servers); err != nil {
		t.Fatalf("UpsertServers: %v", err)
	}

	// Verify nested structure
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	mcp, ok := raw["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcp key, got: %v", raw)
	}
	servers2, ok := mcp["servers"].(map[string]any)
	if !ok {
		t.Fatalf("expected servers key, got: %v", mcp)
	}
	if _, ok := servers2["test-server"]; !ok {
		t.Fatal("expected test-server in config")
	}

	// List should work
	listed, err := adapter.ListServers(ctx)
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 server, got %d", len(listed))
	}
}

func TestGenericJSONAdapter_RemoveServers(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	adapter := NewGenericJSONAdapter("test", configPath, dir, []string{"mcpServers"}, nil, nil)
	ctx := context.Background()

	servers := map[string]model.MCPServerSpec{
		"a": {Transport: model.ServerTransportSTDIO, Command: "a"},
		"b": {Transport: model.ServerTransportSTDIO, Command: "b"},
	}
	if err := adapter.UpsertServers(ctx, servers); err != nil {
		t.Fatalf("UpsertServers: %v", err)
	}

	if err := adapter.RemoveServers(ctx, []string{"a"}); err != nil {
		t.Fatalf("RemoveServers: %v", err)
	}

	listed, err := adapter.ListServers(ctx)
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 server after remove, got %d", len(listed))
	}
	if _, ok := listed["b"]; !ok {
		t.Error("expected server 'b' to remain")
	}
}

func TestGenericJSONAdapter_PreservesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	// Pre-populate with existing config
	existing := map[string]any{
		"theme":      "dark",
		"mcpServers": map[string]any{},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(configPath, data, 0o600)

	adapter := NewGenericJSONAdapter("test", configPath, dir, []string{"mcpServers"}, nil, nil)
	ctx := context.Background()

	servers := map[string]model.MCPServerSpec{
		"vercel": {Transport: model.ServerTransportSTDIO, Command: "npx"},
	}
	if err := adapter.UpsertServers(ctx, servers); err != nil {
		t.Fatalf("UpsertServers: %v", err)
	}

	// Verify existing keys are preserved
	data, _ = os.ReadFile(configPath)
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if raw["theme"] != "dark" {
		t.Errorf("expected theme to be preserved, got %v", raw["theme"])
	}
}

func TestGenericJSONAdapter_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	adapter := NewGenericJSONAdapter("test", configPath, dir, []string{"mcpServers"}, nil, nil)
	ctx := context.Background()

	// List on non-existent file should return empty
	listed, err := adapter.ListServers(ctx)
	if err != nil {
		t.Fatalf("ListServers on missing file: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("expected 0 servers, got %d", len(listed))
	}
}

func TestZedSpecToConfig(t *testing.T) {
	spec := model.MCPServerSpec{
		Transport: model.ServerTransportSTDIO,
		Command:   "npx",
		Args:      []string{"-y", "@vercel/mcp"},
	}
	config := zedSpecToConfig(spec)

	cmd, ok := config["command"].(string)
	if !ok {
		t.Fatal("expected command to be a string (flat format)")
	}
	if cmd != "npx" {
		t.Errorf("expected command 'npx', got %v", cmd)
	}
	args := toStringSlice(config["args"])
	if len(args) != 2 || args[0] != "-y" || args[1] != "@vercel/mcp" {
		t.Errorf("expected args [-y @vercel/mcp], got %v", args)
	}
}

func TestZedSpecToConfig_NoArgs(t *testing.T) {
	spec := model.MCPServerSpec{
		Transport: model.ServerTransportSTDIO,
		Command:   "myserver",
	}
	config := zedSpecToConfig(spec)

	if config["command"] != "myserver" {
		t.Errorf("expected command 'myserver', got %v", config["command"])
	}
	if _, ok := config["args"]; ok {
		t.Error("expected no args key when args is empty")
	}
}

func TestZedConfigToSpec(t *testing.T) {
	cfg := map[string]any{
		"command": "npx",
		"args":    []any{"-y", "@vercel/mcp"},
	}
	spec := zedConfigToSpec(cfg)
	if spec.Command != "npx" {
		t.Errorf("expected command 'npx', got %q", spec.Command)
	}
	if len(spec.Args) != 2 || spec.Args[0] != "-y" {
		t.Errorf("expected args [-y @vercel/mcp], got %v", spec.Args)
	}
}

func TestZedConfigToSpec_Legacy(t *testing.T) {
	cfg := map[string]any{
		"command": map[string]any{
			"path": "npx",
			"args": []any{"-y", "@vercel/mcp"},
		},
	}
	spec := zedConfigToSpec(cfg)
	if spec.Command != "npx" {
		t.Errorf("expected command 'npx', got %q", spec.Command)
	}
	if len(spec.Args) != 2 || spec.Args[0] != "-y" {
		t.Errorf("expected args [-y @vercel/mcp], got %v", spec.Args)
	}
}

func TestClientLabels(t *testing.T) {
	labels := ClientLabels()
	if labels["claude"] != "Claude Code" {
		t.Errorf("expected 'Claude Code' for claude, got %q", labels["claude"])
	}
	if labels["cursor"] != "Cursor" {
		t.Errorf("expected 'Cursor' for cursor, got %q", labels["cursor"])
	}
	if labels["zed"] != "Zed" {
		t.Errorf("expected 'Zed' for zed, got %q", labels["zed"])
	}
}

func TestGetNestedMap(t *testing.T) {
	raw := map[string]any{
		"mcp": map[string]any{
			"servers": map[string]any{
				"test": map[string]any{"command": "echo"},
			},
		},
	}

	result := getNestedMap(raw, []string{"mcp", "servers"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if _, ok := result["test"]; !ok {
		t.Error("expected 'test' key in result")
	}

	// Missing key
	result = getNestedMap(raw, []string{"nonexistent"})
	if result != nil {
		t.Error("expected nil for missing key")
	}
}

func TestSetNestedMap(t *testing.T) {
	raw := map[string]any{}
	value := map[string]any{"test": map[string]any{"command": "echo"}}

	setNestedMap(raw, []string{"mcp", "servers"}, value)

	mcp, ok := raw["mcp"].(map[string]any)
	if !ok {
		t.Fatal("expected mcp key to be created")
	}
	servers, ok := mcp["servers"].(map[string]any)
	if !ok {
		t.Fatal("expected servers key to be created")
	}
	if _, ok := servers["test"]; !ok {
		t.Error("expected test server in result")
	}
}

func TestOpencodeSpecToConfig(t *testing.T) {
	spec := model.MCPServerSpec{
		Transport: model.ServerTransportSTDIO,
		Command:   "npx",
		Args:      []string{"-y", "@vercel/mcp"},
	}
	config := opencodeSpecToConfig(spec)

	if config["type"] != "local" {
		t.Errorf("expected type 'local', got %v", config["type"])
	}
	cmd := toStringSlice(config["command"])
	if len(cmd) != 3 || cmd[0] != "npx" || cmd[1] != "-y" || cmd[2] != "@vercel/mcp" {
		t.Errorf("expected command [npx -y @vercel/mcp], got %v", cmd)
	}
}

func TestOpencodeSpecToConfig_HTTP(t *testing.T) {
	spec := model.MCPServerSpec{
		Transport: model.ServerTransportHTTP,
		URL:       "https://mcp.example.com",
	}
	config := opencodeSpecToConfig(spec)

	if config["type"] != "remote" {
		t.Errorf("expected type 'remote', got %v", config["type"])
	}
	if config["url"] != "https://mcp.example.com" {
		t.Errorf("expected url, got %v", config["url"])
	}
}

func TestOpencodeConfigToSpec(t *testing.T) {
	cfg := map[string]any{
		"type":    "local",
		"command": []any{"npx", "-y", "@vercel/mcp"},
	}
	spec := opencodeConfigToSpec(cfg)
	if spec.Command != "npx" {
		t.Errorf("expected command 'npx', got %q", spec.Command)
	}
	if len(spec.Args) != 2 || spec.Args[0] != "-y" {
		t.Errorf("expected args [-y @vercel/mcp], got %v", spec.Args)
	}
}
