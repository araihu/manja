package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/araihu/manja/internal/app"
	"github.com/araihu/manja/internal/core"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	spec := flag.String("spec", defaultSpecPath(), "OpenAPI spec path")
	dataDir := flag.String("data-dir", ".manja/data", "local Manja data directory")
	endpointSidebarLabel := flag.String("endpoint-sidebar-label", string(app.EndpointSidebarLabelAuto), "endpoint sidebar label mode: auto or path")
	brandName := flag.String("brand-name", "", "public docs brand name")
	brandLogo := flag.String("brand-logo", "", "public docs brand logo URL")
	brandLogoAlt := flag.String("brand-logo-alt", "", "public docs brand logo alt text")
	brandHomeURL := flag.String("brand-home-url", "", "public docs brand home URL")
	brandFavicon := flag.String("brand-favicon", "", "public docs favicon URL")
	flag.Parse()

	handler, err := app.NewWithOptions(context.Background(), app.Options{
		SpecPath:             *spec,
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
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("manja listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}

func defaultSpecPath() string {
	return "internal/adapters/openapi/testdata/github-v3-rest.json"
}
