//go:build !manja_runtime

package selfhosted

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	artifact "github.com/araihu/manja/application/htmlartifact"
	"golang.org/x/net/html"
)

type exportDocumentContext struct{ mount, key string }

type exportFragmentVerifier struct {
	root      string
	declared  map[string]exportFileEntry
	documents map[string]exportDocumentContext
	verified  map[string]artifact.FragmentIdentity
}

func (v *exportFragmentVerifier) require(document exportDocumentContext, kind artifact.FragmentKind, resource string) error {
	identity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: kind, Resource: resource}
	p, meta, err := artifact.FragmentLocation(document.mount, document.key, identity)
	if err != nil {
		return err
	}
	if previous, ok := v.verified[p]; ok {
		if previous != identity {
			return fmt.Errorf("fragment identity collision %q", p)
		}
		return nil
	}
	entry, ok := v.declared[p]
	if !ok || entry.MediaType != "text/html" {
		return fmt.Errorf("required HTML fragment %q is missing", p)
	}
	sidecar, ok := v.declared[meta]
	if !ok || sidecar.MediaType != "application/json" || sidecar.Length > 64<<10 {
		return fmt.Errorf("fragment sidecar %q is missing or invalid", meta)
	}
	data, err := os.ReadFile(filepath.Join(v.root, filepath.FromSlash(meta)))
	if err != nil {
		return err
	}
	var manifest artifact.Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&manifest); err != nil {
		return err
	}
	if err = requireJSONEOF(decoder); err != nil {
		return err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if manifest.Validate() != nil || !bytes.Equal(append(encoded, '\n'), data) || manifest.Fragment != identity || manifest.Content.Length != entry.Length || manifest.Content.SHA256 != entry.SHA256 {
		return fmt.Errorf("fragment sidecar %q differs from HTML", meta)
	}
	if kind == artifact.FragmentSchema && strings.HasPrefix(resource, "tree-sha256-") && strings.TrimPrefix(resource, "tree-sha256-") != entry.SHA256 {
		return fmt.Errorf("schema tree %q digest differs", p)
	}
	v.verified[p] = identity
	return nil
}

func (v *exportFragmentVerifier) inspect(name string) func(*html.Node) error {
	var document exportDocumentContext
	for prefix, d := range v.documents {
		if strings.HasPrefix(name, prefix) {
			document = d
			break
		}
	}
	expectedNode := ""
	if identity, ok := v.verified[name]; ok && identity.Kind == artifact.FragmentSchema && strings.HasPrefix(identity.Resource, "node-") {
		expectedNode = strings.TrimPrefix(identity.Resource, "node-")
	}
	panels := 0
	return func(node *html.Node) error {
		if node == nil { // End of document: a node fragment must contain its panel.
			if expectedNode != "" && panels != 1 {
				return fmt.Errorf("schema panel %q is missing or duplicated", name)
			}
			return nil
		}
		if hasHTMLAttribute(node, "data-manja-sidebar-next-chunk", "true") {
			collection := htmlAttribute(node, "data-manja-sidebar-collection")
			chunk, err := strconv.ParseUint(htmlAttribute(node, "data-manja-sidebar-chunk"), 10, 32)
			if err != nil || document.key == "" || (collection != "operations" && collection != "schemas" && !strings.HasPrefix(collection, "operation-group-")) {
				return fmt.Errorf("invalid sidebar chunk reference in %q", name)
			}
			if err := v.require(document, artifact.FragmentSidebar, document.key+":"+collection+":"+strconv.FormatUint(chunk, 10)); err != nil {
				return err
			}
		}
		resource := ""
		if hasHTMLAttribute(node, "data-manja-static-schema-fragment", "true") {
			resource = htmlAttribute(node, "data-manja-schema-resource")
			if !strings.HasPrefix(resource, "tree-sha256-") {
				return fmt.Errorf("invalid lazy schema resource in %q", name)
			}
		}
		if value := htmlAttribute(node, "data-manja-schema-node"); value != "" {
			if htmlAttribute(node, "id") != "schema-node-panel" {
				return fmt.Errorf("schema panel %q has no panel ID", name)
			}
			panels++
			if expectedNode != "" && value != expectedNode {
				return fmt.Errorf("schema panel %q ordinal differs", name)
			}
			n, err := strconv.ParseUint(value, 10, 32)
			if err != nil {
				return err
			}
			resource = "node-" + strconv.FormatUint(n, 10)
		}
		if resource != "" {
			if document.key == "" {
				return fmt.Errorf("schema resource in %q has no document", name)
			}
			if err := v.require(document, artifact.FragmentSchema, resource); err != nil {
				return err
			}
		}
		if node.Data == "a" && hasHTMLAttribute(node, "data-catalog-schema-reference", "true") {
			values, err := schemaPanelOrdinalsForLink(node)
			if err != nil {
				return err
			}
			if document.key == "" {
				return fmt.Errorf("schema link in %q has no document", name)
			}
			return v.require(document, artifact.FragmentSchema, "node-"+strconv.FormatUint(uint64(values), 10))
		}
		return nil
	}
}

func schemaPanelOrdinalsForLink(node *html.Node) (uint32, error) {
	u, err := url.Parse(htmlAttribute(node, "href"))
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(u.Query().Get("node"), 10, 32)
	return uint32(n), err
}

func (v *exportFragmentVerifier) verifyPath(name string) error {
	if !strings.Contains(name, "/_manja/fragments/") {
		return nil
	}
	sidecar, ok := v.declared[name+".meta.json"]
	if !ok || sidecar.Length > 64<<10 {
		return fmt.Errorf("fragment %q has no bounded sidecar", name)
	}
	data, err := os.ReadFile(filepath.Join(v.root, filepath.FromSlash(name+".meta.json")))
	if err != nil {
		return err
	}
	var manifest artifact.Manifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	for prefix, d := range v.documents {
		if strings.HasPrefix(name, prefix) {
			expected, _, err := artifact.FragmentLocation(d.mount, d.key, manifest.Fragment)
			if err != nil {
				return err
			}
			if expected != name {
				return fmt.Errorf("fragment %q path differs", name)
			}
			return v.require(d, manifest.Fragment.Kind, manifest.Fragment.Resource)
		}
	}
	return fmt.Errorf("fragment %q has no document", name)
}
