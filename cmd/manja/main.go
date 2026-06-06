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
	spec := flag.String("spec", "internal/adapters/openapi/testdata/petstore.yaml", "OpenAPI spec path")
	flag.Parse()

	handler, err := app.New(context.Background(), *spec)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("manja listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
