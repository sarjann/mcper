package service

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/sarjann/mcper/internal/adapters"
	"github.com/sarjann/mcper/internal/model"
)

// stubAdapter implements adapters.Adapter for testing.
type stubAdapter struct {
	name    string
	servers map[string]model.MCPServerSpec
}

func (s *stubAdapter) Name() string { return s.name }
func (s *stubAdapter) Path() string { return "/tmp/" + s.name }
func (s *stubAdapter) UpsertServers(_ context.Context, specs map[string]model.MCPServerSpec) error {
	for k, v := range specs {
		s.servers[k] = v
	}
	return nil
}
func (s *stubAdapter) RemoveServers(_ context.Context, names []string) error {
	for _, n := range names {
		delete(s.servers, n)
	}
	return nil
}
func (s *stubAdapter) ListServers(_ context.Context) (map[string]model.MCPServerSpec, error) {
	return s.servers, nil
}

func newStub(name string, servers map[string]model.MCPServerSpec) *stubAdapter {
	if servers == nil {
		servers = make(map[string]model.MCPServerSpec)
	}
	return &stubAdapter{name: name, servers: servers}
}

func TestCanonicalKey(t *testing.T) {
	tests := []struct {
		name string
		spec model.MCPServerSpec
		want string
	}{
		{
			name: "stdio server",
			spec: model.MCPServerSpec{Transport: model.ServerTransportSTDIO, Command: "npx", Args: []string{"-y", "@vercel/mcp"}},
			want: "stdio:npx -y @vercel/mcp",
		},
		{
			name: "stdio server no args",
			spec: model.MCPServerSpec{Transport: model.ServerTransportSTDIO, Command: "myserver"},
			want: "stdio:myserver",
		},
		{
			name: "http server",
			spec: model.MCPServerSpec{Transport: model.ServerTransportHTTP, URL: "https://example.com/mcp"},
			want: "http:https://example.com/mcp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalKey(tt.spec)
			if got != tt.want {
				t.Errorf("canonicalKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildInstallPlan_CleanInstall(t *testing.T) {
	ctx := context.Background()
	claude := newStub("claude", nil)
	adapterMap := map[string]adapters.Adapter{"claude": claude}

	incoming := map[string]model.MCPServerSpec{
		"vercel": {Transport: model.ServerTransportSTDIO, Command: "npx", Args: []string{"-y", "@vercel/mcp"}},
	}

	plan, err := buildInstallPlan(ctx, []string{"claude"}, adapterMap, incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(plan.Conflicts))
	}
	if len(plan.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(plan.Diffs))
	}
	if plan.Diffs[0].Op != DiffAdd {
		t.Errorf("expected DiffAdd, got %s", plan.Diffs[0].Op)
	}
	if plan.NeedsPrompt() {
		// A clean add with no conflicts still shows the diff; that's meaningful
		// but there's no conflict, so let's verify HasConflicts is false
		if plan.HasConflicts() {
			t.Errorf("expected no conflicts")
		}
	}
}

func TestBuildInstallPlan_NameCollision(t *testing.T) {
	ctx := context.Background()
	claude := newStub("claude", map[string]model.MCPServerSpec{
		"vercel": {Transport: model.ServerTransportSTDIO, Command: "npx", Args: []string{"-y", "@vercel/mcp"}},
	})
	adapterMap := map[string]adapters.Adapter{"claude": claude}

	incoming := map[string]model.MCPServerSpec{
		"vercel": {Transport: model.ServerTransportSTDIO, Command: "npx", Args: []string{"-y", "@vercel/mcp@latest"}},
	}

	plan, err := buildInstallPlan(ctx, []string{"claude"}, adapterMap, incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(plan.Conflicts))
	}
	if plan.Conflicts[0].Kind != ConflictNameExists {
		t.Errorf("expected ConflictNameExists, got %s", plan.Conflicts[0].Kind)
	}
	if len(plan.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(plan.Diffs))
	}
	if plan.Diffs[0].Op != DiffModify {
		t.Errorf("expected DiffModify, got %s", plan.Diffs[0].Op)
	}
	if !plan.NeedsPrompt() {
		t.Error("expected NeedsPrompt to be true")
	}
}

func TestBuildInstallPlan_IdenticalReinstall(t *testing.T) {
	ctx := context.Background()
	spec := model.MCPServerSpec{Transport: model.ServerTransportSTDIO, Command: "npx", Args: []string{"-y", "@vercel/mcp"}}
	claude := newStub("claude", map[string]model.MCPServerSpec{"vercel": spec})
	adapterMap := map[string]adapters.Adapter{"claude": claude}

	incoming := map[string]model.MCPServerSpec{"vercel": spec}

	plan, err := buildInstallPlan(ctx, []string{"claude"}, adapterMap, incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Conflicts) != 0 {
		t.Errorf("expected no conflicts for identical spec, got %d", len(plan.Conflicts))
	}
	if len(plan.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(plan.Diffs))
	}
	if plan.Diffs[0].Op != DiffNoop {
		t.Errorf("expected DiffNoop, got %s", plan.Diffs[0].Op)
	}
	if plan.NeedsPrompt() {
		t.Error("expected NeedsPrompt to be false for identical reinstall")
	}
}

