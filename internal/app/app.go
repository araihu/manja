package app

import (
	"context"
	"net/http"

	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
	sourceadapter "github.com/araihu/manja/internal/adapters/source"
	"github.com/araihu/manja/internal/web"
)

func New(ctx context.Context, specPath string) (http.Handler, error) {
	src := sourceadapter.File{Path: specPath}
	spec, rev, err := src.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(ctx, spec, rev)
	if err != nil {
		return nil, err
	}
	return web.NewServer(idx), nil
}
