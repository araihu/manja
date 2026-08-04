//go:build !manja_runtime

package selfhosted

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/araihu/manja/renderer"
)

// BuildRenderer compiles every configured source and durably activates the
// resulting snapshots. Any degraded catalog fails the build: a published
// image must never silently reuse stale or incomplete state.
func BuildRenderer(ctx context.Context, options RendererOptions) ([]renderer.ActivationReceipt, error) {
	_, receipts, err := NewRenderer(ctx, options)
	if err != nil {
		return nil, err
	}
	for _, receipt := range receipts {
		if receipt.Degraded {
			return nil, fmt.Errorf("build catalog %q: %s", receipt.CatalogID, receipt.Diagnostic)
		}
		if receipt.SnapshotID == "" {
			return nil, fmt.Errorf("build catalog %q produced no active snapshot", receipt.CatalogID)
		}
	}
	return receipts, nil
}

func NewRenderer(ctx context.Context, options RendererOptions) (http.Handler, []renderer.ActivationReceipt, error) {
	configured, runtimeConfig, err := rendererConfiguration(options)
	if err != nil {
		return nil, nil, err
	}
	server, err := renderer.New(runtimeConfig)
	if err != nil {
		return nil, nil, err
	}
	if err := server.Recover(ctx); err != nil {
		return nil, nil, err
	}
	sources := configured.Sources()
	receipts := make([]renderer.ActivationReceipt, 0, len(sources))
	for index, source := range sources {
		catalogID := configured.Catalogs[index].ID
		candidate, err := source.Load(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			receipts = append(receipts, degradedReceipt(server, catalogID, fmt.Errorf("load catalog %q: %w", catalogID, err)))
			continue
		}
		if _, err := server.CheckStartupProcess(); err != nil {
			receipts = append(receipts, degradedReceipt(server, catalogID, err))
			continue
		}
		receipt, err := server.Activate(ctx, candidate)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			receipts = append(receipts, degradedReceipt(server, catalogID, err))
			continue
		}
		receipts = append(receipts, receipt)
	}
	return server.Handler(), receipts, nil
}

func degradedReceipt(server renderer.Server, catalogID string, refreshErr error) renderer.ActivationReceipt {
	receipt, _ := server.Active(catalogID)
	receipt.Degraded = true
	receipt.Diagnostic = boundedDiagnostic(refreshErr.Error(), 256)
	return receipt
}

func boundedDiagnostic(value string, limit int) string {
	value = strings.ToValidUTF8(strings.TrimSpace(value), "?")
	if len(value) <= limit {
		return value
	}
	for limit > 0 && (value[limit]&0xc0) == 0x80 {
		limit--
	}
	return value[:limit]
}
