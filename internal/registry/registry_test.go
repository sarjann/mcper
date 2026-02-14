package registry

import (
	"testing"

	"github.com/sarjann/mcper/internal/model"
)

func TestResolveVersionConstraint(t *testing.T) {
	pkg := model.IndexPackage{Versions: map[string]model.IndexVersion{
		"1.0.0": {},
		"1.2.0": {},
		"2.0.0": {},
	}}

	ver, _, err := resolveVersion(pkg, ">=1.0.0, <2.0.0")
	if err != nil {
		t.Fatalf("resolveVersion returned error: %v", err)
	}
	if ver != "1.2.0" {
		t.Fatalf("expected 1.2.0, got %s", ver)
	}
}

func TestValidateManifest(t *testing.T) {
	m := model.PackageManifest{
		Name:    "demo",
		Version: "1.0.0",
		MCPServers: map[string]model.MCPServerSpec{
			"demo": {
				Transport: model.ServerTransportSTDIO,
				Command:   "npx",
				Args:      []string{"-y", "demo-mcp"},
			},
		},
	}
	if err := validateManifest(m); err != nil {
		t.Fatalf("expected valid manifest, got %v", err)
	}
}
