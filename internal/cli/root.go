package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sarjann/mcper/internal/model"
	"github.com/sarjann/mcper/internal/service"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcper",
		Short: "Homebrew-style MCP package manager for Codex and Claude Code",
		RunE: func(cmd *cobra.Command, args []string) error {
			in := cmd.InOrStdin()
			out := cmd.OutOrStdout()
			if !isInteractiveSession(in, out) {
				return cmd.Help()
			}
			mgr, err := service.NewManager(in, out)
			if err != nil {
				return err
			}
			return runTUI(cmd.Context(), in, out, mgr)
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true

	cmd.AddCommand(
		newSearchCmd(),
		newInstallCmd(),
		newInstallURLCmd(),
		newListCmd(),
		newInfoCmd(),
		newRemoveCmd(),
		newUpgradeCmd(),
		newDoctorCmd(),
		newExportCmd(),
		newTapCmd(),
		newSecretCmd(),
	)

	return cmd
}

func managerOrDie() (*service.Manager, error) {
	return service.NewManager(os.Stdin, os.Stdout)
}

func isInteractiveSession(in io.Reader, out io.Writer) bool {
	inFile, ok := in.(*os.File)
	if !ok {
		return false
	}
	outFile, ok := out.(*os.File)
	if !ok {
		return false
	}
	inInfo, err := inFile.Stat()
	if err != nil {
		return false
	}
	outInfo, err := outFile.Stat()
	if err != nil {
		return false
	}
	return inInfo.Mode()&os.ModeCharDevice != 0 && outInfo.Mode()&os.ModeCharDevice != 0
}

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search packages across taps",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			results, err := mgr.Search(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if len(results) == 0 {
				fmt.Println("No results")
				return nil
			}
			for _, r := range results {
				fmt.Printf("%s/%s %s - %s\n", r.Tap, r.Name, r.Latest, r.Description)
			}
			return nil
		},
	}
	return cmd
}

