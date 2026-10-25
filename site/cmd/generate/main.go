package main

import (
	"context"
	"flag"
	"log"

	"github.com/araihu/manja/site/internal/server"
)

func main() {
	target := flag.String("target", "all", "build site, demo, or all")
	flag.Parse()
	if flag.NArg() != 0 || (*target != "all" && *target != "site" && *target != "demo") {
		log.Fatal("expected -target site, demo, or all with no positional arguments")
	}
	if *target == "all" || *target == "site" {
		if err := server.ExportSite("dist/site"); err != nil {
			log.Fatal(err)
		}
		log.Print("landing and docs: dist/site")
	}
	if *target == "all" || *target == "demo" {
		if err := server.ExportDemo(context.Background(), "dist/demo"); err != nil {
			log.Fatal(err)
		}
		log.Print("documentation demo: dist/demo")
	}
}
