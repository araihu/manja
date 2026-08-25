package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type runtimeAsset struct {
	name   string
	source string
}

var runtimeAssets = []runtimeAsset{
	{name: "sw.js", source: "sw.js"},
	{name: "storage.js", source: "storage.js"},
	{name: "local-docs.js", source: "../local-docs.js"},
	{name: "wasm_exec.js", source: "wasm_exec.js"},
	{name: "manja.wasm", source: "manja.wasm"},
	{name: "manja.wasm.br", source: "manja.wasm.br"},
}

func main() {
	directory := flag.String("dir", "", "local-docs asset directory")
	flag.Parse()
	if *directory == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "manja-local-docs-assets: --dir is required")
		os.Exit(2)
	}
	if err := generate(*directory); err != nil {
		fmt.Fprintf(os.Stderr, "manja-local-docs-assets: %v\n", err)
		os.Exit(1)
	}
}

func generate(directory string) error {
	file, err := os.CreateTemp(directory, ".runtime-assets-*.js")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := writeManifest(file, directory); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(directory, "runtime-assets.js"))
}

func writeManifest(writer io.Writer, directory string) error {
	if _, err := io.WriteString(writer, `(function (root, factory) {
  "use strict"
  const manifest = factory()
  if (typeof module === "object" && module.exports) module.exports = manifest
  root.ManjaLocalDocsAssetManifest = manifest
}(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict"

  // Generated from the embedded production runtime bytes. This companion
  // stays separate so sw.js can validate its own bytes without recursion.
  return Object.freeze({
    schemaVersion: 1,
    assets: Object.freeze({
`); err != nil {
		return err
	}
	for _, asset := range runtimeAssets {
		data, err := os.ReadFile(filepath.Join(directory, asset.source))
		if err != nil {
			return fmt.Errorf("read %s: %w", asset.name, err)
		}
		digest := sha256.Sum256(data)
		urlPath := "/manja-assets/local-docs/" + asset.name
		if asset.name == "local-docs.js" {
			urlPath = "/manja-assets/local-docs.js"
		}
		if _, err := fmt.Fprintf(writer, "      %q: Object.freeze({ length: %d, sha256: %q }),\n", urlPath, len(data), hex.EncodeToString(digest[:])); err != nil {
			return err
		}
	}
	_, err := io.WriteString(writer, `    }),
  })
}))
`)
	return err
}
