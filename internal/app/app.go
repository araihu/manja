package app

import (
	"context"
	"net/http"
	"path/filepath"

	cacheadapter "github.com/araihu/manja/internal/adapters/cache"
	markdownadapter "github.com/araihu/manja/internal/adapters/markdown"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
	sourceadapter "github.com/araihu/manja/internal/adapters/source"
	storeadapter "github.com/araihu/manja/internal/adapters/store"
	"github.com/araihu/manja/internal/core"
	"github.com/araihu/manja/internal/web"
)

type EndpointSidebarLabelMode = web.EndpointSidebarLabelMode

const (
	EndpointSidebarLabelAuto = web.EndpointSidebarLabelAuto
	EndpointSidebarLabelPath = web.EndpointSidebarLabelPath
)

type Options struct {
	ProjectID            string
	SourceID             string
	SpecPath             string
	DataDir              string
	StaticDir            string
	Branding             core.DocsBranding
	EndpointSidebarLabel EndpointSidebarLabelMode
}

func New(ctx context.Context, specPath string) (http.Handler, error) {
	return NewWithOptions(ctx, Options{SpecPath: specPath})
}

func NewWithOptions(ctx context.Context, opts Options) (http.Handler, error) {
	opts = opts.withDefaults()
	src := sourceadapter.File{Path: opts.SpecPath}
	store := storeadapter.NewFileStore(opts.DataDir)
	syncer := core.Syncer{
		Source: src,
		Parser: openapiadapter.Parser{},
		Store:  store,
		Blobs:  store,
		Cache:  cacheadapter.NewMemory(),
	}
	result, err := syncer.Sync(ctx, core.SyncRequest{
		ProjectID: opts.ProjectID,
		SourceID:  opts.SourceID,
		Trigger:   "startup",
	})
	if err != nil {
		return nil, err
	}
	return web.NewServerWithOptions(result.Index, web.Options{
		Public: web.PublicOptions{
			EndpointSidebarLabel: web.EndpointSidebarLabelMode(opts.EndpointSidebarLabel),
			MarkdownRenderer:     markdownadapter.NewRenderer(),
			StaticDir:            opts.StaticDir,
			Branding:             opts.Branding,
		},
		Management: web.ManagementOptions{
			Store: store,
			Project: core.Project{
				ID:   opts.ProjectID,
				Name: result.Index.Title,
				SEO: core.ProjectSEO{
					Robots: "index,follow",
				},
				Theme: core.ThemeSettings{
					Theme:    "manja",
					DarkMode: "auto",
				},
				SourceIDs: []string{opts.SourceID},
			},
			Source: core.Source{
				ID:        opts.SourceID,
				ProjectID: opts.ProjectID,
				Kind:      "file",
				SpecPath:  opts.SpecPath,
			},
			Revision:   result.Revision,
			SyncRecord: result.Record,
		},
	}), nil
}

func (o Options) withDefaults() Options {
	if o.ProjectID == "" {
		o.ProjectID = "default"
	}
	if o.SourceID == "" {
		o.SourceID = "default"
	}
	if o.DataDir == "" {
		o.DataDir = filepath.Join(".manja", "data")
	}
	switch o.EndpointSidebarLabel {
	case EndpointSidebarLabelPath:
	default:
		o.EndpointSidebarLabel = EndpointSidebarLabelAuto
	}
	return o
}