func TestBuildInstallPlan_DuplicateSpec(t *testing.T) {
	ctx := context.Background()
	claude := newStub("claude", map[string]model.MCPServerSpec{
		"my-vercel": {Transport: model.ServerTransportSTDIO, Command: "npx", Args: []string{"-y", "@vercel/mcp"}},
	})
	adapterMap := map[string]adapters.Adapter{"claude": claude}

	incoming := map[string]model.MCPServerSpec{
		"vercel": {Transport: model.ServerTransportSTDIO, Command: "npx", Args: []string{"-y", "@vercel/mcp"}},
	}

	plan, err := buildInstallPlan(ctx, []string{"claude"}, adapterMap, incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(plan.Conflicts))
	}
	if plan.Conflicts[0].Kind != ConflictDuplicateSpec {
		t.Errorf("expected ConflictDuplicateSpec, got %s", plan.Conflicts[0].Kind)
	}
	if plan.Conflicts[0].ExistingName != "my-vercel" {
		t.Errorf("expected existing name 'my-vercel', got %q", plan.Conflicts[0].ExistingName)
	}
	if plan.Diffs[0].Op != DiffAdd {
		t.Errorf("expected DiffAdd, got %s", plan.Diffs[0].Op)
	}
}

func TestBuildInstallPlan_MultiTarget(t *testing.T) {
	ctx := context.Background()
	existingSpec := model.MCPServerSpec{Transport: model.ServerTransportSTDIO, Command: "npx", Args: []string{"-y", "@vercel/mcp"}}
	claude := newStub("claude", map[string]model.MCPServerSpec{"vercel": existingSpec})
	codex := newStub("codex", nil) // no existing servers
	adapterMap := map[string]adapters.Adapter{"claude": claude, "codex": codex}

	incoming := map[string]model.MCPServerSpec{
		"vercel": {Transport: model.ServerTransportSTDIO, Command: "npx", Args: []string{"-y", "@vercel/mcp@latest"}},
	}

	plan, err := buildInstallPlan(ctx, []string{"claude", "codex"}, adapterMap, incoming)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Claude should have a conflict, codex should not
	if len(plan.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(plan.Conflicts))
	}
	if plan.Conflicts[0].Target != "claude" {
		t.Errorf("expected conflict target 'claude', got %q", plan.Conflicts[0].Target)
	}

	// Should have 2 diffs: one modify (claude), one add (codex)
	if len(plan.Diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %d", len(plan.Diffs))
	}

	diffByTarget := make(map[string]ServerDiff)
	for _, d := range plan.Diffs {
		diffByTarget[d.Target] = d
	}

	if diffByTarget["claude"].Op != DiffModify {
		t.Errorf("expected claude DiffModify, got %s", diffByTarget["claude"].Op)
	}
	if diffByTarget["codex"].Op != DiffAdd {
		t.Errorf("expected codex DiffAdd, got %s", diffByTarget["codex"].Op)
	}
}

func TestFormatInstallPlan_Output(t *testing.T) {
	before := model.MCPServerSpec{Transport: model.ServerTransportSTDIO, Command: "npx", Args: []string{"-y", "@vercel/mcp"}}
	after := model.MCPServerSpec{Transport: model.ServerTransportSTDIO, Command: "npx", Args: []string{"-y", "@vercel/mcp@latest"}}

	plan := InstallPlan{
		Conflicts: []ServerConflict{
			{Target: "claude", ServerName: "vercel", Kind: ConflictNameExists},
		},
		Diffs: []ServerDiff{
			{Target: "claude", ServerName: "vercel", Op: DiffModify, Before: &before, After: after},
		},
	}

	var buf bytes.Buffer
	formatInstallPlan(&buf, plan)
	output := buf.String()

	if !strings.Contains(output, "The following changes will be applied:") {
		t.Error("expected header line")
	}
	if !strings.Contains(output, "[claude] vercel") {
		t.Error("expected target/server label")
	}
	if !strings.Contains(output, "- command: npx -y @vercel/mcp") {
		t.Errorf("expected before line, got:\n%s", output)
	}
	if !strings.Contains(output, "+ command: npx -y @vercel/mcp@latest") {
		t.Errorf("expected after line, got:\n%s", output)
	}
	if !strings.Contains(output, "Warning:") {
		t.Error("expected warning")
	}
	if !strings.Contains(output, "already exists") {
		t.Error("expected 'already exists' in warning")
	}
}

func TestFormatInstallPlan_DuplicateSpecWarning(t *testing.T) {
	after := model.MCPServerSpec{Transport: model.ServerTransportHTTP, URL: "https://example.com/mcp"}
	plan := InstallPlan{
		Conflicts: []ServerConflict{
			{Target: "codex", ServerName: "vercel", Kind: ConflictDuplicateSpec, ExistingName: "my-vercel"},
		},
		Diffs: []ServerDiff{
			{Target: "codex", ServerName: "vercel", Op: DiffAdd, After: after},
		},
	}

	var buf bytes.Buffer
	formatInstallPlan(&buf, plan)
	output := buf.String()

	if !strings.Contains(output, "duplicate existing server") {
		t.Errorf("expected duplicate warning, got:\n%s", output)
	}
	if !strings.Contains(output, `"my-vercel"`) {
		t.Errorf("expected existing name in warning, got:\n%s", output)
	}
}

func TestSpecSummary(t *testing.T) {
	tests := []struct {
		name string
		spec model.MCPServerSpec
		want string
	}{
		{
			name: "stdio",
			spec: model.MCPServerSpec{Transport: model.ServerTransportSTDIO, Command: "npx", Args: []string{"-y", "@vercel/mcp"}},
			want: "command: npx -y @vercel/mcp",
		},
		{
			name: "http",
			spec: model.MCPServerSpec{Transport: model.ServerTransportHTTP, URL: "https://example.com/mcp"},
			want: "url: https://example.com/mcp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := specSummary(tt.spec)
			if got != tt.want {
				t.Errorf("specSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}
