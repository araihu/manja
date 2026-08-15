//go:build js && wasm

package main

import (
	"errors"
	"math"
	"syscall/js"

	"github.com/araihu/manja/internal/localdocs/abi"
)

var (
	activateFunc js.Func
	allowsFunc   js.Func
)

func main() {
	api := js.Global().Get("ManjaLocalDocs")
	if api.Type() != js.TypeObject || api.IsNull() {
		api = js.Global().Get("Object").New()
	}

	var active abi.Activation
	var activeReady bool
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
			"ok": true, "catalogId": active.CatalogID(), "snapshotId": active.SnapshotID(),
			"revisionId": active.RevisionID(), "children": children,
		}
	})
	allowsFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if !activeReady || len(args) != 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
			return false
		}
		return active.Allows(args[0].String(), args[1].String())
	})
	api.Set("activate", activateFunc)
	api.Set("allows", allowsFunc)
	js.Global().Set("ManjaLocalDocs", api)
	select {}
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
	children := value.Get("children")
	if !array(children) {
		return abi.Manifest{}, errors.New("manifest children must be an array")
	}
	result := abi.Manifest{
		SchemaVersion: schemaVersion, SnapshotID: stringProperty(value, "snapshotId"),
		Identity:       abi.Identity{SchemaVersion: identityVersion, CatalogID: stringProperty(identity, "catalogId"), RevisionID: stringProperty(identity, "revisionId"), ProjectionFormat: stringProperty(identity, "projectionFormat")},
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
