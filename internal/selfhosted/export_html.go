//go:build !manja_runtime

package selfhosted

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/internal/localdocs"
	"golang.org/x/net/html"
)

type exportHTMLCatalog struct {
	Mount      string
	SnapshotID string
	Directory  catalog.CatalogArtifactV1
	Descriptor localdocs.DescriptorV1
}

func rewriteExportHTML(input []byte, basePath string, catalogContext *exportHTMLCatalog) ([]byte, error) {
	document, err := html.Parse(bytes.NewReader(input))
	if err != nil {
		return nil, fmt.Errorf("parse export HTML: %w", err)
	}
	if err := rewriteHTMLChildren(document, basePath, catalogContext); err != nil {
		return nil, err
	}
	if catalogContext != nil {
		if err := installStaticDescriptor(document, catalogContext.Descriptor, basePath); err != nil {
			return nil, err
		}
	}
	var output bytes.Buffer
	if err := html.Render(&output, document); err != nil {
		return nil, fmt.Errorf("render export HTML: %w", err)
	}
	return output.Bytes(), nil
}

func rewriteHTMLChildren(parent *html.Node, basePath string, catalogContext *exportHTMLCatalog) error {
	for node := parent.FirstChild; node != nil; {
		next := node.NextSibling
		if removeStaticHTMLNode(node) {
			parent.RemoveChild(node)
			node = next
			continue
		}
		if node.Type == html.ElementNode {
			attributes := node.Attr[:0]
			for _, attribute := range node.Attr {
				switch attribute.Key {
				case "hx-get", "data-search-fallback-url":
					continue
				case "data-goshtoso-dependencies":
					rewritten, err := rewriteDependencyURLs(attribute.Val, basePath)
					if err != nil {
						return err
					}
					attribute.Val = rewritten
				case "href", "src", "action", "data-search-child-base":
					rewritten, err := rewriteHTMLURL(node, attribute.Key, attribute.Val, basePath, catalogContext)
					if err != nil {
						return err
					}
					attribute.Val = rewritten
				}
				attributes = append(attributes, attribute)
			}
			node.Attr = attributes
		}
		if err := rewriteHTMLChildren(node, basePath, catalogContext); err != nil {
			return err
		}
		node = next
	}
	return nil
}

func rewriteDependencyURLs(value, basePath string) (string, error) {
	var config map[string]any
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return "", errors.New("export dependency manifest is invalid")
	}
	dependencies, ok := config["dependencies"].([]any)
	if !ok {
		return "", errors.New("export dependency manifest has no dependencies")
	}
	for _, candidate := range dependencies {
		dependency, ok := candidate.(map[string]any)
		if !ok {
			return "", errors.New("export dependency entry is invalid")
		}
		local, _ := dependency["fallback_url"].(string)
		if !strings.HasPrefix(local, "/") {
			local, _ = dependency["primary_url"].(string)
		}
		if !strings.HasPrefix(local, "/") {
			return "", errors.New("export dependency has no local asset")
		}
		local = prefixExportBase(basePath, local)
		dependency["primary_url"] = local
		dependency["fallback_url"] = local
	}
	data, err := json.Marshal(config)
	if err != nil {
		return "", errors.New("export dependency manifest cannot be encoded")
	}
	return string(data), nil
}

func removeStaticHTMLNode(node *html.Node) bool {
	if node.Type != html.ElementNode {
		return false
	}
	for _, attribute := range node.Attr {
		if attribute.Key == "data-manja-copy-page" {
			return true
		}
		if attribute.Key == "href" {
			parsed, err := url.Parse(attribute.Val)
			if err == nil && (strings.HasSuffix(parsed.Path, "/page.md") || parsed.Query().Has("format") && parsed.Query().Get("format") == "markdown") {
				return true
			}
		}
	}
	return false
}

func rewriteHTMLURL(node *html.Node, attribute, value, basePath string, catalogContext *exportHTMLCatalog) (string, error) {
	if value == "" || strings.HasPrefix(value, "#") {
		return value, nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid export URL %q", value)
	}
	if parsed.IsAbs() {
		if parsed.Scheme != "https" || attribute != "href" || node.Data != "a" && !hasHTMLAttribute(node, "rel", "canonical") {
			return "", fmt.Errorf("external export resource %q is not supported", value)
		}
		return value, nil
	}
	if parsed.Host != "" || strings.HasPrefix(value, "//") {
		return "", fmt.Errorf("protocol-relative export URL %q is not supported", value)
	}
	if parsed.Path == "" || !strings.HasPrefix(parsed.Path, "/") {
		return value, nil
	}
	if catalogContext != nil {
		parsed.Path = rewriteStableCatalogPath(parsed.Path, catalogContext)
	}
	parsed.Path = prefixExportBase(basePath, parsed.Path)
	return parsed.String(), nil
}

func rewriteStableCatalogPath(value string, context *exportHTMLCatalog) string {
	mount := strings.TrimSuffix(context.Mount, "/")
	if mount == "" {
		mount = "/"
	}
	stableCatalog := path.Join(mount, "catalog.json")
	if !strings.HasPrefix(stableCatalog, "/") {
		stableCatalog = "/" + stableCatalog
	}
	if value == stableCatalog {
		return catalogRoute(context.Mount, "snapshots", context.SnapshotID, "catalog.json")
	}
	stableOpenAPI := catalogRoute(context.Mount, "openapi") + "/"
	if strings.HasPrefix(value, stableOpenAPI) {
		return catalogRoute(context.Mount, "snapshots", context.SnapshotID, "openapi", strings.TrimPrefix(value, stableOpenAPI))
	}
	return value
}

func prefixExportBase(basePath, value string) string {
	if basePath == "/" {
		return value
	}
	if value == "/" {
		return basePath
	}
	return strings.TrimSuffix(basePath, "/") + value
}

func installStaticDescriptor(document *html.Node, descriptor localdocs.DescriptorV1, basePath string) error {
	head := findHTMLElement(document, "head")
	if head == nil {
		return errors.New("export HTML has no head")
	}
	for child := head.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == html.ElementNode && child.Data == "script" && hasHTMLAttribute(child, "id", "manja-local-docs-descriptor") {
			head.RemoveChild(child)
		}
		child = next
	}
	data, err := json.Marshal(descriptor)
	if err != nil {
		return fmt.Errorf("encode static descriptor: %w", err)
	}
	descriptorNode := &html.Node{Type: html.ElementNode, Data: "script", Attr: []html.Attribute{{Key: "id", Val: "manja-local-docs-descriptor"}, {Key: "type", Val: "application/json"}}}
	descriptorNode.AppendChild(&html.Node{Type: html.TextNode, Data: string(data)})
	head.AppendChild(descriptorNode)
	if findScriptSource(head, prefixExportBase(basePath, "/manja-assets/local-docs.js")) == nil {
		head.AppendChild(&html.Node{Type: html.ElementNode, Data: "script", Attr: []html.Attribute{{Key: "src", Val: prefixExportBase(basePath, "/manja-assets/local-docs.js")}}})
	}
	return nil
}

func findHTMLElement(node *html.Node, name string) *html.Node {
	if node.Type == html.ElementNode && node.Data == name {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func findScriptSource(node *html.Node, source string) *html.Node {
	if node.Type == html.ElementNode && node.Data == "script" && hasHTMLAttribute(node, "src", source) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findScriptSource(child, source); found != nil {
			return found
		}
	}
	return nil
}

func hasHTMLAttribute(node *html.Node, name, value string) bool {
	for _, attribute := range node.Attr {
		if attribute.Key == name && attribute.Val == value {
			return true
		}
	}
	return false
}
