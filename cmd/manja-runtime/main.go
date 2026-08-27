package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/araihu/manja/internal/environment"
	app "github.com/araihu/manja/internal/selfhosted"
)

type runtimeConfig struct {
	Addr              string
	RendererConfig    string
	DataDir           string
	LocalDocsDisabled bool
	ResourceLimits    bool
}

var serveRecovered = func(ctx context.Context, cfg runtimeConfig) error {
	handler, receipts, err := app.NewRecoveredRenderer(ctx, app.RendererOptions{ConfigPath: cfg.RendererConfig, DataDir: cfg.DataDir, LocalDocsDisabled: cfg.LocalDocsDisabled, ResourceLimits: cfg.ResourceLimits})
	if err != nil {
		return err
	}
	log.Printf("manja recovered precompiled renderer: %v", receipts)
	log.Printf("manja listening on %s", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, handler)
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stderr))
}

func run(ctx context.Context, args []string, stderr io.Writer) int {
	environmentConfig, err := environment.Load()
	if err != nil {
		fmt.Fprintf(stderr, "manja-runtime: %v\n", err)
		return 2
	}
	cfg, err := configFromArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "manja-runtime: %v\n", err)
		return 2
	}
	cfg.LocalDocsDisabled = environmentConfig.LocalDocsDisabled()
	cfg.ResourceLimits = environmentConfig.ResourceLimits
	if err := serveRecovered(ctx, cfg); err != nil {
		fmt.Fprintf(stderr, "manja-runtime: %v\n", err)
		return 1
	}
	return 0
}

func configFromArgs(args []string) (runtimeConfig, error) {
	fs := flag.NewFlagSet("manja-runtime", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addr := fs.String("addr", ":8080", "listen address")
	rendererConfig := fs.String("renderer-config", "", "renderer catalog YAML config")
	dataDir := fs.String("data-dir", "", "precompiled renderer data directory")
	if err := fs.Parse(args); err != nil {
		return runtimeConfig{}, err
	}
	if fs.NArg() != 0 {
		return runtimeConfig{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(*rendererConfig) == "" || strings.TrimSpace(*dataDir) == "" {
		return runtimeConfig{}, fmt.Errorf("--renderer-config and --data-dir are required")
	}
	return runtimeConfig{Addr: *addr, RendererConfig: *rendererConfig, DataDir: *dataDir}, nil
}
