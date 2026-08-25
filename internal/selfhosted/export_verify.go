//go:build !manja_runtime

package selfhosted

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/araihu/manja/internal/adapters/catalogjson"
	"github.com/araihu/manja/internal/localdocs"
	"github.com/araihu/manja/internal/web"
	"github.com/araihu/manja/renderer"
	"golang.org/x/net/html"
)

func verifyExportStructure(root string, manifest exportManifest, declared map[string]exportFileEntry) error {
	require := func(name string) error {
		entry, ok := declared[name]
		if !ok {
			return fmt.Errorf("missing required export file %q", name)
		}
		expected := mediaTypeForPath(name)
		switch strings.ToLower(path.Ext(name)) {
		case ".html", ".json", ".js", ".wasm":
			if entry.MediaType != expected {
				return fmt.Errorf("export file %q media type differs", name)
			}
		}
		return nil
	}
	for _, name := range append([]string{"index.html", "search/index.html", "sw.js"}, trimExportAssetPaths(web.CatalogAssetPaths())...) {
		if err := require(name); err != nil {
			return err
		}
	}

	previousCatalog := ""
	for _, receipt := range manifest.Catalogs {
		if receipt.CatalogID == "" || receipt.CatalogID <= previousCatalog || receipt.PublicationKey != receipt.CatalogID || receipt.RevisionID == "" || receipt.SnapshotID == "" {
			return errors.New("export manifest catalog inventory is invalid")
		}
		previousCatalog = receipt.CatalogID
		prefix := strings.Trim(receipt.Mount, "/")
		shells := []string{exportJoin(prefix, "index.html"), exportJoin(prefix, "search/index.html"), exportJoin(prefix, "_manja/offline-shell/index.html")}
		manifestPath := exportJoin(prefix, "snapshots", receipt.SnapshotID, "manifest.json")
		catalogPath := exportJoin(prefix, "snapshots", receipt.SnapshotID, "catalog.json")
		for _, name := range append(shells, exportJoin(prefix, "llms.txt"), manifestPath, catalogPath) {
			if err := require(name); err != nil {
				return err
			}
		}

		manifestBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(manifestPath)))
		if err != nil {
			return err
		}
		snapshot, err := catalogjson.DecodeManifest(manifestBytes)
		if err != nil || snapshot.Identity.CatalogID != receipt.CatalogID || snapshot.Identity.RevisionID != receipt.RevisionID || string(snapshot.SnapshotID) != receipt.SnapshotID {
			return fmt.Errorf("catalog %q export manifest identity differs", receipt.CatalogID)
		}
		catalogBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(catalogPath)))
		if err != nil {
			return err
		}
		directory, err := catalogjson.DecodeCatalog(catalogBytes)
		if err != nil || directory.CatalogID != receipt.CatalogID || catalogjson.ValidateCatalogManifest(directory, snapshot) != nil {
			return fmt.Errorf("catalog %q export directory differs", receipt.CatalogID)
		}
		for _, document := range directory.Documents {
			shells = append(shells, exportJoin(prefix, "documents", document.Key, "index.html"))
		}
		for _, shell := range shells {
			if err := require(shell); err != nil {
				return err
			}
			descriptor, err := verifyExportHTML(root, shell, manifest.BasePath, declared)
			if err != nil {
				return err
			}
			if descriptor == nil || descriptor.CatalogID != receipt.CatalogID || descriptor.PublicationKey != receipt.CatalogID || descriptor.RevisionID != receipt.RevisionID || descriptor.SnapshotID != receipt.SnapshotID {
				return fmt.Errorf("catalog %q shell descriptor differs", receipt.CatalogID)
			}
			if _, err := localdocs.Admit(*descriptor, manifestBytes); err != nil {
				return fmt.Errorf("catalog %q shell descriptor: %w", receipt.CatalogID, err)
			}
		}
		active := renderer.ActivationReceipt{CatalogID: receipt.CatalogID, Mount: receipt.Mount, RevisionID: receipt.RevisionID, SnapshotID: receipt.SnapshotID}
		for _, child := range snapshot.Children {
			if child.Path == "catalog.json" {
				continue
			}
			_, output, ok := exportedChildPath(active, directory, child)
			if !ok {
				return fmt.Errorf("catalog %q child %q cannot be verified", receipt.CatalogID, child.Path)
			}
			if err := require(output); err != nil {
				return err
			}
		}
	}

	for name, entry := range declared {
		if entry.MediaType != "text/html" {
			continue
		}
		if _, err := verifyExportHTML(root, name, manifest.BasePath, declared); err != nil {
			return err
		}
	}
	return nil
}

