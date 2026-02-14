package registry

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sarjann/mcper/internal/model"
)

func VerifyTapIndex(ctx context.Context, tap model.TapConfig, localPath string, indexRaw []byte) error {
	_ = indexRaw
	switch tap.Trust.Mode {
	case "", model.TrustModeHash:
		return nil
	case model.TrustModeCosign:
		if len(tap.Trust.Identities) == 0 {
			return fmt.Errorf("tap %q trust mode is cosign but no trusted identities configured", tap.Name)
		}
		indexPath := filepath.Join(localPath, "index.json")
		sigPath := filepath.Join(localPath, "index.json.sig")
		certPath := filepath.Join(localPath, "index.json.pem")
		if _, err := os.Stat(sigPath); err != nil {
			return fmt.Errorf("missing signature file for tap %q: %s", tap.Name, sigPath)
		}
		if _, err := os.Stat(certPath); err != nil {
			return fmt.Errorf("missing certificate file for tap %q: %s", tap.Name, certPath)
		}
		if err := verifyWithAnyIdentity(ctx, indexPath, sigPath, certPath, tap.Trust.Identities); err != nil {
			return fmt.Errorf("verify index signature for tap %q: %w", tap.Name, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported trust mode %q for tap %q", tap.Trust.Mode, tap.Name)
	}
}

func VerifyManifest(ctx context.Context, tap model.TapConfig, localPath, manifestPath string, meta model.IndexVersion) error {
	switch tap.Trust.Mode {
	case "", model.TrustModeHash:
		return nil
	case model.TrustModeCosign:
		if len(tap.Trust.Identities) == 0 {
			return fmt.Errorf("tap %q trust mode is cosign but no trusted identities configured", tap.Name)
		}
		sigPath := meta.SigPath
		if sigPath == "" {
			sigPath = strings.TrimSuffix(manifestPath, filepath.Ext(manifestPath)) + ".sig"
		}
		certPath := meta.CertPath
		if certPath == "" {
			certPath = strings.TrimSuffix(manifestPath, filepath.Ext(manifestPath)) + ".pem"
		}
		if !filepath.IsAbs(sigPath) {
			sigPath = filepath.Join(localPath, sigPath)
		}
		if !filepath.IsAbs(certPath) {
			certPath = filepath.Join(localPath, certPath)
		}
		if _, err := os.Stat(sigPath); err != nil {
			return fmt.Errorf("missing signature for manifest %s: %s", manifestPath, sigPath)
		}
		if _, err := os.Stat(certPath); err != nil {
			return fmt.Errorf("missing certificate for manifest %s: %s", manifestPath, certPath)
		}
		if err := verifyWithAnyIdentity(ctx, manifestPath, sigPath, certPath, tap.Trust.Identities); err != nil {
			return fmt.Errorf("verify manifest signature for %s: %w", manifestPath, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported trust mode %q for tap %q", tap.Trust.Mode, tap.Name)
	}
}

func verifyWithAnyIdentity(ctx context.Context, payloadPath, sigPath, certPath string, ids []model.SigstoreIdentity) error {
	if _, err := exec.LookPath("cosign"); err != nil {
		return fmt.Errorf("cosign is not installed: %w", err)
	}

	var errs []string
	for _, id := range ids {
		args := []string{
			"verify-blob",
			"--signature", sigPath,
			"--certificate", certPath,
			"--certificate-identity", id.Subject,
			"--certificate-oidc-issuer", id.Issuer,
			payloadPath,
		}
		cmd := exec.CommandContext(ctx, "cosign", args...)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		errs = append(errs, strings.TrimSpace(string(out)))
	}
	return fmt.Errorf("no trusted identity matched. errors: %s", strings.Join(errs, " | "))
}