func newInstallCmd() *cobra.Command {
	var tap string
	var target string
	var force bool

	cmd := &cobra.Command{
		Use:   "install <name[@version]>",
		Short: "Install a package from a configured tap",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			name, ver := splitNameVersion(args[0])
			installed, err := mgr.InstallFromTap(cmd.Context(), service.InstallRequest{
				Name:    name,
				Version: ver,
				Tap:     tap,
				Target:  target,
				Force:   force,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Installed %s@%s targets=%s\n", installed.Name, installed.Version, strings.Join(installed.Targets, ","))
			return nil
		},
	}
	cmd.Flags().StringVar(&tap, "tap", "", "Tap name (default: official)")
	cmd.Flags().StringVar(&target, "target", model.TargetAll, "Target config(s): all (detected clients), or comma-separated (claude, codex, claude-desktop, cursor, vscode, gemini, zed, opencode)")
	cmd.Flags().BoolVar(&force, "force", false, "Skip conflict detection and overwrite existing servers")
	return cmd
}

func newInstallURLCmd() *cobra.Command {
	var target string
	var yes bool
	var force bool

	cmd := &cobra.Command{
		Use:   "install-url <url-or-path>",
		Short: "Install package from direct manifest URL/path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			installed, err := mgr.InstallFromURL(cmd.Context(), service.InstallURLRequest{
				URL:    args[0],
				Target: target,
				Yes:    yes,
				Force:  force,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Installed %s@%s from %s\n", installed.Name, installed.Version, installed.Source.URL)
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", model.TargetAll, "Target config(s): all (detected clients), or comma-separated (claude, codex, claude-desktop, cursor, vscode, gemini, zed, opencode)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Trust direct source without interactive prompt")
	cmd.Flags().BoolVar(&force, "force", false, "Skip conflict detection and overwrite existing servers")
	return cmd
}

func newListCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed packages",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			pkgs, err := mgr.ListInstalled()
			if err != nil {
				return err
			}
			if asJSON {
				data, _ := json.MarshalIndent(pkgs, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			if len(pkgs) == 0 {
				fmt.Println("No packages installed")
				return nil
			}
			for _, p := range pkgs {
				source := p.Source.Type
				if p.Source.Tap != "" {
					source += ":" + p.Source.Tap
				}
				if p.Source.URL != "" {
					source += ":" + p.Source.URL
				}
				fmt.Printf("%s@%s source=%s targets=%s\n", p.Name, p.Version, source, strings.Join(p.Targets, ","))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newInfoCmd() *cobra.Command {
	var tap string
	cmd := &cobra.Command{
		Use:   "info <name>",
		Short: "Show package manifest details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			info, err := mgr.Info(cmd.Context(), args[0], tap)
			if err != nil {
				return err
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			fmt.Println(string(data))
			return nil
		},
	}
	cmd.Flags().StringVar(&tap, "tap", "", "Tap name override")
	return cmd
}

func newRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			if err := mgr.Remove(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Removed %s\n", args[0])
			return nil
		},
	}
	return cmd
}

func newUpgradeCmd() *cobra.Command {
	var major bool
	cmd := &cobra.Command{
		Use:   "upgrade [name]",
		Short: "Upgrade installed package(s)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			res, err := mgr.Upgrade(cmd.Context(), name, major)
			if err != nil {
				return err
			}
			for _, r := range res {
				if r.WasUpgraded {
					fmt.Printf("Upgraded %s %s -> %s\n", r.Name, r.OldVersion, r.NewVersion)
				} else {
					fmt.Printf("No change %s (%s)\n", r.Name, r.OldVersion)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&major, "major", false, "Allow major version upgrades")
	return cmd
}

func newDoctorCmd() *cobra.Command {
	var fix bool
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate installed MCP packages and config health",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			issues, err := mgr.Doctor(cmd.Context(), fix)
			if err != nil {
				return err
			}
			if asJSON {
				data, _ := json.MarshalIndent(issues, "", "  ")
				fmt.Println(string(data))
			} else if len(issues) == 0 {
				fmt.Println("doctor: no issues found")
			} else {
				for _, issue := range issues {
					fmt.Printf("[%s] package=%s target=%s detail=%s\n", issue.Kind, issue.Package, issue.Target, issue.Detail)
				}
			}
			if len(issues) > 0 {
				return errors.New("doctor found issues")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Attempt auto-fixes for missing config entries")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	return cmd
}

func newExportCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export lockfile or SBOM from current installed state",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			payload, err := mgr.Export(format)
			if err != nil {
				return err
			}
			fmt.Println(string(payload))
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "lock", "Export format: lock or sbom")
	return cmd
}

func newTapCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tap", Short: "Manage package taps"}
	cmd.AddCommand(newTapAddCmd(), newTapRemoveCmd(), newTapListCmd())
	return cmd
}

func newTapAddCmd() *cobra.Command {
	var description string
	cmd := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add or update a package tap",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			return mgr.TapAdd(service.TapAddRequest{
				Name:        args[0],
				URL:         args[1],
				Description: description,
			})
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "Tap description")
	return cmd
}

func newTapRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a configured tap",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			return mgr.TapRemove(args[0])
		},
	}
	return cmd
}

func newTapListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured taps",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			taps, err := mgr.TapList()
			if err != nil {
				return err
			}
			for _, tap := range taps {
				fmt.Printf("%s\t%s\tmode=%s\n", tap.Name, tap.URL, tap.Trust.Mode)
			}
			return nil
		},
	}
	return cmd
}

func newSecretCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "secret", Short: "Manage package secrets in OS keychain"}
	cmd.AddCommand(newSecretSetCmd(), newSecretUnsetCmd())
	return cmd
}

func newSecretSetCmd() *cobra.Command {
	var value string
	cmd := &cobra.Command{
		Use:   "set <package> <ENV_NAME>",
		Short: "Set a secret value in keychain",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			if value == "" {
				fmt.Fprint(os.Stdout, "Enter secret value: ")
				var input string
				if _, err := fmt.Fscanln(os.Stdin, &input); err != nil {
					return err
				}
				value = input
			}
			return mgr.SecretSet(args[0], args[1], value)
		},
	}
	cmd.Flags().StringVar(&value, "value", "", "Secret value (omit to prompt)")
	return cmd
}

func newSecretUnsetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unset <package> <ENV_NAME>",
		Short: "Delete a secret from keychain",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := managerOrDie()
			if err != nil {
				return err
			}
			return mgr.SecretUnset(args[0], args[1])
		},
	}
	return cmd
}

func splitNameVersion(raw string) (string, string) {
	parts := strings.SplitN(raw, "@", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func Execute() error {
	ctx := context.Background()
	root := NewRootCmd()
	root.SetContext(ctx)
	return root.Execute()
}
