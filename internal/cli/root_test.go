package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootNoArgsShowsHelpWhenNonInteractive(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs(nil)

	in := bytes.NewBuffer(nil)
	out := bytes.NewBuffer(nil)
	cmd.SetIn(in)
	cmd.SetOut(out)
	cmd.SetErr(out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute root command: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Usage:") {
		t.Fatalf("expected help usage output, got: %q", got)
	}
	if !strings.Contains(got, "mcper") {
		t.Fatalf("expected command name in help output, got: %q", got)
	}
}
