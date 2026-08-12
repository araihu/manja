package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	core "github.com/araihu/manja/domain"
	app "github.com/araihu/manja/internal/selfhosted"
)

type cliConfig struct {
	Addr           string
	RendererConfig string
	Options        app.Options
}

var serve = func(ctx context.Context, cfg cliConfig) error {
	var handler http.Handler
	if cfg.RendererConfig != "" {
		catalogHandler, receipts, err := app.NewRenderer(ctx, app.RendererOptions{ConfigPath: cfg.RendererConfig, DataDir: cfg.Options.DataDir})
		if err != nil {
			return err
		}
		handler = catalogHandler
		log.Printf("manja renderer activated: %v", receipts)
	} else {
		var err error
		handler, err = app.NewWithOptions(ctx, cfg.Options)
		if err != nil {
			return err
		}
	}
	log.Printf("manja listening on %s", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, handler)
}

var buildRenderer = app.BuildRenderer

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "check" {
		return runCheck(ctx, args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "build" {
		return runBuild(ctx, args[1:], stdout, stderr)
	}
	if err := runServer(ctx, args); err != nil {
		fmt.Fprintf(stderr, "manja: %v\n", err)
		return 1
	}
	return 0
}

type buildReceipt struct {
	SchemaVersion uint32                `json:"schemaVersion"`
	Catalogs      []buildCatalogReceipt `json:"catalogs"`
}

type buildCatalogReceipt struct {
	CatalogID  string `json:"catalogId"`
	Mount      string `json:"mount"`
	RevisionID string `json:"revisionId"`
	SnapshotID string `json:"snapshotId"`
}

func runBuild(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("manja build", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	rendererConfig := fs.String("renderer-config", "", "renderer catalog YAML config")
	dataDir := fs.String("data-dir", "", "snapshot output directory")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "manja build: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(*rendererConfig) == "" || strings.TrimSpace(*dataDir) == "" {
		fmt.Fprintln(stderr, "manja build: --renderer-config and --data-dir are required; positional arguments are not accepted")
		return 2
	}
	receipts, err := buildRenderer(ctx, app.RendererOptions{ConfigPath: *rendererConfig, DataDir: *dataDir})
	if err != nil {
		fmt.Fprintf(stderr, "manja build: %v\n", err)
		return 1
	}
	result := buildReceipt{SchemaVersion: 1, Catalogs: make([]buildCatalogReceipt, len(receipts))}
	for index, receipt := range receipts {
		result.Catalogs[index] = buildCatalogReceipt{CatalogID: receipt.CatalogID, Mount: receipt.Mount, RevisionID: receipt.RevisionID, SnapshotID: receipt.SnapshotID}
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		fmt.Fprintf(stderr, "manja build: write receipt: %v\n", err)
		return 1
	}
	return 0
}

func runServer(ctx context.Context, args []string) error {
	cfg, err := configFromArgs(args)
	if err != nil {
		return err
	}
	return serve(ctx, cfg)
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
	publicOrigin := fs.String("public-origin", "", "trusted public docs origin (HTTPS; HTTP allowed only for loopback development)")
	rendererConfig := fs.String("renderer-config", "", "renderer-only catalog YAML config")
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
		Addr: *addr, RendererConfig: *rendererConfig,
		Options: app.Options{
			SourceKind:           *sourceKind,
			SpecPath:             *spec,
			GitRepo:              *gitRepo,
			GitRef:               *gitRef,
			DataDir:              *dataDir,
			PublicOrigin:         *publicOrigin,
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
