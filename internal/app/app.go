package app

import (
	"context"
	"fmt"
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
	SourceKind           string
	SpecPath             string
	GitRepo              string
	GitRef               string
	GitUsername          string
	GitToken             string
	GitSSHPrivateKey     string
	DataDir              string
	StaticDir            string
	Branding             core.DocsBranding
	EndpointSidebarLabel EndpointSidebarLabelMode
}

const (
	SourceKindFile = "file"
	SourceKindGit  = "git"
)

func New(ctx context.Context, specPath string) (http.Handler, error) {
	return NewWithOptions(ctx, Options{SpecPath: specPath})
}

func NewWithOptions(ctx context.Context, opts Options) (http.Handler, error) {
	opts = opts.withDefaults()
	src, source, err := opts.source()
	if err != nil {
		return nil, err
	}
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
			Source:     source,
			Revision:   result.Revision,
			SyncRecord: result.Record,
		},
	}), nil
}

func (o Options) withDefaults() Options {
	if o.SourceKind == "" {
		if o.GitRepo != "" {
			o.SourceKind = SourceKindGit
		} else {
			o.SourceKind = SourceKindFile
		}
	}
	if o.ProjectID == "" {
		o.ProjectID = "default"
	}
	if o.SourceID == "" {
		if o.SourceKind == SourceKindGit && o.GitRepo != "" {
			o.SourceID = o.GitRepo
		} else {
			o.SourceID = "default"
		}
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

func (o Options) source() (core.SourceFetcher, core.Source, error) {
	source := core.Source{
		ID:        o.SourceID,
		ProjectID: o.ProjectID,
		Kind:      o.SourceKind,
		SpecPath:  o.SpecPath,
	}
	switch o.SourceKind {
	case SourceKindFile:
		return sourceadapter.File{Path: o.SpecPath}, source, nil
	case SourceKindGit:
		if o.GitRepo == "" {
			return nil, core.Source{}, fmt.Errorf("git source repo is required")
		}
		if source.ID == "" {
			source.ID = o.GitRepo
		}
		return sourceadapter.Git{
			Repo:          o.GitRepo,
			Ref:           o.GitRef,
			Path:          o.SpecPath,
			Username:      o.GitUsername,
			Token:         o.GitToken,
			SSHPrivateKey: o.GitSSHPrivateKey,
		}, source, nil
	default:
		return nil, core.Source{}, fmt.Errorf("unsupported source kind %q", o.SourceKind)
	}
}
