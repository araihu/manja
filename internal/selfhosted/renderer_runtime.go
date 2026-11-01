package selfhosted

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	configadapter "github.com/araihu/manja/internal/adapters/config"
	"github.com/araihu/manja/renderer"
)

type RendererOptions struct {
	ConfigPath        string
	DataDir           string
	LocalDocsDisabled bool
	ResourceLimits    bool
}

func closeRendererHandler(handler http.Handler) error {
	if closer, ok := handler.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// NewRecoveredRenderer opens only durable renderer state. It never constructs
// source adapters, parsers, or compilers and fails unless every configured
// catalog has an active, preflighted snapshot.
// The returned handler implements io.Closer and owns the data-directory lock.
func NewRecoveredRenderer(ctx context.Context, options RendererOptions) (http.Handler, []renderer.ActivationReceipt, error) {
	configured, runtimeConfig, err := rendererConfiguration(options)
	if err != nil {
		return nil, nil, err
	}
	server, err := renderer.NewRecoveryOnly(runtimeConfig)
	if err != nil {
		return nil, nil, err
	}
	ready := false
	defer func() {
		if !ready {
			_ = closeRendererHandler(server.Handler())
		}
	}()
	if err := server.Recover(ctx); err != nil {
		return nil, nil, err
	}
	receipts := make([]renderer.ActivationReceipt, 0, len(configured.Catalogs))
	for _, catalog := range configured.Catalogs {
		receipt, active := server.Active(catalog.ID)
		if !active || receipt.SnapshotID == "" {
			return nil, nil, fmt.Errorf("catalog %q has no active snapshot", catalog.ID)
		}
		receipts = append(receipts, receipt)
	}
	handler, err := withRendererBrandAssets(ctx, configured.AssetDirectory(), server.Handler())
	if err != nil {
		return nil, nil, err
	}
	ready = true
	return handler, receipts, nil
}

func rendererConfiguration(options RendererOptions) (configadapter.RendererFile, renderer.Config, error) {
	if strings.TrimSpace(options.ConfigPath) == "" {
		return configadapter.RendererFile{}, renderer.Config{}, fmt.Errorf("renderer config path is required")
	}
	configured, err := configadapter.LoadRenderer(options.ConfigPath)
	if err != nil {
		return configadapter.RendererFile{}, renderer.Config{}, err
	}
	runtimeConfig := configured.RuntimeConfig()
	runtimeConfig.LocalDocsDisabled = options.LocalDocsDisabled
	runtimeConfig.ResourceLimits = options.ResourceLimits
	if strings.TrimSpace(options.DataDir) != "" {
		runtimeConfig.DataDir = options.DataDir
	}
	return configured, runtimeConfig, nil
}