func verifyExportHTML(root, name, basePath string, declared map[string]exportFileEntry) (*localdocs.DescriptorV1, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		return nil, err
	}
	document, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse export HTML %q: %w", name, err)
	}
	var descriptor *localdocs.DescriptorV1
	publicPath := prefixExportBase(basePath, "/"+strings.TrimSuffix(name, "index.html"))
	if err := walkExportHTML(document, func(node *html.Node) error {
		if node.Type != html.ElementNode {
			return nil
		}
		if node.Data == "script" && hasHTMLAttribute(node, "id", "manja-local-docs-descriptor") {
			if descriptor != nil || node.FirstChild == nil || node.FirstChild != node.LastChild || node.FirstChild.Type != html.TextNode {
				return errors.New("export HTML descriptor is invalid")
			}
			decoder := json.NewDecoder(strings.NewReader(node.FirstChild.Data))
			decoder.DisallowUnknownFields()
			var value localdocs.DescriptorV1
			if err := decoder.Decode(&value); err != nil {
				return fmt.Errorf("decode export HTML descriptor: %w", err)
			}
			if err := requireJSONEOF(decoder); err != nil {
				return err
			}
			descriptor = &value
		}
		for _, attribute := range node.Attr {
			switch attribute.Key {
			case "href", "src", "action", "data-search-child-base":
				if err := verifyExportReference(node, attribute.Key, attribute.Val, publicPath, basePath, declared); err != nil {
					return fmt.Errorf("export HTML %q: %w", name, err)
				}
			case "data-goshtoso-dependencies":
				if err := verifyExportDependencyURLs(node, attribute.Val, publicPath, basePath, declared); err != nil {
					return fmt.Errorf("export HTML %q: %w", name, err)
				}
			case "hx-get", "data-search-fallback-url":
				return fmt.Errorf("export HTML %q retains runtime-only route", name)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return descriptor, nil
}

func verifyExportDependencyURLs(node *html.Node, value, publicPath, basePath string, declared map[string]exportFileEntry) error {
	var config struct {
		Dependencies []struct {
			Primary  string `json:"primary_url"`
			Fallback string `json:"fallback_url"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(value), &config); err != nil || config.Dependencies == nil {
		return errors.New("dependency manifest is invalid")
	}
	for _, dependency := range config.Dependencies {
		if dependency.Primary == "" || dependency.Primary != dependency.Fallback {
			return errors.New("dependency is not bound to one local asset")
		}
		if err := verifyExportReference(node, "src", dependency.Primary, publicPath, basePath, declared); err != nil {
			return err
		}
	}
	return nil
}

func verifyExportReference(node *html.Node, attribute, value, publicPath, basePath string, declared map[string]exportFileEntry) error {
	if value == "" || strings.HasPrefix(value, "#") {
		return nil
	}
	reference, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid reference %q", value)
	}
	if reference.IsAbs() {
		if reference.Scheme == "https" && attribute == "href" && (node.Data == "a" || hasHTMLAttribute(node, "rel", "canonical")) {
			return nil
		}
		return fmt.Errorf("external reference %q is not supported", value)
	}
	if reference.Host != "" || strings.HasPrefix(value, "//") {
		return fmt.Errorf("protocol-relative reference %q is not supported", value)
	}
	base, _ := url.Parse(publicPath)
	resolved := base.ResolveReference(reference)
	if !strings.HasPrefix(resolved.Path, basePath) {
		return fmt.Errorf("reference %q escapes deployment base", value)
	}
	relative := strings.TrimPrefix(resolved.Path, basePath)
	if attribute == "data-search-child-base" {
		for name := range declared {
			if strings.HasPrefix(name, relative) {
				return nil
			}
		}
		return fmt.Errorf("reference %q has no exported child", value)
	}
	if relative == "" {
		relative = "index.html"
	}
	if strings.HasSuffix(relative, "/") {
		relative += "index.html"
	}
	if _, ok := declared[relative]; ok {
		return nil
	}
	if _, ok := declared[path.Join(relative, "index.html")]; ok {
		return nil
	}
	return fmt.Errorf("reference %q has no exported target", value)
}

func walkExportHTML(node *html.Node, visit func(*html.Node) error) error {
	if err := visit(node); err != nil {
		return err
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := walkExportHTML(child, visit); err != nil {
			return err
		}
	}
	return nil
}

func trimExportAssetPaths(values []string) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = strings.TrimPrefix(value, "/")
	}
	return result
}

func exportJoin(parts ...string) string {
	return strings.TrimPrefix(path.Join(parts...), "/")
}
