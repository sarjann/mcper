package model

import "time"

const (
	StateVersion          = 1
	TrustModeCosign       = "cosign"
	TrustModeHash         = "hash"
	SourceTypeTap         = "tap"
	SourceTypeDirect      = "direct"
	TargetCodex           = "codex"
	TargetClaude          = "claude"
	TargetAll             = "all"
	DefaultTapName        = "official"
	DefaultTapURL         = "https://github.com/sarjann/mcp-registry.git"
	DefaultTapDescription = "Official mcper registry"
	ServerTransportSTDIO  = "stdio"
	ServerTransportHTTP   = "http"
)

type State struct {
	Version              int                         `json:"version"`
	Taps                 map[string]TapConfig        `json:"taps"`
	Installed            map[string]InstalledPackage `json:"installed"`
	TrustedDirectSources map[string]TrustDecision    `json:"trusted_direct_sources"`
}

type TapConfig struct {
	Name        string         `json:"name"`
	URL         string         `json:"url"`
	Description string         `json:"description,omitempty"`
	Trust       TapTrustConfig `json:"trust"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type TapTrustConfig struct {
	Mode       string             `json:"mode"`
	Identities []SigstoreIdentity `json:"identities,omitempty"`
}

type SigstoreIdentity struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}

type TrustDecision struct {
	URL       string    `json:"url"`
	Approved  bool      `json:"approved"`
	CreatedAt time.Time `json:"created_at"`
}

type InstalledPackage struct {
	Name           string    `json:"name"`
	Version        string    `json:"version"`
	Description    string    `json:"description,omitempty"`
	Source         SourceRef `json:"source"`
	ManifestDigest string    `json:"manifest_digest,omitempty"`
	Servers        []string  `json:"servers"`
	Targets        []string  `json:"targets"`
	InstalledAt    time.Time `json:"installed_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type SourceRef struct {
	Type string `json:"type"`
	Tap  string `json:"tap,omitempty"`
	URL  string `json:"url,omitempty"`
}

type RegistryIndex struct {
	SchemaVersion int                     `json:"schema_version"`
	GeneratedAt   string                  `json:"generated_at,omitempty"`
	Packages      map[string]IndexPackage `json:"packages"`
}

type IndexPackage struct {
	Description string                  `json:"description,omitempty"`
	Versions    map[string]IndexVersion `json:"versions"`
}

type IndexVersion struct {
	ManifestPath string `json:"manifest"`
	SHA256       string `json:"sha256,omitempty"`
	SigPath      string `json:"sig,omitempty"`
	CertPath     string `json:"cert,omitempty"`
}

type PackageManifest struct {
	SchemaVersion int                      `json:"schema_version"`
	Name          string                   `json:"name"`
	Version       string                   `json:"version"`
	Description   string                   `json:"description,omitempty"`
	MCPServers    map[string]MCPServerSpec `json:"mcp_servers"`
	Compatibility Compatibility            `json:"compatibility,omitempty"`
}

type Compatibility struct {
	OS        []string `json:"os,omitempty"`
	Arch      []string `json:"arch,omitempty"`
	CodexMin  string   `json:"codex_min,omitempty"`
	ClaudeMin string   `json:"claude_min,omitempty"`
}

type MCPServerSpec struct {
	Transport   string   `json:"transport"`
	Command     string   `json:"command,omitempty"`
	Args        []string `json:"args,omitempty"`
	URL         string   `json:"url,omitempty"`
	EnvRequired []string `json:"env_required,omitempty"`
}

type Lockfile struct {
	SchemaVersion int                `json:"schema_version"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Packages      []InstalledPackage `json:"packages"`
}

type SBOM struct {
	SchemaVersion int        `json:"schema_version"`
	GeneratedAt   time.Time  `json:"generated_at"`
	Components    []SBOMItem `json:"components"`
}

type SBOMItem struct {
	Name    string    `json:"name"`
	Version string    `json:"version"`
	Source  SourceRef `json:"source"`
}

func NewDefaultState() State {
	now := time.Now().UTC()
	return State{
		Version: StateVersion,
		Taps: map[string]TapConfig{
			DefaultTapName: {
				Name:        DefaultTapName,
				URL:         DefaultTapURL,
				Description: DefaultTapDescription,
				Trust: TapTrustConfig{
					Mode: TrustModeCosign,
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		Installed:            map[string]InstalledPackage{},
		TrustedDirectSources: map[string]TrustDecision{},
	}
}
