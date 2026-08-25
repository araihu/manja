//go:build js && wasm

package main

import (
	"context"
	"errors"
	"math"
	"strings"
	"syscall/js"

	"github.com/araihu/manja/internal/localdocs"
	"github.com/araihu/manja/internal/localdocs/abi"
	localbrowser "github.com/araihu/manja/internal/localdocs/browser"
)

var (
	activateFunc         js.Func
	allowsFunc           js.Func
	resolveFunc          js.Func
	prepareFunc          js.Func
	renderFunc           js.Func
	searchFunc           js.Func
	canonicalJSONEscapes = strings.NewReplacer("<", `\u003c`, ">", `\u003e`, "&", `\u0026`, "\u2028", `\u2028`, "\u2029", `\u2029`)
)

func main() {
	api := js.Global().Get("ManjaLocalDocs")
	if api.Type() != js.TypeObject || api.IsNull() {
		api = js.Global().Get("Object").New()
	}

	var active abi.Activation
	var activeReady bool
	var browser *localbrowser.Browser
	activateFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 2 {
			return failure("activate expects descriptor and manifest")
		}
		descriptor, err := descriptorFromJS(args[0])
		if err != nil {
			return failure(err.Error())
		}
		manifest, err := manifestFromJS(args[1])
		if err != nil {
			return failure(err.Error())
		}
		candidate, err := abi.Admit(descriptor, manifest)
		if err != nil {
			return failure(err.Error())
		}
		active = candidate
		activeReady = true
		children := make([]any, 0, len(active.Inventory()))
		for _, artifact := range active.Inventory() {
			children = append(children, map[string]any{"path": artifact.Path, "kind": artifact.Kind, "length": artifact.Length, "sha256": artifact.SHA256})
		}
		return map[string]any{
			"ok": true, "catalogId": active.CatalogID(), "publicationKey": active.PublicationKey(),
			"snapshotId": active.SnapshotID(), "revisionId": active.RevisionID(),
			"projectionDigest": active.ProjectionDigest(), "children": children,
		}
	})
	allowsFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if !activeReady || len(args) != 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
			return false
		}
		return active.Allows(args[0].String(), args[1].String())
	})
	resolveFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if !activeReady || len(args) != 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
			return false
		}
		artifact, ok := active.Resolve(args[0].String(), args[1].String())
		if !ok {
			return false
		}
		return map[string]any{"path": artifact.Path, "kind": artifact.Kind, "length": artifact.Length, "sha256": artifact.SHA256}
	})
	prepareFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 4 || !object(args[1]) || !object(args[2]) || !object(args[3]) {
			return failure("prepare expects descriptor, manifest, catalog, and children")
		}
		descriptor, err := browserDescriptorFromJS(args[0])
		if err != nil {
			return failure(err.Error())
		}
		manifestBytes, err := canonicalJSBytes(args[1])
		if err != nil {
			return failure(err.Error())
		}
		catalogBytes, err := canonicalJSBytes(args[2])
		if err != nil {
			return failure(err.Error())
		}
		children, err := childBytesFromJS(args[3])
		if err != nil {
			return failure(err.Error())
		}
		candidate, err := localbrowser.Prepare(descriptor, manifestBytes, catalogBytes, children)
		if err != nil {
			return failure(err.Error())
		}
		browser = candidate
		return map[string]any{"ok": true, "catalogId": descriptor.CatalogID, "snapshotId": descriptor.SnapshotID}
	})
	renderFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if browser == nil || len(args) != 1 || !object(args[0]) {
			return failure("render expects prepared route state")
		}
		route, err := browserRouteFromJS(args[0])
		if err != nil {
			return failure(err.Error())
		}
		page, err := browser.Render(context.Background(), route)
		if err != nil {
			return failure(err.Error())
		}
		return map[string]any{"ok": true, "mainHtml": page.MainHTML, "sidebarHtml": page.SidebarHTML, "title": page.Title, "canonical": page.Canonical}
	})
	searchFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if browser == nil || len(args) != 1 || args[0].Type() != js.TypeString {
			return failure("search expects a prepared query")
		}
		records, err := browser.Search(context.Background(), args[0].String())
		if err != nil {
			return failure(err.Error())
		}
		results := make([]any, len(records))
		for index, record := range records {
			results[index] = map[string]any{
				"detailId": record.DetailID, "documentKey": record.DocumentKey, "kind": record.Kind,
				"title": record.Title, "description": record.Description, "href": record.Href,
				"operationId": record.OperationID, "method": record.Method, "path": record.Path, "schemaName": record.SchemaName,
			}
		}
		return map[string]any{"ok": true, "results": results}
	})
	api.Set("activate", activateFunc)
	api.Set("allows", allowsFunc)
	api.Set("resolve", resolveFunc)
	api.Set("prepare", prepareFunc)
	api.Set("render", renderFunc)
	api.Set("search", searchFunc)
	js.Global().Set("ManjaLocalDocs", api)
	select {}
}

