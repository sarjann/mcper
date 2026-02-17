package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/manifoldco/promptui"

	"github.com/sarjann/mcper/internal/service"
)

func runTUI(ctx context.Context, in io.Reader, out io.Writer, mgr *service.Manager) error {
	_ = in
	fmt.Fprintln(out, "mcper interactive mode")
	fmt.Fprintln(out, "Use arrow keys + Enter. Press Ctrl+C to exit.")

	items := []string{
		"Search packages",
		"List installed packages",
		"Install package from tap",
		"Install package from URL/path",
		"Remove package",
		"Upgrade packages",
		"Run doctor",
		"Export lockfile (JSON)",
		"Quit",
	}

	for {
		fmt.Fprintln(out)
		choice, err := selectOne("Action", items)
		if err != nil {
			if isPromptCanceled(err) {
				fmt.Fprintln(out)
				return nil
			}
			return err
		}

		var actionErr error
		switch choice {
		case "Search packages":
			actionErr = tuiSearch(ctx, out, mgr)
		case "List installed packages":
			actionErr = tuiList(out, mgr)
		case "Install package from tap":
			actionErr = tuiInstallFromTap(ctx, out, mgr)
		case "Install package from URL/path":
			actionErr = tuiInstallFromURL(ctx, out, mgr)
		case "Remove package":
			actionErr = tuiRemove(ctx, out, mgr)
		case "Upgrade packages":
			actionErr = tuiUpgrade(ctx, out, mgr)
		case "Run doctor":
			actionErr = tuiDoctor(ctx, out, mgr)
		case "Export lockfile (JSON)":
			actionErr = tuiExport(out, mgr)
		case "Quit":
			fmt.Fprintln(out, "Goodbye.")
			return nil
		}
		if actionErr != nil {
			fmt.Fprintf(out, "Error: %v\n", actionErr)
		}
	}
}

func tuiSearch(ctx context.Context, out io.Writer, mgr *service.Manager) error {
	query, err := promptText("Search query", "", false)
	if err != nil {
		return err
	}
	results, err := mgr.Search(ctx, query)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Fprintln(out, "No results.")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TAP\tPACKAGE\tLATEST\tDESCRIPTION")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Tap, r.Name, r.Latest, r.Description)
	}
	return w.Flush()
}

func tuiList(out io.Writer, mgr *service.Manager) error {
	pkgs, err := mgr.ListInstalled()
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		fmt.Fprintln(out, "No packages installed.")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PACKAGE\tVERSION\tSOURCE\tTARGETS")
	for _, p := range pkgs {
		source := p.Source.Type
		if p.Source.Tap != "" {
			source += ":" + p.Source.Tap
		}
		if p.Source.URL != "" {
			source += ":" + p.Source.URL
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Version, source, strings.Join(p.Targets, ","))
	}
	return w.Flush()
}

func tuiInstallFromTap(ctx context.Context, out io.Writer, mgr *service.Manager) error {
	nameRaw, err := promptText("Package (name[@version])", "", true)
	if err != nil {
		return err
	}
	tap, err := promptText("Tap name (blank = default)", "", false)
	if err != nil {
		return err
	}
	target, err := selectOne("Target configs", []string{
		"all",
		"codex",
		"claude",
		"codex,claude",
	})
	if err != nil {
		return err
	}
	name, version := splitNameVersion(nameRaw)

	installed, err := mgr.InstallFromTap(ctx, service.InstallRequest{
		Name:    name,
		Version: version,
		Tap:     tap,
		Target:  target,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Installed %s@%s targets=%s\n", installed.Name, installed.Version, strings.Join(installed.Targets, ","))
	return nil
}

func tuiInstallFromURL(ctx context.Context, out io.Writer, mgr *service.Manager) error {
	url, err := promptText("Manifest URL or local path", "", true)
	if err != nil {
		return err
	}
	target, err := selectOne("Target configs", []string{
		"all",
		"codex",
		"claude",
		"codex,claude",
	})
	if err != nil {
		return err
	}
	approved, err := promptYesNo("Trust this direct source")
	if err != nil {
		return err
	}
	if !approved {
		fmt.Fprintln(out, "Install canceled.")
		return nil
	}

	installed, err := mgr.InstallFromURL(ctx, service.InstallURLRequest{
		URL:    url,
		Target: target,
		Yes:    true,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Installed %s@%s from %s\n", installed.Name, installed.Version, installed.Source.URL)
	return nil
}

func tuiRemove(ctx context.Context, out io.Writer, mgr *service.Manager) error {
	name, err := promptText("Package name to remove", "", true)
	if err != nil {
		return err
	}
	approved, err := promptYesNo("Confirm remove")
	if err != nil {
		return err
	}
	if !approved {
		fmt.Fprintln(out, "Remove canceled.")
		return nil
	}
	if err := mgr.Remove(ctx, name); err != nil {
		return err
	}
	fmt.Fprintf(out, "Removed %s\n", name)
	return nil
}

func tuiUpgrade(ctx context.Context, out io.Writer, mgr *service.Manager) error {
	name, err := promptText("Package name (blank = all)", "", false)
	if err != nil {
		return err
	}
	allowMajor, err := promptYesNo("Allow major upgrades")
	if err != nil {
		return err
	}
	results, err := mgr.Upgrade(ctx, name, allowMajor)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Fprintln(out, "No packages installed.")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PACKAGE\tFROM\tTO\tSTATUS")
	for _, r := range results {
		status := "No change"
		if r.WasUpgraded {
			status = "Upgraded"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, r.OldVersion, r.NewVersion, status)
	}
	return w.Flush()
}

func tuiDoctor(ctx context.Context, out io.Writer, mgr *service.Manager) error {
	fix, err := promptYesNo("Attempt auto-fix")
	if err != nil {
		return err
	}
	issues, err := mgr.Doctor(ctx, fix)
	if err != nil {
		return err
	}
	if len(issues) == 0 {
		fmt.Fprintln(out, "doctor: no issues found")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "KIND\tPACKAGE\tTARGET\tDETAIL")
	for _, issue := range issues {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", issue.Kind, issue.Package, issue.Target, issue.Detail)
	}
	return w.Flush()
}

func tuiExport(out io.Writer, mgr *service.Manager) error {
	payload, err := mgr.Export("lock")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(payload))
	return nil
}

func promptText(label, defaultValue string, required bool) (string, error) {
	prompt := promptui.Prompt{
		Label:   label,
		Default: defaultValue,
	}
	if required {
		prompt.Validate = func(input string) error {
			if strings.TrimSpace(input) == "" {
				return errors.New("value is required")
			}
			return nil
		}
	}
	out, err := prompt.Run()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func promptYesNo(label string) (bool, error) {
	choice, err := selectOne(label, []string{"yes", "no"})
	if err != nil {
		return false, err
	}
	return choice == "yes", nil
}

func selectOne(label string, items []string) (string, error) {
	prompt := promptui.Select{
		Label: label,
		Items: items,
		Size:  len(items),
	}
	_, selected, err := prompt.Run()
	return selected, err
}

func isPromptCanceled(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, promptui.ErrInterrupt) || errors.Is(err, promptui.ErrEOF)
}
