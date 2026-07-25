package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	core "github.com/araihu/manja/domain"
	"github.com/araihu/manja/site/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	spec := flag.String("spec", "", "OpenAPI spec path for the /demo renderer")
	dataDir := flag.String("data-dir", "", "local Manja data directory for the /demo renderer")
	staticDir := flag.String("static-dir", "", "Manja renderer static asset directory for /demo")
	brandName := flag.String("brand-name", "", "public docs brand name for /demo")
	brandLogo := flag.String("brand-logo", "", "public docs brand logo URL for /demo")
	brandLogoAlt := flag.String("brand-logo-alt", "", "public docs brand logo alt text for /demo")
	brandHomeURL := flag.String("brand-home-url", "", "public docs brand home URL for /demo")
	brandFavicon := flag.String("brand-favicon", "", "public docs favicon URL for /demo")
	flag.Parse()

	log.Printf("manja site listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, server.NewWithOptions(context.Background(), server.Options{
		SpecPath:  *spec,
		DataDir:   *dataDir,
		StaticDir: *staticDir,
		Branding: core.DocsBranding{
			DisplayName: *brandName,
			Logo: core.DocsBrandingLogo{
				Src:     *brandLogo,
				Alt:     *brandLogoAlt,
				HomeURL: *brandHomeURL,
			},
			Favicon: *brandFavicon,
		},
	})))
}
