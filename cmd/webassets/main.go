package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/araihu/manja/internal/webassets"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "generate" && os.Args[1] != "check") {
		fmt.Fprintln(os.Stderr, "usage: webassets generate|check")
		os.Exit(2)
	}
	root, err := filepath.Abs(".")
	if err == nil {
		_, err = webassets.Generate(root, os.Args[1] == "check")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
