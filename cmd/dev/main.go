package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/araihu/manja/internal/devlauncher"
)

func main() {
	options, err := devlauncher.Parse(os.Args[1:], os.Getenv)
	if err != nil {
		fatal(err)
	}
	if options.Help {
		fmt.Println("Usage: go run ./cmd/dev [--print-ports] [--app-port N] [--proxy-port N] [--site-port N] [-- manja flags]")
		return
	}
	root, err := filepath.Abs(".")
	if err != nil {
		fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ports, err := devlauncher.ChoosePorts(ctx, options, root)
	if err != nil {
		fatal(err)
	}
	if options.PrintPorts {
		payload := struct {
			AppPort   int      `json:"appPort"`
			ProxyPort int      `json:"proxyPort"`
			SitePort  int      `json:"sitePort"`
			AppURL    string   `json:"appURL"`
			ProxyURL  string   `json:"proxyURL"`
			SiteURL   string   `json:"siteURL"`
			AirArgs   []string `json:"airArgs"`
			SiteArgs  []string `json:"siteArgs"`
		}{ports.AppPort, ports.ProxyPort, ports.SitePort, fmt.Sprintf("http://%s:%d", options.Host, ports.AppPort), fmt.Sprintf("http://%s:%d", options.Host, ports.ProxyPort), fmt.Sprintf("http://%s:%d", options.Host, ports.SitePort), devlauncher.AirArgs(options, ports), devlauncher.SiteArgs(options, ports)}
		encoded, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(encoded))
		return
	}
	if err := devlauncher.Run(ctx, root, options, ports, os.Stdout); err != nil && ctx.Err() == nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
