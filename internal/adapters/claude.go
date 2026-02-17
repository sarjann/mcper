package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarjann/mcper/internal/fsutil"
	"github.com/sarjann/mcper/internal/model"
	"github.com/sarjann/mcper/internal/paths"
)

type ClaudeAdapter struct {
	path      string
	backupDir string
}

func NewClaudeAdapter() (*ClaudeAdapter, error) {
	backupDir, err := paths.BackupDir()
	if err != nil {
		return nil, err
	}
	p, err := detectClaudeSettingsPath()
	if err != nil {
		return nil, err
	}
	return &ClaudeAdapter{path: p, backupDir: backupDir}, nil
}

func (a *ClaudeAdapter) Name() string { return model.TargetClaude }
func (a *ClaudeAdapter) Path() string { return a.path }

func detectClaudeSettingsPath() (string, error) {
	local, err := paths.ExpandHome("~/.claude/settings.local.json")
	if err != nil {
		return "", err
	}
	global, err := paths.ExpandHome("~/.claude/settings.json")
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}
	if _, err := os.Stat(global); err == nil {
		return global, nil
	}
	return local, nil
}

func (a *ClaudeAdapter) UpsertServers(ctx context.Context, servers map[string]model.MCPServerSpec) error {
	_ = ctx
	raw, err := a.readRaw()
	if err != nil {
		return err
	}
	mcp := claudeMCPServers(raw)
	for name, spec := range servers {
		mcp[name] = serverSpecToConfig(spec)
	}
	raw["mcpServers"] = mcp
	delete(raw, "mcp_servers")
	return a.writeRaw(raw)
}

func (a *ClaudeAdapter) RemoveServers(ctx context.Context, names []string) error {
	_ = ctx
	raw, err := a.readRaw()
	if err != nil {
		return err
	}
	mcp := claudeMCPServers(raw)
	if len(mcp) == 0 {
		return nil
	}
	for _, name := range names {
		delete(mcp, name)
	}
	raw["mcpServers"] = mcp
	delete(raw, "mcp_servers")
	return a.writeRaw(raw)
}

func (a *ClaudeAdapter) ListServers(ctx context.Context) (map[string]model.MCPServerSpec, error) {
	_ = ctx
	raw, err := a.readRaw()
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]model.MCPServerSpec{}, nil
		}
		return nil, err
	}
	mcp := claudeMCPServers(raw)
	if len(mcp) == 0 {
		return map[string]model.MCPServerSpec{}, nil
	}
	result := make(map[string]model.MCPServerSpec, len(mcp))
	for name, cfgRaw := range mcp {
		cfgMap, ok := toMap(cfgRaw)
		if !ok {
			continue
		}
		result[name] = configToServerSpec(cfgMap)
	}
	return result, nil
}

// claudeMCPServers reads servers from mcpServers (preferred) or mcp_servers (legacy).
func claudeMCPServers(raw map[string]any) map[string]any {
	if mcp, ok := toMap(raw["mcpServers"]); ok {
		return mcp
	}
	if mcp, ok := toMap(raw["mcp_servers"]); ok {
		return mcp
	}
	return map[string]any{}
}

func (a *ClaudeAdapter) readRaw() (map[string]any, error) {
	data, err := os.ReadFile(a.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read claude settings: %w", err)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode claude settings JSON: %w", err)
	}
	if raw == nil {
		raw = map[string]any{}
	}
	return raw, nil
}

func (a *ClaudeAdapter) writeRaw(raw map[string]any) error {
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("encode claude settings JSON: %w", err)
	}
	if err := json.Unmarshal(data, &map[string]any{}); err != nil {
		return fmt.Errorf("validate generated claude JSON: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		return fmt.Errorf("create claude settings dir: %w", err)
	}
	if _, err := fsutil.BackupFile(a.path, a.backupDir); err != nil {
		return err
	}
	payload := append(data, '\n')
	if err := fsutil.AtomicWriteFile(a.path, payload, 0o600); err != nil {
		return fmt.Errorf("write claude settings: %w", err)
	}
	return nil
}