func browserDescriptorFromJS(value js.Value) (localdocs.DescriptorV1, error) {
	base, err := descriptorFromJS(value)
	if err != nil {
		return localdocs.DescriptorV1{}, err
	}
	staticValue := value.Get("static")
	if !object(staticValue) {
		return localdocs.DescriptorV1{}, errors.New("descriptor static fields are required")
	}
	return localdocs.DescriptorV1{
		SchemaVersion: base.SchemaVersion, CatalogID: base.CatalogID, PublicationKey: base.PublicationKey,
		Public: base.Public, Anonymous: base.Anonymous, PublicationBase: base.PublicationBase,
		SnapshotID: base.SnapshotID, RevisionID: base.RevisionID, ProjectionFormat: base.ProjectionFormat,
		ProjectionDigest: base.ProjectionDigest, ProjectionManifestURL: base.ProjectionManifestURL,
		CatalogURL: base.CatalogURL, SearchDataBase: base.SearchDataBase, ProjectionDataBase: base.ProjectionDataBase,
		Static: &localdocs.StaticDescriptorV1{
			DeploymentBase: stringProperty(staticValue, "deploymentBase"), WorkerURL: stringProperty(staticValue, "workerUrl"),
			WorkerScope: stringProperty(staticValue, "workerScope"), OfflineShellURL: stringProperty(staticValue, "offlineShellUrl"),
			ExportManifestURL: stringProperty(staticValue, "exportManifestUrl"),
		},
	}, nil
}

func canonicalJSBytes(value js.Value) ([]byte, error) {
	encoded := js.Global().Get("JSON").Call("stringify", value)
	if encoded.Type() != js.TypeString {
		return nil, errors.New("runtime JSON value cannot be encoded")
	}
	return []byte(canonicalizeJSONString(encoded.String())), nil
}

func canonicalizeJSONString(value string) string {
	return canonicalJSONEscapes.Replace(value)
}

func childBytesFromJS(value js.Value) (map[string][]byte, error) {
	keys := js.Global().Get("Object").Call("keys", value)
	if !array(keys) {
		return nil, errors.New("runtime children must be an object")
	}
	result := make(map[string][]byte, keys.Length())
	for index := 0; index < keys.Length(); index++ {
		key := keys.Index(index)
		if key.Type() != js.TypeString {
			return nil, errors.New("runtime child path is invalid")
		}
		encoded, err := canonicalJSBytes(value.Get(key.String()))
		if err != nil {
			return nil, err
		}
		result[key.String()] = encoded
	}
	return result, nil
}

func browserRouteFromJS(value js.Value) (localbrowser.Route, error) {
	route := localbrowser.Route{DocumentKey: stringProperty(value, "documentKey"), Selected: stringProperty(value, "selected")}
	node := value.Get("node")
	if node.Type() == js.TypeNumber {
		ordinal, err := uintProperty(value, "node")
		if err != nil {
			return localbrowser.Route{}, err
		}
		route.Node = &ordinal
	}
	groups := value.Get("groups")
	if groups.Type() != js.TypeUndefined {
		if !array(groups) {
			return localbrowser.Route{}, errors.New("route groups must be an array")
		}
		for index := 0; index < groups.Length(); index++ {
			if groups.Index(index).Type() != js.TypeString {
				return localbrowser.Route{}, errors.New("route group must be a string")
			}
			route.Groups = append(route.Groups, groups.Index(index).String())
		}
	}
	return route, nil
}

