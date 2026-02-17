package adapters

import (
	"os"
	"runtime"

	"github.com/sarjann/mcper/internal/model"
	"github.com/sarjann/mcper/internal/paths"
)

type clientDef struct {
	target     string
	label      string
	detectDirs []string // dirs with ~ prefix to check for detection
	configPath string   // config file path with ~ prefix
	serverKeys []string // JSON key path to servers section
	toConfig   SpecToConfig
	fromConfig ConfigToSpec
	customNew  func() (Adapter, error) // for adapters with custom logic (Claude Code, Codex)
}

func (c clientDef) isDetected() bool {
	for _, dir := range c.detectDirs {
		expanded, err := paths.ExpandHome(dir)
		if err != nil {
			continue
		}
		if fi, err := os.Stat(expanded); err == nil && fi.IsDir() {
			return true
		}
	}
	return false
}

func (c clientDef) createAdapter(backupDir string) (Adapter, error) {
	if c.customNew != nil {
		return c.customNew()
	}
	expanded, err := paths.ExpandHome(c.configPath)
	if err != nil {
		return nil, err
	}
	return NewGenericJSONAdapter(c.target, expanded, backupDir, c.serverKeys, c.toConfig, c.fromConfig), nil
}

func knownClients() []clientDef {
	return []clientDef{
		{
			target:     model.TargetClaude,
			label:      "Claude Code",
			detectDirs: []string{"~/.claude"},
			customNew:  func() (Adapter, error) { return NewClaudeAdapter() },
		},
		{
			target:     model.TargetCodex,
			label:      "Codex CLI",
			detectDirs: []string{"~/.codex"},
			customNew:  func() (Adapter, error) { return NewCodexAdapter() },
		},
		{
			target:     model.TargetClaudeDesktop,
			label:      "Claude Desktop",
			detectDirs: claudeDesktopDetectDirs(),
			configPath: claudeDesktopConfigPath(),
			serverKeys: []string{"mcpServers"},
		},
		{
			target:     model.TargetCursor,
			label:      "Cursor",
			detectDirs: []string{"~/.cursor"},
			configPath: "~/.cursor/mcp.json",
			serverKeys: []string{"mcpServers"},
		},
		{
			target:     model.TargetVSCode,
			label:      "VS Code",
			detectDirs: vscodeDetectDirs(),
			configPath: vscodeConfigPath(),
			serverKeys: []string{"mcp", "servers"},
		},
		{
			target:     model.TargetGemini,
			label:      "Gemini CLI",
			detectDirs: []string{"~/.gemini"},
			configPath: "~/.gemini/settings.json",
			serverKeys: []string{"mcpServers"},
		},
		// Goose uses YAML config (~/.config/goose/config.yaml) which requires
		// an additional dependency. Support can be added in the future.
		{
			target:     model.TargetZed,
			label:      "Zed",
			detectDirs: zedDetectDirs(),
			configPath: zedConfigPath(),
			serverKeys: []string{"context_servers"},
			toConfig:   zedSpecToConfig,
			fromConfig: zedConfigToSpec,
		},
		{
			target:     model.TargetOpenCode,
			label:      "OpenCode",
			detectDirs: []string{"~/.config/opencode"},
			configPath: "~/.config/opencode/config.json",
			serverKeys: []string{"mcpServers"},
		},
	}
}

// DetectedAdapters returns adapters for all AI clients found on the system.
func DetectedAdapters() (map[string]Adapter, error) {
	backupDir, err := paths.BackupDir()
	if err != nil {
		return nil, err
	}
	result := make(map[string]Adapter)
	for _, client := range knownClients() {
		if !client.isDetected() {
			continue
		}
		adapter, err := client.createAdapter(backupDir)
		if err != nil {
			continue
		}
		result[client.target] = adapter
	}
	return result, nil
}

// ClientLabels returns a map of target name to human-readable label for all known clients.
func ClientLabels() map[string]string {
	labels := make(map[string]string)
	for _, c := range knownClients() {
		labels[c.target] = c.label
	}
	return labels
}

// Platform-specific paths

func claudeDesktopDetectDirs() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"~/Library/Application Support/Claude"}
	default:
		return []string{"~/.config/Claude"}
	}
}

func claudeDesktopConfigPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "~/Library/Application Support/Claude/claude_desktop_config.json"
	default:
		return "~/.config/Claude/claude_desktop_config.json"
	}
}

func vscodeDetectDirs() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"~/Library/Application Support/Code"}
	default:
		return []string{"~/.config/Code"}
	}
}

func vscodeConfigPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "~/Library/Application Support/Code/User/settings.json"
	default:
		return "~/.config/Code/User/settings.json"
	}
}

func zedDetectDirs() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"~/Library/Application Support/Zed"}
	default:
		return []string{"~/.config/zed"}
	}
}

func zedConfigPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "~/Library/Application Support/Zed/settings.json"
	default:
		return "~/.config/zed/settings.json"
	}
}

// Zed-specific converters

func zedSpecToConfig(spec model.MCPServerSpec) map[string]any {
	if spec.Transport == model.ServerTransportHTTP {
		return map[string]any{"url": spec.URL}
	}
	return map[string]any{
		"command": map[string]any{
			"path": spec.Command,
			"args": spec.Args,
		},
	}
}

func zedConfigToSpec(cfg map[string]any) model.MCPServerSpec {
	if url, ok := cfg["url"].(string); ok && url != "" {
		return model.MCPServerSpec{Transport: model.ServerTransportHTTP, URL: url}
	}
	s := model.MCPServerSpec{Transport: model.ServerTransportSTDIO}
	if cmdMap, ok := toMap(cfg["command"]); ok {
		if path, ok := cmdMap["path"].(string); ok {
			s.Command = path
		}
		s.Args = toStringSlice(cmdMap["args"])
	}
	return s
}
