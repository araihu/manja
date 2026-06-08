package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/araihu/manja/site/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	spec := flag.String("spec", "", "OpenAPI spec path for the /demo renderer")
	dataDir := flag.String("data-dir", "", "local Manja data directory for the /demo renderer")
	staticDir := flag.String("static-dir", "", "Manja renderer static asset directory for /demo")
	flag.Parse()

	log.Printf("manja site listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, server.NewWithOptions(context.Background(), server.Options{
		SpecPath:  *spec,
		DataDir:   *dataDir,
		StaticDir: *staticDir,
	})))
}
