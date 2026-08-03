package selfhosted

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	configadapter "github.com/araihu/manja/internal/adapters/config"
	"github.com/araihu/manja/renderer"
)

type RendererOptions struct {
	ConfigPath string
	DataDir    string
}

func NewRenderer(ctx context.Context, options RendererOptions) (http.Handler, []renderer.ActivationReceipt, error) {
	if strings.TrimSpace(options.ConfigPath) == "" {
		return nil, nil, fmt.Errorf("renderer config path is required")
	}
	configured, err := configadapter.LoadRenderer(options.ConfigPath)
	if err != nil {
		return nil, nil, err
	}
	runtimeConfig := configured.RuntimeConfig()
	if strings.TrimSpace(options.DataDir) != "" {
		runtimeConfig.DataDir = options.DataDir
	}
	server, err := renderer.New(runtimeConfig)
	if err != nil {
		return nil, nil, err
	}
	sources := configured.Sources()
	receipts := make([]renderer.ActivationReceipt, 0, len(sources))
	for index, source := range sources {
		candidate, err := source.Load(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("load catalog %q: %w", configured.Catalogs[index].ID, err)
		}
		receipt, err := server.Activate(ctx, candidate)
		if err != nil {
			return nil, nil, err
		}
		receipts = append(receipts, receipt)
	}
	return server.Handler(), receipts, nil
}
