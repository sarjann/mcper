package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	semver "github.com/Masterminds/semver/v3"

	"github.com/sarjann/mcper/internal/fsutil"
	"github.com/sarjann/mcper/internal/model"
	"github.com/sarjann/mcper/internal/paths"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

type TapSnapshot struct {
	Tap       model.TapConfig
	LocalPath string
	Index     model.RegistryIndex
	IndexRaw  []byte
}

func (c *Client) SyncTap(ctx context.Context, tap model.TapConfig) (TapSnapshot, error) {
	localPath, err := c.materializeTap(ctx, tap)
	if err != nil {
		return TapSnapshot{}, err
	}

	indexPath := filepath.Join(localPath, "index.json")
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		return TapSnapshot{}, fmt.Errorf("read index for tap %q: %w", tap.Name, err)
	}

	if err := VerifyTapIndex(ctx, tap, localPath, indexRaw); err != nil {
		return TapSnapshot{}, err
	}

	var idx model.RegistryIndex
	if err := json.Unmarshal(indexRaw, &idx); err != nil {
		return TapSnapshot{}, fmt.Errorf("decode tap index %q: %w", tap.Name, err)
	}
	if idx.Packages == nil {
		idx.Packages = map[string]model.IndexPackage{}
	}

	return TapSnapshot{Tap: tap, LocalPath: localPath, Index: idx, IndexRaw: indexRaw}, nil
}

func (c *Client) materializeTap(ctx context.Context, tap model.TapConfig) (string, error) {
	if tap.URL == "" {
		return "", fmt.Errorf("tap %q has empty URL", tap.Name)
	}

	if strings.HasPrefix(tap.URL, "file://") {
		p := strings.TrimPrefix(tap.URL, "file://")
		return p, nil
	}
	if fi, err := os.Stat(tap.URL); err == nil && fi.IsDir() {
		return tap.URL, nil
	}

	cacheDir, err := paths.TapCacheDir(tap.Name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o755); err != nil {
		return "", fmt.Errorf("create tap cache parent: %w", err)
	}

	if _, err := os.Stat(cacheDir); err == nil {
		cmd := exec.CommandContext(ctx, "git", "-C", cacheDir, "pull", "--ff-only")
		out, runErr := cmd.CombinedOutput()
		if runErr == nil {
			return cacheDir, nil
		}
		_ = os.RemoveAll(cacheDir)
		_ = out
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", tap.URL, cacheDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("clone tap %q from %q: %w (%s)", tap.Name, tap.URL, err, strings.TrimSpace(string(out)))
	}
	return cacheDir, nil
}

