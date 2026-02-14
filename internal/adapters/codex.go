package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pelletier/go-toml/v2"

	"github.com/sarjann/mcper/internal/fsutil"
	"github.com/sarjann/mcper/internal/model"
	"github.com/sarjann/mcper/internal/paths"
)

type CodexAdapter struct {
	path      string
	backupDir string
}

func NewCodexAdapter() (*CodexAdapter, error) {
	p, err := paths.ExpandHome("~/.codex/config.toml")
	if err != nil {
		return nil, err
	}
	backupDir, err := paths.BackupDir()
	if err != nil {
		return nil, err
	}
	return &CodexAdapter{path: p, backupDir: backupDir}, nil
}

func (a *CodexAdapter) Name() string { return model.TargetCodex }
func (a *CodexAdapter) Path() string { return a.path }

func (a *CodexAdapter) UpsertServers(ctx context.Context, servers map[string]model.MCPServerSpec) error {
	_ = ctx
	raw, err := a.readRaw()
	if err != nil {
		return err
	}

	mcp, ok := toMap(raw["mcp_servers"])
	if !ok {
		mcp = map[string]any{}
	}
	for name, spec := range servers {
		mcp[name] = serverSpecToConfig(spec)
	}
	raw["mcp_servers"] = mcp
	return a.writeRaw(raw)
}

func (a *CodexAdapter) RemoveServers(ctx context.Context, names []string) error {
	_ = ctx
	raw, err := a.readRaw()
	if err != nil {
		return err
	}
	mcp, ok := toMap(raw["mcp_servers"])
	if !ok {
		return nil
	}
	for _, name := range names {
		delete(mcp, name)
	}
	raw["mcp_servers"] = mcp
	return a.writeRaw(raw)
}

func (a *CodexAdapter) ListServers(ctx context.Context) (map[string]model.MCPServerSpec, error) {
	_ = ctx
	raw, err := a.readRaw()
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]model.MCPServerSpec{}, nil
		}
		return nil, err
	}
	mcp, ok := toMap(raw["mcp_servers"])
	if !ok {
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

func (a *CodexAdapter) readRaw() (map[string]any, error) {
	data, err := os.ReadFile(a.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read codex config: %w", err)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode codex config TOML: %w", err)
	}
	if raw == nil {
		raw = map[string]any{}
	}
	return raw, nil
}

func (a *CodexAdapter) writeRaw(raw map[string]any) error {
	data, err := toml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("encode codex config TOML: %w", err)
	}
	if err := toml.Unmarshal(data, &map[string]any{}); err != nil {
		return fmt.Errorf("validate generated codex TOML: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		return fmt.Errorf("create codex config dir: %w", err)
	}
	if _, err := fsutil.BackupFile(a.path, a.backupDir); err != nil {
		return err
	}
	if err := fsutil.AtomicWriteFile(a.path, data, 0o600); err != nil {
		return fmt.Errorf("write codex config: %w", err)
	}
	return nil
}

func serverSpecToConfig(spec model.MCPServerSpec) map[string]any {
	out := map[string]any{}
	if spec.Transport == model.ServerTransportHTTP {
		out["url"] = spec.URL
	} else {
		out["command"] = spec.Command
		if len(spec.Args) > 0 {
			out["args"] = spec.Args
		}
	}
	if len(spec.EnvRequired) > 0 {
		env := append([]string{}, spec.EnvRequired...)
		sort.Strings(env)
		out["env_vars"] = env
	}
	return out
}

func configToServerSpec(cfg map[string]any) model.MCPServerSpec {
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
	s.EnvRequired = toStringSlice(cfg["env_vars"])
	return s
}

func toMap(v any) (map[string]any, bool) {
	if v == nil {
		return nil, false
	}
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	if m, ok := v.(map[string]interface{}); ok {
		out := map[string]any{}
		for k, vv := range m {
			out[k] = vv
		}
		return out, true
	}
	return nil, false
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		if sitems, ok := v.([]string); ok {
			return append([]string{}, sitems...)
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if ok {
			out = append(out, s)
		}
	}
	return out
}
