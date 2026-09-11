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
	sidebarChunkSize := fs.Uint("sidebar-chunk-size", 12, "operations per static sidebar chunk")
	schemaCacheMiB := fs.Uint64("schema-cache-mib", 128, "decoded schema cache budget in MiB shared by export workers (0 disables)")
	fragmentWorkers := fs.Uint("fragment-workers", 4, "bounded concurrent static fragment renderers")
	var progressMode exportProgressFlag
	fs.Var(&progressMode, "progress", "emit export progress to stderr (human or json; --progress defaults to human)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "manja export: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 || blank(*rendererConfig) || blank(*dataDir) || blank(*output) || blank(*basePath) {
		fmt.Fprintln(stderr, "manja export: --renderer-config, --data-dir, --output, and --base-path are required; positional arguments are not accepted")
		return 2
	}
	if *schemaCacheMiB > ^uint64(0)>>20 {
		fmt.Fprintln(stderr, "manja export: schema cache budget is too large")
		return 2
	}
	schemaCacheBytes := *schemaCacheMiB << 20
	if progressMode.mode != "json" {
		fmt.Fprintln(stderr, "manja export: warning: exporting every configured catalog regardless of catalog visibility")
	}
	receipt, err := exportRenderer(ctx, app.ExportOptions{
		RendererOptions:  app.RendererOptions{ConfigPath: *rendererConfig, DataDir: *dataDir, ResourceLimits: resourceLimits},
		Output:           *output,
		BasePath:         *basePath,
		SidebarChunkSize: uint32(*sidebarChunkSize),
		FragmentWorkers:  uint32(*fragmentWorkers),
		SchemaCacheBytes: &schemaCacheBytes,
		Progress:         newExportProgressWriter(progressMode.mode, stderr),
	})
	if err != nil {
		if progressMode.mode != "json" {
			fmt.Fprintf(stderr, "manja export: %v\n", err)
		}
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

type exportProgressFlag struct {
	mode string
}

func (flag *exportProgressFlag) String() string { return flag.mode }

func (flag *exportProgressFlag) Set(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "true", "human":
		flag.mode = "human"
	case "false", "off":
		flag.mode = ""
	case "json":
		flag.mode = "json"
	default:
		return fmt.Errorf("must be human or json")
	}
	return nil
}

func (flag *exportProgressFlag) IsBoolFlag() bool { return true }

func newExportProgressWriter(mode string, stderr io.Writer) app.ExportProgress {
	switch mode {
	case "human":
		return func(event app.ExportProgressEvent) {
			_, _ = fmt.Fprintln(stderr, app.FormatExportProgress(event))
		}
	case "json":
		return func(event app.ExportProgressEvent) {
			_ = json.NewEncoder(stderr).Encode(event)
		}
	default:
		return nil
	}
}
