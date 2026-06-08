package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/araihu/manja/site/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	log.Printf("manja site listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, server.New()))
}
