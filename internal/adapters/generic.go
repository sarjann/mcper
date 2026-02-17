package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarjann/mcper/internal/fsutil"
	"github.com/sarjann/mcper/internal/model"
)

// SpecToConfig converts an MCPServerSpec to a client's JSON config format.
type SpecToConfig func(model.MCPServerSpec) map[string]any

// ConfigToSpec converts a client's JSON config to an MCPServerSpec.
type ConfigToSpec func(map[string]any) model.MCPServerSpec

// GenericJSONAdapter handles MCP server configuration for JSON-based AI clients.
type GenericJSONAdapter struct {
	name       string
	path       string
	backupDir  string
	serverKeys []string     // JSON key path to servers section (e.g., ["mcpServers"])
	toConfig   SpecToConfig // converts spec → client config format
	fromConfig ConfigToSpec // converts client config → spec
}

func NewGenericJSONAdapter(name, path, backupDir string, serverKeys []string, toConfig SpecToConfig, fromConfig ConfigToSpec) *GenericJSONAdapter {
	if toConfig == nil {
		toConfig = standardSpecToConfig
	}
	if fromConfig == nil {
		fromConfig = standardConfigToSpec
	}
	return &GenericJSONAdapter{
		name:       name,
		path:       path,
		backupDir:  backupDir,
		serverKeys: serverKeys,
		toConfig:   toConfig,
		fromConfig: fromConfig,
	}
}

func (a *GenericJSONAdapter) Name() string { return a.name }
func (a *GenericJSONAdapter) Path() string { return a.path }

func (a *GenericJSONAdapter) UpsertServers(ctx context.Context, servers map[string]model.MCPServerSpec) error {
	_ = ctx
	raw, err := a.readRaw()
	if err != nil {
		return err
	}
	mcp := getNestedMap(raw, a.serverKeys)
	if mcp == nil {
		mcp = map[string]any{}
	}
	for name, spec := range servers {
		mcp[name] = a.toConfig(spec)
	}
	setNestedMap(raw, a.serverKeys, mcp)
	return a.writeRaw(raw)
}

func (a *GenericJSONAdapter) RemoveServers(ctx context.Context, names []string) error {
	_ = ctx
	raw, err := a.readRaw()
	if err != nil {
		return err
	}
	mcp := getNestedMap(raw, a.serverKeys)
	if mcp == nil {
		return nil
	}
	for _, name := range names {
		delete(mcp, name)
	}
	setNestedMap(raw, a.serverKeys, mcp)
	return a.writeRaw(raw)
}

func (a *GenericJSONAdapter) ListServers(ctx context.Context) (map[string]model.MCPServerSpec, error) {
	_ = ctx
	raw, err := a.readRaw()
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]model.MCPServerSpec{}, nil
		}
		return nil, err
	}
	mcp := getNestedMap(raw, a.serverKeys)
	if mcp == nil {
		return map[string]model.MCPServerSpec{}, nil
	}
	result := make(map[string]model.MCPServerSpec, len(mcp))
	for name, cfgRaw := range mcp {
		cfgMap, ok := toMap(cfgRaw)
		if !ok {
			continue
		}
		result[name] = a.fromConfig(cfgMap)
	}
	return result, nil
}

func (a *GenericJSONAdapter) readRaw() (map[string]any, error) {
	data, err := os.ReadFile(a.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read %s config: %w", a.name, err)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode %s config JSON: %w", a.name, err)
	}
	if raw == nil {
		raw = map[string]any{}
	}
	return raw, nil
}

func (a *GenericJSONAdapter) writeRaw(raw map[string]any) error {
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s config JSON: %w", a.name, err)
	}
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		return fmt.Errorf("create %s config dir: %w", a.name, err)
	}
	if _, statErr := os.Stat(a.path); statErr == nil {
		if _, err := fsutil.BackupFile(a.path, a.backupDir); err != nil {
			return err
		}
	}
	payload := append(data, '\n')
	if err := fsutil.AtomicWriteFile(a.path, payload, 0o600); err != nil {
		return fmt.Errorf("write %s config: %w", a.name, err)
	}
	return nil
}

// standardSpecToConfig converts a server spec to the standard MCP format
// used by most clients (Claude Desktop, Cursor, Gemini CLI, etc.).
func standardSpecToConfig(spec model.MCPServerSpec) map[string]any {
	out := map[string]any{}
	if spec.Transport == model.ServerTransportHTTP {
		out["url"] = spec.URL
	} else {
		out["command"] = spec.Command
		if len(spec.Args) > 0 {
			out["args"] = spec.Args
		}
	}
	return out
}

// standardConfigToSpec converts a standard MCP config to a server spec.
func standardConfigToSpec(cfg map[string]any) model.MCPServerSpec {
	s := model.MCPServerSpec{}
	if url, ok := cfg["url"].(string); ok && url != "" {
		s.Transport = model.ServerTransportHTTP
		s.URL = url
	} else {
		s.Transport = model.ServerTransportSTDIO
		if cmd, ok := cfg["command"].(string); ok {
			s.Command = cmd
		}
		s.Args = toStringSlice(cfg["args"])
	}
	return s
}

// getNestedMap navigates a JSON structure by key path.
func getNestedMap(raw map[string]any, keys []string) map[string]any {
	current := raw
	for i, key := range keys {
		val, ok := current[key]
		if !ok {
			return nil
		}
		m, ok := toMap(val)
		if !ok {
			return nil
		}
		if i == len(keys)-1 {
			return m
		}
		current = m
	}
	return nil
}

// setNestedMap sets a value at a nested key path, creating intermediate maps as needed.
func setNestedMap(raw map[string]any, keys []string, value map[string]any) {
	current := raw
	for i, key := range keys {
		if i == len(keys)-1 {
			current[key] = value
			return
		}
		next, ok := toMap(current[key])
		if !ok {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
}
