package localdocs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
)

const projectionFormatV2 = "projection-v2"

type ProjectionArtifact struct {
	Path   string
	Kind   string
	Length uint64
	SHA256 string
}

type Activation struct {
	catalogID      string
	publicationKey string
	snapshotID     string
	revisionID     string
	inventory      []ProjectionArtifact
}

func (activation Activation) PublicationKey() string { return activation.publicationKey }

func (activation Activation) CatalogID() string { return activation.catalogID }

func (activation Activation) SnapshotID() string { return activation.snapshotID }

func (activation Activation) RevisionID() string { return activation.revisionID }

func (activation Activation) Inventory() []ProjectionArtifact {
	return append([]ProjectionArtifact(nil), activation.inventory...)
}

func (activation Activation) artifact(pathValue, kind string) (ProjectionArtifact, bool) {
	index := sort.Search(len(activation.inventory), func(index int) bool {
		return activation.inventory[index].Path >= pathValue
	})
	if index == len(activation.inventory) || activation.inventory[index].Path != pathValue || activation.inventory[index].Kind != kind {
		return ProjectionArtifact{}, false
	}
	return activation.inventory[index], true
}

// Admit validates the complete immutable activation envelope without reading
// the network or filesystem. A failure always returns an empty Activation.
func Admit(descriptor DescriptorV1, manifestBytes []byte) (Activation, error) {
	if err := validateDescriptor(descriptor); err != nil {
		return Activation{}, err
	}
	manifest, err := catalogjson.DecodeManifest(manifestBytes)
	if err != nil {
		return Activation{}, errors.New("local docs manifest is invalid")
	}
	if string(manifest.SnapshotID) != descriptor.SnapshotID || manifest.Identity.CatalogID != descriptor.CatalogID || manifest.Identity.RevisionID != descriptor.RevisionID || manifest.Identity.Versions.ProjectionFormat != descriptor.ProjectionFormat {
		return Activation{}, errors.New("local docs manifest identity differs")
	}
	identityBytes, err := json.Marshal(manifest.Identity)
	if err != nil {
		return Activation{}, errors.New("local docs manifest identity cannot be encoded")
	}
	digest := sha256.Sum256(identityBytes)
	if hex.EncodeToString(digest[:]) != descriptor.ProjectionDigest {
		return Activation{}, errors.New("local docs manifest digest differs")
	}

	inventory := make([]ProjectionArtifact, 0)
	for _, child := range manifest.Children {
		projectedPath := strings.HasPrefix(child.Path, "details/") || strings.HasPrefix(child.Path, "schema-nodes/")
		switch child.Kind {
		case "detail":
			if !validProjectionPath(child.Path, "details/") {
				return Activation{}, errors.New("local docs detail path is invalid")
			}
		case "schema-node":
			if !validProjectionPath(child.Path, "schema-nodes/") {
				return Activation{}, errors.New("local docs schema-node path is invalid")
			}
		default:
			if projectedPath {
				return Activation{}, errors.New("local docs projection kind is invalid")
			}
			continue
		}
		inventory = append(inventory, ProjectionArtifact{
			Path: child.Path, Kind: child.Kind, Length: child.Length, SHA256: child.SHA256,
		})
	}
	sort.Slice(inventory, func(left, right int) bool { return inventory[left].Path < inventory[right].Path })
	return Activation{
		catalogID: descriptor.CatalogID, publicationKey: descriptor.PublicationKey,
		snapshotID: descriptor.SnapshotID,
		revisionID: descriptor.RevisionID,
		inventory:  inventory,
	}, nil
}

func validateDescriptor(descriptor DescriptorV1) error {
	if descriptor.SchemaVersion != 1 || domain.ValidateCatalogID(descriptor.CatalogID) != nil || domain.ValidateCatalogPublicationKey(descriptor.PublicationKey) != nil {
		return errors.New("local docs descriptor identity is invalid")
	}
	if !validPublicationBase(descriptor.PublicationBase) || domain.ValidateCanonicalIdentity("local docs revision id", descriptor.RevisionID, false) != nil {
		return errors.New("local docs descriptor route is invalid")
	}
	if descriptor.ProjectionFormat != projectionFormatV2 || !lowerHexSHA256(descriptor.ProjectionDigest) {
		return errors.New("local docs descriptor projection is invalid")
	}
	wantSnapshot := "snapshot-sha256-" + descriptor.ProjectionDigest
	if descriptor.SnapshotID != wantSnapshot {
		return errors.New("local docs descriptor snapshot is invalid")
	}
	wantManifest := descriptor.PublicationBase + "snapshots/" + descriptor.SnapshotID + "/manifest.json"
	wantCatalog := descriptor.PublicationBase + "snapshots/" + descriptor.SnapshotID + "/catalog.json"
	wantSearch := descriptor.PublicationBase + "snapshots/" + descriptor.SnapshotID + "/search-data/"
	wantDataBase := descriptor.PublicationBase + "snapshots/" + descriptor.SnapshotID + "/projection-data/"
	if descriptor.ProjectionManifestURL != wantManifest || descriptor.CatalogURL != wantCatalog || descriptor.SearchDataBase != wantSearch || descriptor.ProjectionDataBase != wantDataBase {
		return errors.New("local docs descriptor URL is invalid")
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
	return len(value) > len(prefix) && strings.HasPrefix(value, prefix) && path.Clean(value) == value && !strings.Contains(value, `\`)
}

func lowerHexSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}
