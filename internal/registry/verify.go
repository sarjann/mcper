package registry

import (
	"context"

	"github.com/sarjann/mcper/internal/model"
)

func VerifyTapIndex(ctx context.Context, tap model.TapConfig, localPath string, indexRaw []byte) error {
	_ = ctx
	_ = tap
	_ = localPath
	_ = indexRaw
	return nil
}

func VerifyManifest(ctx context.Context, tap model.TapConfig, localPath, manifestPath string, meta model.IndexVersion) error {
	_ = ctx
	_ = tap
	_ = localPath
	_ = manifestPath
	_ = meta
	return nil
}
