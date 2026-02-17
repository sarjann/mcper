package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sarjann/mcper/internal/model"
)

type SetupStatus string

const (
	SetupStored   SetupStatus = "stored"
	SetupSkipped  SetupStatus = "skipped"
	SetupFailed   SetupStatus = "failed"
	SetupExisting SetupStatus = "existing"
)

type SetupResult struct {
	EnvVar string
	Status SetupStatus
	Detail string
}

func (m *Manager) runSetupCommands(ctx context.Context, pkgName string, manifest model.PackageManifest) []SetupResult {
	if len(manifest.SetupCommands) == 0 {
		return nil
	}
	if !m.isInteractive() {
		return nil
	}

	envVars := make([]string, 0, len(manifest.SetupCommands))
	for k := range manifest.SetupCommands {
		envVars = append(envVars, k)
	}
	sort.Strings(envVars)

	results := make([]SetupResult, 0, len(envVars))
	reader := bufio.NewReader(m.stdin)

	for _, envVar := range envVars {
		sc := manifest.SetupCommands[envVar]

		// Check if secret already exists
		if _, err := m.secret.Get(pkgName, envVar); err == nil {
			results = append(results, SetupResult{EnvVar: envVar, Status: SetupExisting})
			continue
		}

		// Show description if available
		if sc.Description != "" {
			fmt.Fprintf(m.stdout, "\n%s\n", sc.Description)
		}

		// Prompt to run
		cmdStr := strings.Join(sc.Run, " ")
		fmt.Fprintf(m.stdout, "Run %q to obtain %s? [yes/skip] ", cmdStr, envVar)
		resp, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			results = append(results, SetupResult{EnvVar: envVar, Status: SetupFailed, Detail: fmt.Sprintf("read input: %v", err)})
			continue
		}
		resp = strings.ToLower(strings.TrimSpace(resp))
		if resp != "yes" {
			results = append(results, SetupResult{EnvVar: envVar, Status: SetupSkipped})
			continue
		}

		// Execute
		fmt.Fprintf(m.stdout, "\nRunning %s...\n", cmdStr)
		value, err := m.executeSetupCommand(ctx, sc)
		if err != nil {
			fmt.Fprintf(m.stdout, "Error: %v\n", err)
			fmt.Fprintf(m.stdout, "  Set manually: mcper secret set %s %s\n", pkgName, envVar)
			results = append(results, SetupResult{EnvVar: envVar, Status: SetupFailed, Detail: err.Error()})
			continue
		}

		// Show masked value and prompt to store/edit/skip
		fmt.Fprintf(m.stdout, "\nExtracted value: %s\n", maskValue(value))
		fmt.Fprintf(m.stdout, "Store as %s? [yes/edit/skip] ", envVar)
		resp, err = reader.ReadString('\n')
		if err != nil && err != io.EOF {
			results = append(results, SetupResult{EnvVar: envVar, Status: SetupFailed, Detail: fmt.Sprintf("read input: %v", err)})
			continue
		}
		resp = strings.ToLower(strings.TrimSpace(resp))

		switch resp {
		case "edit":
			fmt.Fprintf(m.stdout, "Enter value for %s: ", envVar)
			edited, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				results = append(results, SetupResult{EnvVar: envVar, Status: SetupFailed, Detail: fmt.Sprintf("read input: %v", err)})
				continue
			}
			value = strings.TrimSpace(edited)
			if value == "" {
				results = append(results, SetupResult{EnvVar: envVar, Status: SetupSkipped, Detail: "empty value"})
				continue
			}
		case "yes":
			// use extracted value as-is
		default:
			results = append(results, SetupResult{EnvVar: envVar, Status: SetupSkipped})
			continue
		}

		// Store
		if err := m.secret.Set(pkgName, envVar, value); err != nil {
			fmt.Fprintf(m.stdout, "Error storing secret: %v\n", err)
			results = append(results, SetupResult{EnvVar: envVar, Status: SetupFailed, Detail: err.Error()})
			continue
		}
		results = append(results, SetupResult{EnvVar: envVar, Status: SetupStored})
	}

	return results
}

func (m *Manager) executeSetupCommand(ctx context.Context, sc model.SetupCommand) (string, error) {
	if len(sc.Run) == 0 {
		return "", fmt.Errorf("empty command")
	}

	if _, err := exec.LookPath(sc.Run[0]); err != nil {
		return "", fmt.Errorf("%q not found in PATH", sc.Run[0])
	}

	timeout := m.setupTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, sc.Run[0], sc.Run[1:]...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %s", timeout)
	}
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("command failed: %s", errMsg)
	}

	output := stdout.String()
	if sc.Pattern != "" {
		re, err := regexp.Compile(sc.Pattern)
		if err != nil {
			return "", fmt.Errorf("invalid pattern %q: %w", sc.Pattern, err)
		}
		matches := re.FindStringSubmatch(output)
		if len(matches) < 2 {
			return "", fmt.Errorf("pattern %q did not match output:\n%s", sc.Pattern, strings.TrimSpace(output))
		}
		return strings.TrimSpace(matches[1]), nil
	}

	return strings.TrimSpace(output), nil
}

func maskValue(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:3] + "..." + s[len(s)-4:]
}

func formatSetupSummary(w io.Writer, pkgName string, results []SetupResult, targets []string) {
	fmt.Fprintln(w, "\nSetup summary:")
	targetInfo := ""
	if len(targets) > 0 {
		targetInfo = fmt.Sprintf(" (configured for: %s)", strings.Join(targets, ", "))
	}
	for _, r := range results {
		switch r.Status {
		case SetupStored:
			fmt.Fprintf(w, "  %s: stored ✓%s\n", r.EnvVar, targetInfo)
		case SetupExisting:
			fmt.Fprintf(w, "  %s: already configured ✓\n", r.EnvVar)
		case SetupSkipped:
			fmt.Fprintf(w, "  %s: skipped — set manually: mcper secret set %s %s\n", r.EnvVar, pkgName, r.EnvVar)
		case SetupFailed:
			fmt.Fprintf(w, "  %s: failed — set manually: mcper secret set %s %s\n", r.EnvVar, pkgName, r.EnvVar)
		}
	}
}
