package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/sarjann/mcper/internal/adapters"
	"github.com/sarjann/mcper/internal/model"
	"github.com/sarjann/mcper/internal/registry"
	"github.com/sarjann/mcper/internal/secrets"
	"github.com/sarjann/mcper/internal/state"
)

type Manager struct {
	store    *state.Store
	registry *registry.Client
	secret   secrets.Store
	adapters map[string]adapters.Adapter
	stdin    io.Reader
	stdout   io.Writer
}

func NewManager(stdin io.Reader, stdout io.Writer) (*Manager, error) {
	st, err := state.NewStore()
	if err != nil {
		return nil, err
	}
	codexAdapter, err := adapters.NewCodexAdapter()
	if err != nil {
		return nil, err
	}
	claudeAdapter, err := adapters.NewClaudeAdapter()
	if err != nil {
		return nil, err
	}

	return &Manager{
		store:    st,
		registry: registry.NewClient(),
		secret:   secrets.NewKeyringStore(),
		adapters: map[string]adapters.Adapter{
			model.TargetCodex:  codexAdapter,
			model.TargetClaude: claudeAdapter,
		},
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

type InstallRequest struct {
	Name    string
	Version string
	Tap     string
	Target  string
}

type InstallURLRequest struct {
	URL    string
	Target string
	Yes    bool
}

func (m *Manager) InstallFromTap(ctx context.Context, req InstallRequest) (model.InstalledPackage, error) {
	st, err := m.store.Load()
	if err != nil {
		return model.InstalledPackage{}, err
	}

	tapName := req.Tap
	if tapName == "" {
		tapName = model.DefaultTapName
	}
	tap, ok := st.Taps[tapName]
	if !ok {
		return model.InstalledPackage{}, fmt.Errorf("tap %q not found", tapName)
	}

	resolved, err := m.registry.ResolveFromTap(ctx, tap, req.Name, req.Version)
	if err != nil {
		return model.InstalledPackage{}, err
	}

	installed, err := m.applyInstall(ctx, st, resolved.Manifest, resolved.ManifestDigest, model.SourceRef{
		Type: model.SourceTypeTap,
		Tap:  tap.Name,
	}, req.Target)
	if err != nil {
		return model.InstalledPackage{}, err
	}

	installed.Version = resolved.Version
	installed.Description = resolved.Manifest.Description
	installed.Source = model.SourceRef{Type: model.SourceTypeTap, Tap: tap.Name}
	installed.ManifestDigest = resolved.ManifestDigest
	installed.UpdatedAt = time.Now().UTC()
	if installed.InstalledAt.IsZero() {
		installed.InstalledAt = installed.UpdatedAt
	}
	st.Installed[installed.Name] = installed
	if err := m.store.Save(st); err != nil {
		return model.InstalledPackage{}, err
	}
	return installed, nil
}

func (m *Manager) InstallFromURL(ctx context.Context, req InstallURLRequest) (model.InstalledPackage, error) {
	st, err := m.store.Load()
	if err != nil {
		return model.InstalledPackage{}, err
	}

	trusted := st.TrustedDirectSources[req.URL]
	if !trusted.Approved {
		if !req.Yes {
			approved, err := m.promptTrust(req.URL)
			if err != nil {
				return model.InstalledPackage{}, err
			}
			if !approved {
				return model.InstalledPackage{}, errors.New("direct source not trusted")
			}
		}
		st.TrustedDirectSources[req.URL] = model.TrustDecision{
			URL:       req.URL,
			Approved:  true,
			CreatedAt: time.Now().UTC(),
		}
	}

	resolved, err := m.registry.ResolveFromURL(ctx, req.URL)
	if err != nil {
		return model.InstalledPackage{}, err
	}

	installed, err := m.applyInstall(ctx, st, resolved.Manifest, resolved.ManifestDigest, model.SourceRef{
		Type: model.SourceTypeDirect,
		URL:  req.URL,
	}, req.Target)
	if err != nil {
		return model.InstalledPackage{}, err
	}
	installed.Version = resolved.Manifest.Version
	installed.Description = resolved.Manifest.Description
	installed.Source = model.SourceRef{Type: model.SourceTypeDirect, URL: req.URL}
	installed.ManifestDigest = resolved.ManifestDigest
	installed.UpdatedAt = time.Now().UTC()
	if installed.InstalledAt.IsZero() {
		installed.InstalledAt = installed.UpdatedAt
	}
	st.Installed[installed.Name] = installed

	if err := m.store.Save(st); err != nil {
		return model.InstalledPackage{}, err
	}
	return installed, nil
}

func (m *Manager) applyInstall(ctx context.Context, st model.State, manifest model.PackageManifest, digest string, source model.SourceRef, target string) (model.InstalledPackage, error) {
	targets, err := m.resolveTargets(target)
	if err != nil {
		return model.InstalledPackage{}, err
	}

	applied := make([]adapters.Adapter, 0, len(targets))
	for _, targetName := range targets {
		adapter := m.adapters[targetName]
		if err := adapter.UpsertServers(ctx, manifest.MCPServers); err != nil {
			for i := len(applied) - 1; i >= 0; i-- {
				_ = applied[i].RemoveServers(ctx, keys(manifest.MCPServers))
			}
			return model.InstalledPackage{}, fmt.Errorf("apply %s config: %w", targetName, err)
		}
		applied = append(applied, adapter)
	}

	now := time.Now().UTC()
	cur := st.Installed[manifest.Name]
	if cur.Name == "" {
		cur.Name = manifest.Name
		cur.InstalledAt = now
	}
	cur.Version = manifest.Version
	cur.Description = manifest.Description
	cur.Source = source
	cur.ManifestDigest = digest
	cur.Servers = keys(manifest.MCPServers)
	cur.Targets = targets
	cur.UpdatedAt = now

	return cur, nil
}

func (m *Manager) Remove(ctx context.Context, name string) error {
	st, err := m.store.Load()
	if err != nil {
		return err
	}
	pkg, ok := st.Installed[name]
	if !ok {
		return fmt.Errorf("package %q is not installed", name)
	}

	for _, target := range pkg.Targets {
		adapter, ok := m.adapters[target]
		if !ok {
			continue
		}
		if err := adapter.RemoveServers(ctx, pkg.Servers); err != nil {
			return fmt.Errorf("remove servers from %s: %w", target, err)
		}
	}

	delete(st.Installed, name)
	return m.store.Save(st)
}

func (m *Manager) ListInstalled() ([]model.InstalledPackage, error) {
	st, err := m.store.Load()
	if err != nil {
		return nil, err
	}
	items := make([]model.InstalledPackage, 0, len(st.Installed))
	for _, pkg := range st.Installed {
		items = append(items, pkg)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (m *Manager) Search(ctx context.Context, query string) ([]registry.SearchResult, error) {
	st, err := m.store.Load()
	if err != nil {
		return nil, err
	}
	return m.registry.Search(ctx, st.Taps, query)
}

func (m *Manager) Info(ctx context.Context, name, tapName string) (model.PackageManifest, error) {
	st, err := m.store.Load()
	if err != nil {
		return model.PackageManifest{}, err
	}
	if pkg, ok := st.Installed[name]; ok {
		if pkg.Source.Type == model.SourceTypeTap && pkg.Source.Tap != "" {
			tapName = pkg.Source.Tap
		}
	}
	if tapName == "" {
		tapName = model.DefaultTapName
	}
	tap, ok := st.Taps[tapName]
	if !ok {
		return model.PackageManifest{}, fmt.Errorf("tap %q not found", tapName)
	}
	resolved, err := m.registry.ResolveFromTap(ctx, tap, name, "")
	if err != nil {
		return model.PackageManifest{}, err
	}
	return resolved.Manifest, nil
}

type UpgradeResult struct {
	Name        string
	OldVersion  string
	NewVersion  string
	WasUpgraded bool
}

func (m *Manager) Upgrade(ctx context.Context, name string, allowMajor bool) ([]UpgradeResult, error) {
	st, err := m.store.Load()
	if err != nil {
		return nil, err
	}

	candidates := make([]model.InstalledPackage, 0, len(st.Installed))
	if name != "" {
		pkg, ok := st.Installed[name]
		if !ok {
			return nil, fmt.Errorf("package %q is not installed", name)
		}
		candidates = append(candidates, pkg)
	} else {
		for _, pkg := range st.Installed {
			candidates = append(candidates, pkg)
		}
	}

	results := make([]UpgradeResult, 0, len(candidates))
	for _, pkg := range candidates {
		if pkg.Source.Type != model.SourceTypeTap || pkg.Source.Tap == "" {
			results = append(results, UpgradeResult{Name: pkg.Name, OldVersion: pkg.Version, NewVersion: pkg.Version, WasUpgraded: false})
			continue
		}
		tap, ok := st.Taps[pkg.Source.Tap]
		if !ok {
			return nil, fmt.Errorf("tap %q used by package %q is no longer configured", pkg.Source.Tap, pkg.Name)
		}
		resolved, hasUpgrade, err := m.registry.ResolveUpgrade(ctx, tap, pkg.Name, pkg.Version, allowMajor)
		if err != nil {
			return nil, err
		}
		if !hasUpgrade {
			results = append(results, UpgradeResult{Name: pkg.Name, OldVersion: pkg.Version, NewVersion: pkg.Version, WasUpgraded: false})
			continue
		}

		oldVersion := pkg.Version
		if _, err := m.applyInstall(ctx, st, resolved.Manifest, resolved.ManifestDigest, model.SourceRef{
			Type: model.SourceTypeTap,
			Tap:  tap.Name,
		}, strings.Join(pkg.Targets, ",")); err != nil {
			return nil, err
		}

		now := time.Now().UTC()
		pkg.Version = resolved.Version
		pkg.Description = resolved.Manifest.Description
		pkg.ManifestDigest = resolved.ManifestDigest
		pkg.Servers = keys(resolved.Manifest.MCPServers)
		pkg.UpdatedAt = now
		st.Installed[pkg.Name] = pkg
		results = append(results, UpgradeResult{Name: pkg.Name, OldVersion: oldVersion, NewVersion: resolved.Version, WasUpgraded: true})
	}

	if err := m.store.Save(st); err != nil {
		return nil, err
	}
	return results, nil
}

type DoctorIssue struct {
	Package string `json:"package"`
	Target  string `json:"target"`
	Kind    string `json:"kind"`
	Detail  string `json:"detail"`
}

func (m *Manager) Doctor(ctx context.Context, fix bool) ([]DoctorIssue, error) {
	st, err := m.store.Load()
	if err != nil {
		return nil, err
	}

	issues := make([]DoctorIssue, 0)
	for _, pkg := range st.Installed {
		manifest, err := m.resolveManifestForInstalled(ctx, st, pkg)
		if err != nil {
			issues = append(issues, DoctorIssue{Package: pkg.Name, Kind: "manifest", Detail: err.Error()})
			continue
		}

		for _, target := range pkg.Targets {
			adapter := m.adapters[target]
			servers, err := adapter.ListServers(ctx)
			if err != nil {
				issues = append(issues, DoctorIssue{Package: pkg.Name, Target: target, Kind: "adapter", Detail: err.Error()})
				continue
			}
			missing := make(map[string]model.MCPServerSpec)
			for serverName, expected := range manifest.MCPServers {
				actual, ok := servers[serverName]
				if !ok {
					issues = append(issues, DoctorIssue{Package: pkg.Name, Target: target, Kind: "missing_server", Detail: serverName})
					missing[serverName] = expected
					continue
				}
				if expected.Transport == model.ServerTransportSTDIO {
					if _, err := exec.LookPath(actual.Command); err != nil {
						issues = append(issues, DoctorIssue{Package: pkg.Name, Target: target, Kind: "missing_command", Detail: fmt.Sprintf("%s (%s)", serverName, actual.Command)})
					}
				}
				for _, env := range expected.EnvRequired {
					if _, err := m.secret.Get(pkg.Name, env); err != nil {
						if errors.Is(err, keyring.ErrNotFound) {
							issues = append(issues, DoctorIssue{Package: pkg.Name, Target: target, Kind: "missing_secret", Detail: fmt.Sprintf("%s:%s", serverName, env)})
						}
					}
				}
			}
			if fix && len(missing) > 0 {
				if err := adapter.UpsertServers(ctx, missing); err != nil {
					issues = append(issues, DoctorIssue{Package: pkg.Name, Target: target, Kind: "fix_failed", Detail: err.Error()})
				}
			}
		}
	}

	return issues, nil
}

func (m *Manager) resolveManifestForInstalled(ctx context.Context, st model.State, pkg model.InstalledPackage) (model.PackageManifest, error) {
	switch pkg.Source.Type {
	case model.SourceTypeTap:
		tap, ok := st.Taps[pkg.Source.Tap]
		if !ok {
			return model.PackageManifest{}, fmt.Errorf("tap %q not configured", pkg.Source.Tap)
		}
		resolved, err := m.registry.ResolveFromTap(ctx, tap, pkg.Name, pkg.Version)
		if err != nil {
			return model.PackageManifest{}, err
		}
		return resolved.Manifest, nil
	case model.SourceTypeDirect:
		resolved, err := m.registry.ResolveFromURL(ctx, pkg.Source.URL)
		if err != nil {
			return model.PackageManifest{}, err
		}
		return resolved.Manifest, nil
	default:
		return model.PackageManifest{}, fmt.Errorf("unknown source type %q", pkg.Source.Type)
	}
}

func (m *Manager) Export(format string) ([]byte, error) {
	st, err := m.store.Load()
	if err != nil {
		return nil, err
	}
	pkgs := make([]model.InstalledPackage, 0, len(st.Installed))
	for _, pkg := range st.Installed {
		pkgs = append(pkgs, pkg)
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].Name < pkgs[j].Name
	})

	switch format {
	case "lock":
		lock := model.Lockfile{
			SchemaVersion: 1,
			GeneratedAt:   time.Now().UTC(),
			Packages:      pkgs,
		}
		return json.MarshalIndent(lock, "", "  ")
	case "sbom":
		items := make([]model.SBOMItem, 0, len(pkgs))
		for _, pkg := range pkgs {
			items = append(items, model.SBOMItem{Name: pkg.Name, Version: pkg.Version, Source: pkg.Source})
		}
		sbom := model.SBOM{SchemaVersion: 1, GeneratedAt: time.Now().UTC(), Components: items}
		return json.MarshalIndent(sbom, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported export format %q", format)
	}
}

type TapAddRequest struct {
	Name        string
	URL         string
	Description string
}

func (m *Manager) TapAdd(req TapAddRequest) error {
	st, err := m.store.Load()
	if err != nil {
		return err
	}
	if req.Name == "" {
		return errors.New("tap name is required")
	}
	if req.URL == "" {
		return errors.New("tap url is required")
	}
	trust := model.TapTrustConfig{Mode: model.TrustModeHash}

	now := time.Now().UTC()
	st.Taps[req.Name] = model.TapConfig{
		Name:        req.Name,
		URL:         req.URL,
		Description: req.Description,
		Trust:       trust,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return m.store.Save(st)
}

func (m *Manager) TapRemove(name string) error {
	if name == model.DefaultTapName {
		return errors.New("cannot remove default tap")
	}
	st, err := m.store.Load()
	if err != nil {
		return err
	}
	if _, ok := st.Taps[name]; !ok {
		return fmt.Errorf("tap %q not found", name)
	}
	delete(st.Taps, name)
	return m.store.Save(st)
}

func (m *Manager) TapList() ([]model.TapConfig, error) {
	st, err := m.store.Load()
	if err != nil {
		return nil, err
	}
	items := make([]model.TapConfig, 0, len(st.Taps))
	for _, tap := range st.Taps {
		items = append(items, tap)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (m *Manager) SecretSet(pkg, key, value string) error {
	if pkg == "" || key == "" {
		return errors.New("package and key are required")
	}
	if value == "" {
		return errors.New("secret value is empty")
	}
	return m.secret.Set(pkg, key, value)
}

func (m *Manager) SecretUnset(pkg, key string) error {
	if pkg == "" || key == "" {
		return errors.New("package and key are required")
	}
	return m.secret.Delete(pkg, key)
}

func (m *Manager) resolveTargets(target string) ([]string, error) {
	target = strings.TrimSpace(target)
	if target == "" || target == model.TargetAll {
		return []string{model.TargetCodex, model.TargetClaude}, nil
	}

	parts := strings.Split(target, ",")
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.ToLower(strings.TrimSpace(part))
		if p == "" {
			continue
		}
		if p != model.TargetCodex && p != model.TargetClaude {
			return nil, fmt.Errorf("unknown target %q", p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no valid targets specified")
	}
	return out, nil
}

func (m *Manager) promptTrust(url string) (bool, error) {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false, fmt.Errorf("inspect stdin: %w", err)
	}
	if fi.Mode()&os.ModeCharDevice == 0 {
		return false, errors.New("direct URL trust requires --yes in non-interactive mode")
	}

	fmt.Fprintf(m.stdout, "Direct source trust required for %s\n", url)
	fmt.Fprint(m.stdout, "Type 'yes' to trust this source: ")
	reader := bufio.NewReader(m.stdin)
	resp, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read trust response: %w", err)
	}
	return strings.EqualFold(strings.TrimSpace(resp), "yes"), nil
}

func keys(m map[string]model.MCPServerSpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
