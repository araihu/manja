// Package abi contains the parser-free data boundary used by the public-docs
// browser ABI. It accepts values that the browser already decoded and never
// performs I/O or decodes JSON itself.
package abi

import (
	"errors"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	activationSchemaVersion = 1
	projectionFormat        = "projection-v2"
	maxProjectionChildren   = 10000
	maxProjectionChildBytes = 2 << 20
)

// Descriptor is the browser-facing immutable identity and route envelope. It
// deliberately contains no credentials or runtime state.
type Descriptor struct {
	SchemaVersion         uint32
	CatalogID             string
	PublicationKey        string
	PublicationBase       string
	SnapshotID            string
	RevisionID            string
	ProjectionFormat      string
	ProjectionDigest      string
	ProjectionManifestURL string
	CatalogURL            string
	SearchDataBase        string
	ProjectionDataBase    string
}

// Identity is the subset of manifest identity needed before the full
// projection is handed to the renderer. The browser verifies the canonical
// identity digest before calling this boundary.
type Identity struct {
	SchemaVersion    uint32
	CatalogID        string
	RevisionID       string
	ProjectionFormat string
}

// Artifact describes one immutable projection child declared by the manifest.
type Artifact struct {
	Path   string
	Kind   string
	Length uint64
	SHA256 string
}

// Manifest is the already-decoded, public projection manifest envelope.
type Manifest struct {
	SchemaVersion  uint32
	SnapshotID     string
	Identity       Identity
	IdentityDigest string
	Children       []Artifact
}

// Activation is immutable after Admit returns. Its inventory is private and
// copied so a JS caller cannot mutate the allowlist through a retained input.
type Activation struct {
	descriptor Descriptor
	inventory  []Artifact
}

// Admit validates the typed browser ABI envelope. It does not parse JSON,
// access the network, touch the filesystem, or render HTML.
func Admit(descriptor Descriptor, manifest Manifest) (Activation, error) {
	if err := validateDescriptor(descriptor); err != nil {
		return Activation{}, err
	}
	if manifest.SchemaVersion != activationSchemaVersion || manifest.SnapshotID != descriptor.SnapshotID {
		return Activation{}, errors.New("local docs ABI manifest identity is invalid")
	}
	if manifest.Identity.SchemaVersion != activationSchemaVersion || manifest.Identity.CatalogID != descriptor.CatalogID || manifest.Identity.RevisionID != descriptor.RevisionID || manifest.Identity.ProjectionFormat != descriptor.ProjectionFormat || manifest.IdentityDigest != descriptor.ProjectionDigest {
		return Activation{}, errors.New("local docs ABI manifest identity differs")
	}
	if len(manifest.Children) > maxProjectionChildren {
		return Activation{}, errors.New("local docs ABI manifest children exceed limit")
	}

	inventory := make([]Artifact, 0, len(manifest.Children))
	seen := make(map[string]struct{}, len(manifest.Children))
	for _, child := range manifest.Children {
		isProjection := strings.HasPrefix(child.Path, "details/") || strings.HasPrefix(child.Path, "schema-nodes/")
		switch child.Kind {
		case "detail":
			if !validProjectionPath(child.Path, "details/") {
				return Activation{}, errors.New("local docs ABI detail path is invalid")
			}
		case "schema-node":
			if !validProjectionPath(child.Path, "schema-nodes/") {
				return Activation{}, errors.New("local docs ABI schema-node path is invalid")
			}
		default:
			if isProjection {
				return Activation{}, errors.New("local docs ABI projection kind is invalid")
			}
			continue
		}
		if _, duplicate := seen[child.Path]; duplicate || child.Length == 0 || child.Length > maxProjectionChildBytes || !lowerHexSHA256(child.SHA256) {
			return Activation{}, errors.New("local docs ABI projection child is invalid")
		}
		seen[child.Path] = struct{}{}
		inventory = append(inventory, child)
	}
	sort.Slice(inventory, func(left, right int) bool { return inventory[left].Path < inventory[right].Path })
	return Activation{descriptor: descriptor, inventory: inventory}, nil
}

// CatalogID returns admitted public catalog identity.
func (activation Activation) CatalogID() string { return activation.descriptor.CatalogID }

// PublicationKey returns the composition-authorized public cache namespace.
func (activation Activation) PublicationKey() string { return activation.descriptor.PublicationKey }

// SnapshotID returns admitted immutable snapshot identity.
func (activation Activation) SnapshotID() string { return activation.descriptor.SnapshotID }

