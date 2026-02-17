package service

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/sarjann/mcper/internal/adapters"
	"github.com/sarjann/mcper/internal/model"
)

type ConflictKind string

const (
	ConflictNameExists   ConflictKind = "name_exists"
	ConflictDuplicateSpec ConflictKind = "duplicate_spec"
)

type ServerConflict struct {
	Target       string
	ServerName   string
	Kind         ConflictKind
	ExistingName string // for duplicate_spec, the existing name that shares the same canonical key
}

type DiffOp string

const (
	DiffAdd    DiffOp = "add"
	DiffModify DiffOp = "modify"
	DiffNoop   DiffOp = "noop"
)

type ServerDiff struct {
	Target     string
	ServerName string
	Op         DiffOp
	Before     *model.MCPServerSpec
	After      model.MCPServerSpec
}

type InstallPlan struct {
	Conflicts []ServerConflict
	Diffs     []ServerDiff
}

func (p InstallPlan) HasConflicts() bool {
	return len(p.Conflicts) > 0
}

func (p InstallPlan) HasMeaningfulChanges() bool {
	for _, d := range p.Diffs {
		if d.Op != DiffNoop {
			return true
		}
	}
	return false
}

func (p InstallPlan) NeedsPrompt() bool {
	return p.HasConflicts() || p.HasMeaningfulChanges()
}

func buildInstallPlan(ctx context.Context, targets []string, adapterMap map[string]adapters.Adapter, incoming map[string]model.MCPServerSpec) (InstallPlan, error) {
	var plan InstallPlan

	for _, target := range targets {
		adapter := adapterMap[target]
		existing, err := adapter.ListServers(ctx)
		if err != nil {
			return plan, fmt.Errorf("list servers for %s: %w", target, err)
		}

		// Build reverse index: canonical key -> existing server name
		keyToName := make(map[string]string, len(existing))
		for name, spec := range existing {
			keyToName[canonicalKey(spec)] = name
		}

		for serverName, incomingSpec := range incoming {
			existingSpec, nameExists := existing[serverName]
			incomingKey := canonicalKey(incomingSpec)

			if nameExists {
				// Same name already exists
				if specsEqual(existingSpec, incomingSpec) {
					plan.Diffs = append(plan.Diffs, ServerDiff{
						Target:     target,
						ServerName: serverName,
						Op:         DiffNoop,
						Before:     &existingSpec,
						After:      incomingSpec,
					})
				} else {
					plan.Conflicts = append(plan.Conflicts, ServerConflict{
						Target:     target,
						ServerName: serverName,
						Kind:       ConflictNameExists,
					})
					plan.Diffs = append(plan.Diffs, ServerDiff{
						Target:     target,
						ServerName: serverName,
						Op:         DiffModify,
						Before:     &existingSpec,
						After:      incomingSpec,
					})
				}
			} else if existingName, found := keyToName[incomingKey]; found {
				// Same server spec exists under a different name
				plan.Conflicts = append(plan.Conflicts, ServerConflict{
					Target:       target,
					ServerName:   serverName,
					Kind:         ConflictDuplicateSpec,
					ExistingName: existingName,
				})
				plan.Diffs = append(plan.Diffs, ServerDiff{
					Target:     target,
					ServerName: serverName,
					Op:         DiffAdd,
					After:      incomingSpec,
				})
			} else {
				plan.Diffs = append(plan.Diffs, ServerDiff{
					Target:     target,
					ServerName: serverName,
					Op:         DiffAdd,
					After:      incomingSpec,
				})
			}
		}
	}

	sort.Slice(plan.Diffs, func(i, j int) bool {
		if plan.Diffs[i].Target != plan.Diffs[j].Target {
			return plan.Diffs[i].Target < plan.Diffs[j].Target
		}
		return plan.Diffs[i].ServerName < plan.Diffs[j].ServerName
	})
	sort.Slice(plan.Conflicts, func(i, j int) bool {
		if plan.Conflicts[i].Target != plan.Conflicts[j].Target {
			return plan.Conflicts[i].Target < plan.Conflicts[j].Target
		}
		return plan.Conflicts[i].ServerName < plan.Conflicts[j].ServerName
	})

	return plan, nil
}

func canonicalKey(spec model.MCPServerSpec) string {
	if spec.Transport == model.ServerTransportHTTP {
		return "http:" + spec.URL
	}
	parts := []string{spec.Command}
	parts = append(parts, spec.Args...)
	return "stdio:" + strings.Join(parts, " ")
}

func specsEqual(a, b model.MCPServerSpec) bool {
	return canonicalKey(a) == canonicalKey(b)
}

func specSummary(spec model.MCPServerSpec) string {
	if spec.Transport == model.ServerTransportHTTP {
		return "url: " + spec.URL
	}
	parts := []string{spec.Command}
	parts = append(parts, spec.Args...)
	return "command: " + strings.Join(parts, " ")
}

func formatInstallPlan(w io.Writer, plan InstallPlan) {
	fmt.Fprintln(w, "The following changes will be applied:")

	// Group diffs by target
	byTarget := make(map[string][]ServerDiff)
	for _, d := range plan.Diffs {
		byTarget[d.Target] = append(byTarget[d.Target], d)
	}
	targets := make([]string, 0, len(byTarget))
	for t := range byTarget {
		targets = append(targets, t)
	}
	sort.Strings(targets)

	for _, target := range targets {
		diffs := byTarget[target]
		for _, d := range diffs {
			fmt.Fprintf(w, "  [%s] %s\n", target, d.ServerName)
			switch d.Op {
			case DiffAdd:
				fmt.Fprintf(w, "    + %s\n", specSummary(d.After))
			case DiffModify:
				if d.Before != nil {
					fmt.Fprintf(w, "    - %s\n", specSummary(*d.Before))
				}
				fmt.Fprintf(w, "    + %s\n", specSummary(d.After))
			case DiffNoop:
				fmt.Fprintf(w, "    (no change)\n")
			}
		}
	}

	// Print warnings
	for _, c := range plan.Conflicts {
		fmt.Fprintln(w)
		switch c.Kind {
		case ConflictNameExists:
			fmt.Fprintf(w, "Warning: server %q already exists in %s config and will be overwritten.\n", c.ServerName, c.Target)
		case ConflictDuplicateSpec:
			fmt.Fprintf(w, "Warning: server %q appears to duplicate existing server %q in %s config.\n", c.ServerName, c.ExistingName, c.Target)
		}
	}

	fmt.Fprintln(w)
}
