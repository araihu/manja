package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/araihu/manja/internal/app"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	spec := flag.String("spec", defaultSpecPath(), "OpenAPI spec path")
	dataDir := flag.String("data-dir", ".manja/data", "local Manja data directory")
	flag.Parse()

	handler, err := app.NewWithOptions(context.Background(), app.Options{
		SpecPath: *spec,
		DataDir:  *dataDir,
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