// RevisionID returns admitted source revision identity.
func (activation Activation) RevisionID() string { return activation.descriptor.RevisionID }

// ProjectionDigest returns the immutable projection identity bound to the
// admitted snapshot and manifest.
func (activation Activation) ProjectionDigest() string { return activation.descriptor.ProjectionDigest }

// Inventory returns a defensive copy of the admitted projection children.
func (activation Activation) Inventory() []Artifact {
	return append([]Artifact(nil), activation.inventory...)
}

// Artifact returns a defensive copy of one admitted projection child.
func (activation Activation) Artifact(pathValue string) (Artifact, bool) {
	index := sort.Search(len(activation.inventory), func(index int) bool { return activation.inventory[index].Path >= pathValue })
	if index == len(activation.inventory) || activation.inventory[index].Path != pathValue {
		return Artifact{}, false
	}
	return activation.inventory[index], true
}

// Resolve returns the immutable metadata for an admitted GET projection child.
// It is intentionally pathname-only: callers must perform same-origin URL
// parsing before entering the Wasm ABI.
func (activation Activation) Resolve(method, pathValue string) (Artifact, bool) {
	if method != "GET" || strings.ContainsAny(pathValue, "?#%") || strings.Contains(pathValue, `\`) || path.Clean(pathValue) != pathValue {
		return Artifact{}, false
	}
	for _, artifact := range activation.inventory {
		if activation.descriptor.ProjectionDataBase+artifact.Path == pathValue {
			return artifact, true
		}
	}
	return Artifact{}, false
}

// Allows reports whether a GET request is an admitted projection child for
// this publication.
func (activation Activation) Allows(method, pathValue string) bool {
	_, ok := activation.Resolve(method, pathValue)
	return ok
}

func validateDescriptor(descriptor Descriptor) error {
	if descriptor.SchemaVersion != activationSchemaVersion || !validCatalogKey(descriptor.CatalogID) || !validCatalogKey(descriptor.PublicationKey) {
		return errors.New("local docs ABI descriptor identity is invalid")
	}
	if !validPublicationBase(descriptor.PublicationBase) || !validCanonicalIdentity(descriptor.RevisionID) {
		return errors.New("local docs ABI descriptor route is invalid")
	}
	if descriptor.ProjectionFormat != projectionFormat || !lowerHexSHA256(descriptor.ProjectionDigest) || descriptor.SnapshotID != "snapshot-sha256-"+descriptor.ProjectionDigest {
		return errors.New("local docs ABI descriptor projection is invalid")
	}
	wantManifest := descriptor.PublicationBase + "snapshots/" + descriptor.SnapshotID + "/manifest.json"
	wantCatalog := descriptor.PublicationBase + "snapshots/" + descriptor.SnapshotID + "/catalog.json"
	wantSearch := descriptor.PublicationBase + "snapshots/" + descriptor.SnapshotID + "/search-data/"
	wantDataBase := descriptor.PublicationBase + "snapshots/" + descriptor.SnapshotID + "/projection-data/"
	if descriptor.ProjectionManifestURL != wantManifest || descriptor.CatalogURL != wantCatalog || descriptor.SearchDataBase != wantSearch || descriptor.ProjectionDataBase != wantDataBase {
		return errors.New("local docs ABI descriptor URL is invalid")
	}
	return nil
}

func validPublicationBase(value string) bool {
	if value == "/" {
		return true
	}
	if len(value) < 3 || len(value) > 1024 || !strings.HasPrefix(value, "/") || !strings.HasSuffix(value, "/") || strings.Contains(value, `\`) || strings.ContainsAny(value, "%?#") {
		return false
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return false
		}
	}
	trimmed := strings.TrimSuffix(value, "/")
	return path.Clean(trimmed) == trimmed && !strings.Contains(trimmed, "//")
}

func validProjectionPath(value, prefix string) bool {
	if len(value) <= len(prefix) || !strings.HasPrefix(value, prefix) || path.Clean(value) != value || strings.Contains(value, `\`) || strings.ContainsAny(value, "%?#") {
		return false
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func lowerHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func validCanonicalIdentity(value string) bool {
	if value == "" || !utf8.ValidString(value) || value != strings.TrimSpace(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validCatalogKey(value string) bool {
	if !validCanonicalIdentity(value) || len(value) > 64 || !catalogKeyEdge(value[0]) || !catalogKeyEdge(value[len(value)-1]) {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func catalogKeyEdge(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}