type SearchResult struct {
	Tap         string `json:"tap"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Latest      string `json:"latest"`
}

func (c *Client) Search(ctx context.Context, taps map[string]model.TapConfig, query string) ([]SearchResult, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	results := make([]SearchResult, 0)
	for _, tap := range taps {
		snap, err := c.SyncTap(ctx, tap)
		if err != nil {
			continue
		}
		for name, pkg := range snap.Index.Packages {
			if query != "" {
				hay := strings.ToLower(name + " " + pkg.Description)
				if !strings.Contains(hay, query) {
					continue
				}
			}
			latest, _ := latestVersion(pkg)
			results = append(results, SearchResult{
				Tap:         tap.Name,
				Name:        name,
				Description: pkg.Description,
				Latest:      latest,
			})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Name == results[j].Name {
			return results[i].Tap < results[j].Tap
		}
		return results[i].Name < results[j].Name
	})
	return results, nil
}

type ResolvedPackage struct {
	Manifest       model.PackageManifest
	ManifestRaw    []byte
	ManifestDigest string
	Tap            model.TapConfig
	Version        string
}

func (c *Client) ResolveFromTap(ctx context.Context, tap model.TapConfig, name, versionExpr string) (ResolvedPackage, error) {
	snap, err := c.SyncTap(ctx, tap)
	if err != nil {
		return ResolvedPackage{}, err
	}
	pkg, ok := snap.Index.Packages[name]
	if !ok {
		return ResolvedPackage{}, fmt.Errorf("package %q not found in tap %q", name, tap.Name)
	}

	resolvedVersion, meta, err := resolveVersion(pkg, versionExpr)
	if err != nil {
		return ResolvedPackage{}, err
	}

	manifestPath := filepath.Join(snap.LocalPath, meta.ManifestPath)
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return ResolvedPackage{}, fmt.Errorf("read manifest %s: %w", manifestPath, err)
	}
	if meta.SHA256 != "" {
		actual := fsutil.SHA256Hex(manifestRaw)
		if !strings.EqualFold(actual, meta.SHA256) {
			return ResolvedPackage{}, fmt.Errorf("manifest hash mismatch for %s@%s: expected %s got %s", name, resolvedVersion, meta.SHA256, actual)
		}
	}
	if err := VerifyManifest(ctx, tap, snap.LocalPath, manifestPath, meta); err != nil {
		return ResolvedPackage{}, err
	}

	var mf model.PackageManifest
	if err := json.Unmarshal(manifestRaw, &mf); err != nil {
		return ResolvedPackage{}, fmt.Errorf("decode manifest %s: %w", manifestPath, err)
	}
	if err := validateManifest(mf); err != nil {
		return ResolvedPackage{}, err
	}

	return ResolvedPackage{
		Manifest:       mf,
		ManifestRaw:    manifestRaw,
		ManifestDigest: fsutil.SHA256Hex(manifestRaw),
		Tap:            tap,
		Version:        resolvedVersion,
	}, nil
}

func (c *Client) ResolveUpgrade(ctx context.Context, tap model.TapConfig, name, currentVersion string, allowMajor bool) (ResolvedPackage, bool, error) {
	snap, err := c.SyncTap(ctx, tap)
	if err != nil {
		return ResolvedPackage{}, false, err
	}
	_, ok := snap.Index.Packages[name]
	if !ok {
		return ResolvedPackage{}, false, fmt.Errorf("package %q not found in tap %q", name, tap.Name)
	}

	targetExpr := ""
	if !allowMajor {
		v, err := semver.NewVersion(currentVersion)
		if err != nil {
			return ResolvedPackage{}, false, fmt.Errorf("parse current version %q: %w", currentVersion, err)
		}
		nextMajor := v.Major() + 1
		targetExpr = fmt.Sprintf(">=%s, <%d.0.0", currentVersion, nextMajor)
	}

	resolved, err := c.ResolveFromTap(ctx, tap, name, targetExpr)
	if err != nil {
		return ResolvedPackage{}, false, err
	}

	cmpCurrent, err := semver.NewVersion(currentVersion)
	if err != nil {
		return ResolvedPackage{}, false, fmt.Errorf("parse current version %q: %w", currentVersion, err)
	}
	cmpResolved, err := semver.NewVersion(resolved.Version)
	if err != nil {
		return ResolvedPackage{}, false, fmt.Errorf("parse resolved version %q: %w", resolved.Version, err)
	}
	if !cmpResolved.GreaterThan(cmpCurrent) {
		return ResolvedPackage{}, false, nil
	}

	return resolved, true, nil
}

func (c *Client) ResolveFromURL(ctx context.Context, url string) (ResolvedPackage, error) {
	_ = ctx
	data, err := c.readURLOrFile(url)
	if err != nil {
		return ResolvedPackage{}, err
	}
	var mf model.PackageManifest
	if err := json.Unmarshal(data, &mf); err != nil {
		return ResolvedPackage{}, fmt.Errorf("decode manifest from %q: %w", url, err)
	}
	if err := validateManifest(mf); err != nil {
		return ResolvedPackage{}, err
	}

	return ResolvedPackage{
		Manifest:       mf,
		ManifestRaw:    data,
		ManifestDigest: fsutil.SHA256Hex(data),
		Version:        mf.Version,
	}, nil
}

func (c *Client) readURLOrFile(raw string) ([]byte, error) {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch %q: %w", raw, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("fetch %q: status %s", raw, resp.Status)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response from %q: %w", raw, err)
		}
		return data, nil
	}
	if strings.HasPrefix(raw, "file://") {
		raw = strings.TrimPrefix(raw, "file://")
	}
	return os.ReadFile(raw)
}

func resolveVersion(pkg model.IndexPackage, versionExpr string) (string, model.IndexVersion, error) {
	if len(pkg.Versions) == 0 {
		return "", model.IndexVersion{}, errors.New("package has no versions")
	}

	if versionExpr != "" && !strings.ContainsAny(versionExpr, "<>=~^,") {
		meta, ok := pkg.Versions[versionExpr]
		if !ok {
			return "", model.IndexVersion{}, fmt.Errorf("version %q not found", versionExpr)
		}
		return versionExpr, meta, nil
	}

	constraintExpr := ">=0.0.0"
	if versionExpr != "" {
		constraintExpr = versionExpr
	}
	constraint, err := semver.NewConstraint(constraintExpr)
	if err != nil {
		return "", model.IndexVersion{}, fmt.Errorf("parse version constraint %q: %w", versionExpr, err)
	}

	versions := make([]*semver.Version, 0, len(pkg.Versions))
	lookup := map[string]model.IndexVersion{}
	for raw, meta := range pkg.Versions {
		v, err := semver.NewVersion(raw)
		if err != nil {
			continue
		}
		versions = append(versions, v)
		lookup[v.Original()] = meta
	}
	sort.Sort(sort.Reverse(semver.Collection(versions)))
	for _, v := range versions {
		if constraint.Check(v) {
			return v.Original(), lookup[v.Original()], nil
		}
	}
	return "", model.IndexVersion{}, fmt.Errorf("no version satisfies constraint %q", constraintExpr)
}

func latestVersion(pkg model.IndexPackage) (string, error) {
	v, _, err := resolveVersion(pkg, ">=0.0.0")
	return v, err
}

func validateManifest(m model.PackageManifest) error {
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("manifest missing name")
	}
	if strings.TrimSpace(m.Version) == "" {
		return errors.New("manifest missing version")
	}
	if len(m.MCPServers) == 0 {
		return errors.New("manifest has no mcp_servers")
	}
	for name, server := range m.MCPServers {
		if strings.TrimSpace(name) == "" {
			return errors.New("manifest has empty server name")
		}
		switch server.Transport {
		case model.ServerTransportSTDIO:
			if strings.TrimSpace(server.Command) == "" {
				return fmt.Errorf("server %q missing command for stdio transport", name)
			}
		case model.ServerTransportHTTP:
			if strings.TrimSpace(server.URL) == "" {
				return fmt.Errorf("server %q missing url for http transport", name)
			}
		default:
			return fmt.Errorf("server %q has unsupported transport %q", name, server.Transport)
		}
	}
	for envVar, sc := range m.SetupCommands {
		if len(sc.Run) == 0 {
			return fmt.Errorf("setup_command for %q has empty run", envVar)
		}
		if sc.Pattern != "" {
			if _, err := regexp.Compile(sc.Pattern); err != nil {
				return fmt.Errorf("setup_command for %q has invalid pattern: %w", envVar, err)
			}
		}
	}
	return nil
}