func descriptorFromJS(value js.Value) (abi.Descriptor, error) {
	if !object(value) {
		return abi.Descriptor{}, errors.New("descriptor must be an object")
	}
	schemaVersion, err := uintProperty(value, "schemaVersion")
	if err != nil {
		return abi.Descriptor{}, err
	}
	return abi.Descriptor{
		SchemaVersion: schemaVersion,
		CatalogID:     stringProperty(value, "catalogId"), PublicationKey: stringProperty(value, "publicationKey"),
		Public: value.Get("public").Type() == js.TypeBoolean && value.Get("public").Bool(), Anonymous: value.Get("anonymous").Type() == js.TypeBoolean && value.Get("anonymous").Bool(),
		PublicationBase: stringProperty(value, "publicationBase"), SnapshotID: stringProperty(value, "snapshotId"),
		RevisionID: stringProperty(value, "revisionId"), ProjectionFormat: stringProperty(value, "projectionFormat"),
		ProjectionDigest: stringProperty(value, "projectionDigest"), ProjectionManifestURL: stringProperty(value, "projectionManifestUrl"),
		CatalogURL: stringProperty(value, "catalogUrl"), SearchDataBase: stringProperty(value, "searchDataBase"),
		ProjectionDataBase: stringProperty(value, "projectionDataBase"),
	}, nil
}

func manifestFromJS(value js.Value) (abi.Manifest, error) {
	if !object(value) {
		return abi.Manifest{}, errors.New("manifest must be an object")
	}
	schemaVersion, err := uintProperty(value, "schemaVersion")
	if err != nil {
		return abi.Manifest{}, err
	}
	identity := value.Get("identity")
	if !object(identity) {
		return abi.Manifest{}, errors.New("manifest identity must be an object")
	}
	identityVersion, err := uintProperty(identity, "schemaVersion")
	if err != nil {
		return abi.Manifest{}, err
	}
	projectionFormat := stringProperty(identity, "projectionFormat")
	if projectionFormat == "" {
		versions := identity.Get("versions")
		if object(versions) {
			projectionFormat = stringProperty(versions, "projectionFormat")
		}
	}
	children := value.Get("children")
	if !array(children) {
		return abi.Manifest{}, errors.New("manifest children must be an array")
	}
	result := abi.Manifest{
		SchemaVersion: schemaVersion, SnapshotID: stringProperty(value, "snapshotId"),
		Identity:       abi.Identity{SchemaVersion: identityVersion, CatalogID: stringProperty(identity, "catalogId"), RevisionID: stringProperty(identity, "revisionId"), ProjectionFormat: projectionFormat},
		IdentityDigest: stringProperty(value, "identityDigest"), Children: make([]abi.Artifact, 0, children.Length()),
	}
	for index := 0; index < children.Length(); index++ {
		child := children.Index(index)
		if !object(child) {
			return abi.Manifest{}, errors.New("manifest child must be an object")
		}
		length, err := uintProperty(child, "length")
		if err != nil {
			return abi.Manifest{}, err
		}
		result.Children = append(result.Children, abi.Artifact{Path: stringProperty(child, "path"), Kind: stringProperty(child, "kind"), Length: uint64(length), SHA256: stringProperty(child, "sha256")})
	}
	return result, nil
}

func object(value js.Value) bool {
	return value.Type() == js.TypeObject && !value.IsNull()
}

func array(value js.Value) bool {
	return object(value) && js.Global().Get("Array").Call("isArray", value).Bool()
}

func stringProperty(objectValue js.Value, name string) string {
	property := objectValue.Get(name)
	if property.Type() != js.TypeString {
		return ""
	}
	return property.String()
}

func uintProperty(objectValue js.Value, name string) (uint32, error) {
	property := objectValue.Get(name)
	if property.Type() != js.TypeNumber {
		return 0, errors.New("manifest numeric property is invalid")
	}
	value := property.Float()
	if value < 0 || value > float64(^uint32(0)) || math.Trunc(value) != value {
		return 0, errors.New("manifest numeric property is invalid")
	}
	return uint32(value), nil
}

func failure(message string) map[string]any {
	return map[string]any{"ok": false, "error": message}
}
