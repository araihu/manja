package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	app "github.com/araihu/manja/internal/selfhosted"
)

var exportRenderer = app.ExportRenderer
var verifyExport = app.VerifyExport

func runExport(ctx context.Context, args []string, stdout, stderr io.Writer, resourceLimits bool) int {
	if len(args) > 0 && args[0] == "verify" {
		return runExportVerify(ctx, args[1:], stdout, stderr)
	}
	fs := flag.NewFlagSet("manja export", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	rendererConfig := fs.String("renderer-config", "", "renderer catalog YAML config")
	dataDir := fs.String("data-dir", "", "snapshot data directory")
	output := fs.String("output", "", "static export output directory")
	basePath := fs.String("base-path", "", "published URL base path")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "manja export: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 || blank(*rendererConfig) || blank(*dataDir) || blank(*output) || blank(*basePath) {
		fmt.Fprintln(stderr, "manja export: --renderer-config, --data-dir, --output, and --base-path are required; positional arguments are not accepted")
		return 2
	}
	fmt.Fprintln(stderr, "manja export: warning: exporting every configured catalog regardless of catalog visibility")
	receipt, err := exportRenderer(ctx, app.ExportOptions{
		RendererOptions: app.RendererOptions{ConfigPath: *rendererConfig, DataDir: *dataDir, ResourceLimits: resourceLimits},
		Output:          *output,
		BasePath:        *basePath,
	})
	if err != nil {
		fmt.Fprintf(stderr, "manja export: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
		fmt.Fprintf(stderr, "manja export: write receipt: %v\n", err)
		return 1
	}
	return 0
}

func runExportVerify(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("manja export verify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	output := fs.String("output", "", "static export output directory")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "manja export verify: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 || blank(*output) {
		fmt.Fprintln(stderr, "manja export verify: --output is required; positional arguments are not accepted")
		return 2
	}
	receipt, err := verifyExport(ctx, *output)
	if err != nil {
		fmt.Fprintf(stderr, "manja export verify: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
		fmt.Fprintf(stderr, "manja export verify: write receipt: %v\n", err)
		return 1
	}
	return 0
}

func blank(value string) bool { return strings.TrimSpace(value) == "" }
