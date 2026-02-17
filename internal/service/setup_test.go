package service

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sarjann/mcper/internal/model"
)

// stubSecretStore implements secrets.Store for testing.
type stubSecretStore struct {
	data map[string]string
}

func newStubSecretStore() *stubSecretStore {
	return &stubSecretStore{data: make(map[string]string)}
}

func (s *stubSecretStore) Set(pkg, key, value string) error {
	s.data[pkg+"/"+key] = value
	return nil
}

func (s *stubSecretStore) Get(pkg, key string) (string, error) {
	v, ok := s.data[pkg+"/"+key]
	if !ok {
		return "", fmt.Errorf("secret not found")
	}
	return v, nil
}

func (s *stubSecretStore) Delete(pkg, key string) error {
	delete(s.data, pkg+"/"+key)
	return nil
}

func testManager(stdin string, secretStore *stubSecretStore) (*Manager, *bytes.Buffer) {
	var buf bytes.Buffer
	if secretStore == nil {
		secretStore = newStubSecretStore()
	}
	m := &Manager{
		stdin:         strings.NewReader(stdin),
		stdout:        &buf,
		secret:        secretStore,
		setupTimeout:  10 * time.Second,
		isInteractive: func() bool { return true },
	}
	return m, &buf
}

func TestRunSetupCommands_NoCommands(t *testing.T) {
	m, _ := testManager("", nil)
	manifest := model.PackageManifest{Name: "test-pkg"}
	results := m.runSetupCommands(context.Background(), "test-pkg", manifest)
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

func TestRunSetupCommands_NonInteractive(t *testing.T) {
	var buf bytes.Buffer
	m := &Manager{
		stdin:  strings.NewReader(""),
		stdout: &buf,
		secret: newStubSecretStore(),
		isInteractive: func() bool { return false },
	}
	manifest := model.PackageManifest{
		Name: "test-pkg",
		SetupCommands: map[string]model.SetupCommand{
			"TOKEN": {Run: []string{"echo", "test"}},
		},
	}
	results := m.runSetupCommands(context.Background(), "test-pkg", manifest)
	if results != nil {
		t.Errorf("expected nil results for non-interactive, got %v", results)
	}
}

func TestRunSetupCommands_SecretAlreadyExists(t *testing.T) {
	secrets := newStubSecretStore()
	secrets.Set("test-pkg", "TOKEN", "existing-value")
	m, _ := testManager("", secrets)

	manifest := model.PackageManifest{
		Name: "test-pkg",
		SetupCommands: map[string]model.SetupCommand{
			"TOKEN": {Run: []string{"echo", "new-value"}},
		},
	}
	results := m.runSetupCommands(context.Background(), "test-pkg", manifest)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != SetupExisting {
		t.Errorf("expected SetupExisting, got %s", results[0].Status)
	}
}

func TestRunSetupCommands_UserSkips(t *testing.T) {
	m, _ := testManager("skip\n", nil)

	manifest := model.PackageManifest{
		Name: "test-pkg",
		SetupCommands: map[string]model.SetupCommand{
			"TOKEN": {Run: []string{"echo", "test"}, Description: "Get a token"},
		},
	}
	results := m.runSetupCommands(context.Background(), "test-pkg", manifest)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != SetupSkipped {
		t.Errorf("expected SetupSkipped, got %s", results[0].Status)
	}
}

func TestRunSetupCommands_UserConfirmsStore(t *testing.T) {
	// "yes" to run, "yes" to store
	m, buf := testManager("yes\nyes\n", nil)

	manifest := model.PackageManifest{
		Name: "test-pkg",
		SetupCommands: map[string]model.SetupCommand{
			"TOKEN": {Run: []string{"echo", "my-secret-token"}, Description: "Get a token"},
		},
	}
	results := m.runSetupCommands(context.Background(), "test-pkg", manifest)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != SetupStored {
		t.Errorf("expected SetupStored, got %s", results[0].Status)
	}

	// Verify secret was stored
	val, err := m.secret.Get("test-pkg", "TOKEN")
	if err != nil {
		t.Fatalf("secret not stored: %v", err)
	}
	if val != "my-secret-token" {
		t.Errorf("expected 'my-secret-token', got %q", val)
	}

	output := buf.String()
	if !strings.Contains(output, "Get a token") {
		t.Error("expected description in output")
	}
	if !strings.Contains(output, "Extracted value:") {
		t.Error("expected extracted value display")
	}
}

func TestRunSetupCommands_UserEditsValue(t *testing.T) {
	// "yes" to run, "edit" to edit, provide new value
	m, _ := testManager("yes\nedit\nmy-edited-token\n", nil)

	manifest := model.PackageManifest{
		Name: "test-pkg",
		SetupCommands: map[string]model.SetupCommand{
			"TOKEN": {Run: []string{"echo", "original-token"}},
		},
	}
	results := m.runSetupCommands(context.Background(), "test-pkg", manifest)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != SetupStored {
		t.Errorf("expected SetupStored, got %s", results[0].Status)
	}

	val, err := m.secret.Get("test-pkg", "TOKEN")
	if err != nil {
		t.Fatalf("secret not stored: %v", err)
	}
	if val != "my-edited-token" {
		t.Errorf("expected 'my-edited-token', got %q", val)
	}
}

func TestRunSetupCommands_UserEditsEmptyValue(t *testing.T) {
	// "yes" to run, "edit", empty value
	m, _ := testManager("yes\nedit\n\n", nil)

	manifest := model.PackageManifest{
		Name: "test-pkg",
		SetupCommands: map[string]model.SetupCommand{
			"TOKEN": {Run: []string{"echo", "original-token"}},
		},
	}
	results := m.runSetupCommands(context.Background(), "test-pkg", manifest)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != SetupSkipped {
		t.Errorf("expected SetupSkipped for empty edit, got %s", results[0].Status)
	}
}

func TestRunSetupCommands_UserDeclinesStore(t *testing.T) {
	// "yes" to run, "no" to store
	m, _ := testManager("yes\nno\n", nil)

	manifest := model.PackageManifest{
		Name: "test-pkg",
		SetupCommands: map[string]model.SetupCommand{
			"TOKEN": {Run: []string{"echo", "my-token"}},
		},
	}
	results := m.runSetupCommands(context.Background(), "test-pkg", manifest)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != SetupSkipped {
		t.Errorf("expected SetupSkipped, got %s", results[0].Status)
	}
}

func TestExecuteSetupCommand_Success(t *testing.T) {
	m, _ := testManager("", nil)
	sc := model.SetupCommand{Run: []string{"echo", "my-token"}}
	val, err := m.executeSetupCommand(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "my-token" {
		t.Errorf("expected 'my-token', got %q", val)
	}
}

func TestExecuteSetupCommand_WithPattern(t *testing.T) {
	m, _ := testManager("", nil)
	sc := model.SetupCommand{
		Run:     []string{"echo", "token=abc123"},
		Pattern: `token=(.+)`,
	}
	val, err := m.executeSetupCommand(context.Background(), sc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "abc123" {
		t.Errorf("expected 'abc123', got %q", val)
	}
}

func TestExecuteSetupCommand_PatternNoMatch(t *testing.T) {
	m, _ := testManager("", nil)
	sc := model.SetupCommand{
		Run:     []string{"echo", "no match here"},
		Pattern: `token=(.+)`,
	}
	_, err := m.executeSetupCommand(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error for pattern no match")
	}
	if !strings.Contains(err.Error(), "did not match") {
		t.Errorf("expected 'did not match' in error, got %q", err.Error())
	}
}

func TestExecuteSetupCommand_CommandNotFound(t *testing.T) {
	m, _ := testManager("", nil)
	sc := model.SetupCommand{Run: []string{"nonexistent-binary-xyz"}}
	_, err := m.executeSetupCommand(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error for command not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}

func TestExecuteSetupCommand_NonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows")
	}
	m, _ := testManager("", nil)
	sc := model.SetupCommand{Run: []string{"sh", "-c", "echo err >&2; exit 1"}}
	_, err := m.executeSetupCommand(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if !strings.Contains(err.Error(), "command failed") {
		t.Errorf("expected 'command failed' in error, got %q", err.Error())
	}
}

func TestExecuteSetupCommand_EmptyRun(t *testing.T) {
	m, _ := testManager("", nil)
	sc := model.SetupCommand{Run: []string{}}
	_, err := m.executeSetupCommand(context.Background(), sc)
	if err == nil {
		t.Fatal("expected error for empty command")
	}
	if !strings.Contains(err.Error(), "empty command") {
		t.Errorf("expected 'empty command' in error, got %q", err.Error())
	}
}

func TestExecuteSetupCommand_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows")
	}
	var buf bytes.Buffer
	m := &Manager{
		stdin:        strings.NewReader(""),
		stdout:       &buf,
		secret:       newStubSecretStore(),
		setupTimeout: 100 * time.Millisecond,
	}
	sc := model.SetupCommand{Run: []string{"sleep", "10"}}
	_, err := m.executeSetupCommand(context.Background(), sc)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got %q", err.Error())
	}
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "****"},
		{"ab", "****"},
		{"abcdefgh", "****"},
		{"abcdefghi", "abc...fghi"},
		{"sk-abc123def456", "sk-...f456"},
		{"vrc_longtoken_k9f2", "vrc...k9f2"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := maskValue(tt.input)
			if got != tt.want {
				t.Errorf("maskValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatSetupSummary_Stored(t *testing.T) {
	var buf bytes.Buffer
	results := []SetupResult{
		{EnvVar: "TOKEN", Status: SetupStored},
	}
	formatSetupSummary(&buf, "test-pkg", results, []string{"claude", "codex"})
	output := buf.String()
	if !strings.Contains(output, "TOKEN: stored") {
		t.Errorf("expected stored status, got:\n%s", output)
	}
	if !strings.Contains(output, "configured for: claude, codex") {
		t.Errorf("expected target info, got:\n%s", output)
	}
}

func TestFormatSetupSummary_Existing(t *testing.T) {
	var buf bytes.Buffer
	results := []SetupResult{
		{EnvVar: "TOKEN", Status: SetupExisting},
	}
	formatSetupSummary(&buf, "test-pkg", results, nil)
	output := buf.String()
	if !strings.Contains(output, "already configured") {
		t.Errorf("expected 'already configured', got:\n%s", output)
	}
}

func TestFormatSetupSummary_Skipped(t *testing.T) {
	var buf bytes.Buffer
	results := []SetupResult{
		{EnvVar: "TOKEN", Status: SetupSkipped},
	}
	formatSetupSummary(&buf, "test-pkg", results, nil)
	output := buf.String()
	if !strings.Contains(output, "skipped") {
		t.Errorf("expected 'skipped', got:\n%s", output)
	}
	if !strings.Contains(output, "mcper secret set test-pkg TOKEN") {
		t.Errorf("expected fallback command, got:\n%s", output)
	}
}

func TestFormatSetupSummary_Failed(t *testing.T) {
	var buf bytes.Buffer
	results := []SetupResult{
		{EnvVar: "TOKEN", Status: SetupFailed, Detail: "not found"},
	}
	formatSetupSummary(&buf, "test-pkg", results, nil)
	output := buf.String()
	if !strings.Contains(output, "failed") {
		t.Errorf("expected 'failed', got:\n%s", output)
	}
	if !strings.Contains(output, "mcper secret set test-pkg TOKEN") {
		t.Errorf("expected fallback command, got:\n%s", output)
	}
}

func TestFormatSetupSummary_Mixed(t *testing.T) {
	var buf bytes.Buffer
	results := []SetupResult{
		{EnvVar: "API_KEY", Status: SetupStored},
		{EnvVar: "TOKEN", Status: SetupSkipped},
	}
	formatSetupSummary(&buf, "my-pkg", results, []string{"claude"})
	output := buf.String()
	if !strings.Contains(output, "Setup summary:") {
		t.Error("expected header")
	}
	if !strings.Contains(output, "API_KEY: stored") {
		t.Error("expected API_KEY stored")
	}
	if !strings.Contains(output, "TOKEN: skipped") {
		t.Error("expected TOKEN skipped")
	}
}

func TestRunSetupCommands_MultipleEnvVars_Sorted(t *testing.T) {
	// Should process ALPHA before BETA (sorted order)
	// "yes" + "yes" for ALPHA, "skip" for BETA
	m, _ := testManager("yes\nyes\nskip\n", nil)

	manifest := model.PackageManifest{
		Name: "test-pkg",
		SetupCommands: map[string]model.SetupCommand{
			"BETA":  {Run: []string{"echo", "beta-val"}},
			"ALPHA": {Run: []string{"echo", "alpha-val"}},
		},
	}
	results := m.runSetupCommands(context.Background(), "test-pkg", manifest)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].EnvVar != "ALPHA" {
		t.Errorf("expected first result for ALPHA, got %s", results[0].EnvVar)
	}
	if results[0].Status != SetupStored {
		t.Errorf("expected ALPHA stored, got %s", results[0].Status)
	}
	if results[1].EnvVar != "BETA" {
		t.Errorf("expected second result for BETA, got %s", results[1].EnvVar)
	}
	if results[1].Status != SetupSkipped {
		t.Errorf("expected BETA skipped, got %s", results[1].Status)
	}
}

func TestRunSetupCommands_CommandNotFound_ShowsFallback(t *testing.T) {
	// "yes" to run
	m, buf := testManager("yes\n", nil)

	manifest := model.PackageManifest{
		Name: "test-pkg",
		SetupCommands: map[string]model.SetupCommand{
			"TOKEN": {Run: []string{"nonexistent-binary-xyz"}},
		},
	}
	results := m.runSetupCommands(context.Background(), "test-pkg", manifest)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != SetupFailed {
		t.Errorf("expected SetupFailed, got %s", results[0].Status)
	}

	output := buf.String()
	if !strings.Contains(output, "not found") {
		t.Errorf("expected 'not found' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "mcper secret set test-pkg TOKEN") {
		t.Errorf("expected fallback command in output, got:\n%s", output)
	}
}

func TestRunSetupCommands_WithPattern_Success(t *testing.T) {
	// "yes" to run, "yes" to store
	m, _ := testManager("yes\nyes\n", nil)

	manifest := model.PackageManifest{
		Name: "test-pkg",
		SetupCommands: map[string]model.SetupCommand{
			"TOKEN": {
				Run:     []string{"echo", "auth token: sk-abc123xyz"},
				Pattern: `token: (.+)`,
			},
		},
	}
	results := m.runSetupCommands(context.Background(), "test-pkg", manifest)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != SetupStored {
		t.Errorf("expected SetupStored, got %s", results[0].Status)
	}

	val, _ := m.secret.Get("test-pkg", "TOKEN")
	if val != "sk-abc123xyz" {
		t.Errorf("expected 'sk-abc123xyz', got %q", val)
	}
}
