package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/araihu/manja/internal/app"
	"github.com/araihu/manja/internal/core"
)

type cliConfig struct {
	Addr    string
	Options app.Options
}

func main() {
	cfg, err := configFromArgs(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	handler, err := app.NewWithOptions(context.Background(), cfg.Options)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("manja listening on %s", cfg.Addr)
	log.Fatal(http.ListenAndServe(cfg.Addr, handler))
}

func optionsFromArgs(args []string) (app.Options, error) {
	cfg, err := configFromArgs(args)
	return cfg.Options, err
}

func configFromArgs(args []string) (cliConfig, error) {
	fs := flag.NewFlagSet("manja", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "listen address")
	sourceKind := fs.String("source", app.SourceKindFile, "source kind: file or git")
	spec := fs.String("spec", defaultSpecPath(), "OpenAPI spec path")
	gitRepo := fs.String("git-repo", "", "Git repository URL or local path")
	gitRef := fs.String("git-ref", "", "Git ref to publish")
	dataDir := fs.String("data-dir", ".manja/data", "local Manja data directory")
	endpointSidebarLabel := fs.String("endpoint-sidebar-label", string(app.EndpointSidebarLabelAuto), "endpoint sidebar label mode: auto or path")
	brandName := fs.String("brand-name", "", "public docs brand name")
	brandLogo := fs.String("brand-logo", "", "public docs brand logo URL")
	brandLogoAlt := fs.String("brand-logo-alt", "", "public docs brand logo alt text")
	brandHomeURL := fs.String("brand-home-url", "", "public docs brand home URL")
	brandFavicon := fs.String("brand-favicon", "", "public docs favicon URL")
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}

	return cliConfig{
		Addr: *addr,
		Options: app.Options{
			SourceKind:           *sourceKind,
			SpecPath:             *spec,
			GitRepo:              *gitRepo,
			GitRef:               *gitRef,
			DataDir:              *dataDir,
			EndpointSidebarLabel: app.EndpointSidebarLabelMode(*endpointSidebarLabel),
			Branding: core.DocsBranding{
				DisplayName: *brandName,
				Logo: core.DocsBrandingLogo{
					Src:     *brandLogo,
					Alt:     *brandLogoAlt,
					HomeURL: *brandHomeURL,
				},
				Favicon: *brandFavicon,
			},
		},
	}, nil
}

func defaultSpecPath() string {
	return "internal/adapters/openapi/testdata/github-v3-rest.json"
}
