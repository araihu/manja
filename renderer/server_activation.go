//go:build !manja_runtime

package renderer

import (
	"fmt"

	"github.com/araihu/manja/application/catalog"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
)

// New constructs a renderer with source activation support.
func New(config Config) (Server, error) {
	server, err := newServer(config)
	if err != nil {
		return nil, err
	}
	for _, configured := range config.Catalogs {
		parser, err := openapiadapter.NewCatalogParserWithResourceLimits(configured.CompatibilityAllowlist, config.ResourceLimits)
		if err != nil {
			return nil, fmt.Errorf("catalog %q compatibility allowlist: %w", configured.ID, err)
		}
		options := catalog.DefaultCompilerOptions()
		options.ProfileAllowlist = configured.CompatibilityAllowlist
		options.ResourceLimits = config.ResourceLimits
		compiler, err := catalog.NewCompiler(options)
		if err != nil {
			return nil, fmt.Errorf("catalog %q compiler: %w", configured.ID, err)
		}
		server.parsers[configured.ID] = parser
		server.compilers[configured.ID] = compiler
	}
	return server, nil
}
