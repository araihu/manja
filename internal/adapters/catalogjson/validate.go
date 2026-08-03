package catalogjson

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

func validateCatalog(value catalog.CatalogArtifactV1) error {
	if value.SchemaVersion != 1 {
		return fmt.Errorf("catalogjson: catalog schema version 1 is required")
	}
	if err := domain.ValidateCatalogID(value.CatalogID); err != nil {
		return fmt.Errorf("catalogjson: %w", err)
	}
	seenDocuments := make(map[string]struct{}, len(value.Documents))
	defaultFound := value.DefaultDocumentKey == ""
	for index, document := range value.Documents {
		if err := domain.ValidateCatalogDocumentKey(document.Key); err != nil {
			return fmt.Errorf("catalogjson: %w", err)
		}
		if index > 0 && value.Documents[index-1].Key >= document.Key {
			return fmt.Errorf("catalogjson: documents are not strictly sorted")
		}
		if _, duplicate := seenDocuments[document.Key]; duplicate {
			return fmt.Errorf("catalogjson: document %q is duplicated", document.Key)
		}
		seenDocuments[document.Key] = struct{}{}
		defaultFound = defaultFound || value.DefaultDocumentKey == document.Key
		if err := validateChildPath(document.SourceChild); err != nil {
			return err
		}
		for operationIndex, operation := range document.Operations {
			if err := validateDetailID(operation.DetailID); err != nil {
				return err
			}
			if err := validateChildPath(operation.DetailChild); err != nil {
				return err
			}
			if operationIndex > 0 {
				previous := document.Operations[operationIndex-1]
				if previous.Path > operation.Path || previous.Path == operation.Path && previous.Method >= operation.Method {
					return fmt.Errorf("catalogjson: operations are not strictly sorted")
				}
			}
		}
		for schemaIndex, schema := range document.Schemas {
			if err := validateDetailID(schema.DetailID); err != nil {
				return err
			}
			if err := validateChildPath(schema.DetailChild); err != nil {
				return err
			}
			if schemaIndex > 0 && document.Schemas[schemaIndex-1].Name >= schema.Name {
				return fmt.Errorf("catalogjson: schemas are not strictly sorted")
			}
		}
		var expectedOrdinal uint32
		for shardIndex, shard := range document.SchemaNodeShards {
			if err := validateShardReference(shard); err != nil {
				return err
			}
			if shardIndex > 0 && shard.FirstOrdinal != expectedOrdinal {
				return fmt.Errorf("catalogjson: schema-node shard ranges are not contiguous")
			}
			expectedOrdinal = shard.LastOrdinal + 1
		}
	}
	if !defaultFound {
		return fmt.Errorf("catalogjson: default document does not exist")
	}
	return nil
}

func validateDetailShard(value catalog.DetailShardV1) error {
	if value.SchemaVersion != 1 {
		return fmt.Errorf("catalogjson: detail schema version 1 is required")
	}
	if err := domain.ValidateCatalogDocumentKey(value.DocumentKey); err != nil {
		return err
	}
	for index, record := range value.Records {
		if err := validateDetailID(record.ID); err != nil {
			return err
		}
		if index > 0 && value.Records[index-1].ID >= record.ID {
			return fmt.Errorf("catalogjson: detail records are not strictly sorted")
		}
		switch record.Kind {
		case "operation":
			if record.Operation == nil || record.Schema != nil || record.Operation.ID != string(record.ID) || record.Operation.Anchor != string(record.ID) || record.Operation.HeadingID != string(record.ID) {
				return fmt.Errorf("catalogjson: malformed operation detail %q", record.ID)
			}
		case "schema":
			if record.Schema == nil || record.Operation != nil || record.Schema.ID != string(record.ID) || record.Schema.Anchor != string(record.ID) || record.Schema.HeadingID != string(record.ID) {
				return fmt.Errorf("catalogjson: malformed schema detail %q", record.ID)
			}
		default:
			return fmt.Errorf("catalogjson: detail kind %q is unsupported", record.Kind)
		}
	}
	return nil
}

func validateSchemaNodeShard(value catalog.SchemaNodeShardV1) error {
	if value.SchemaVersion != 1 {
		return fmt.Errorf("catalogjson: schema-node schema version 1 is required")
	}
	if err := domain.ValidateCatalogDocumentKey(value.DocumentKey); err != nil {
		return err
	}
	for index, node := range value.Nodes {
		if node.Ordinal != value.FirstOrdinal+uint32(index) {
			return fmt.Errorf("catalogjson: schema-node ordinals are not contiguous")
		}
	}
	return nil
}

func validateManifest(value catalog.ManifestV1) error {
	if value.SchemaVersion != 1 || !validSnapshotID(value.SnapshotID) {
		return fmt.Errorf("catalogjson: manifest identity is invalid")
	}
	identityBytes, err := json.Marshal(value.Identity)
	if err != nil {
		return fmt.Errorf("catalogjson: encode identity: %w", err)
	}
	digest := sha256.Sum256(identityBytes)
	want := catalog.SnapshotID("snapshot-sha256-" + hex.EncodeToString(digest[:]))
	if value.SnapshotID != want {
		return fmt.Errorf("catalogjson: snapshot ID does not match identity")
	}
	if !equalChildren(value.Children, value.Identity.Children) {
		return fmt.Errorf("catalogjson: manifest children differ from identity")
	}
	for index, child := range value.Children {
		if err := validateChildIdentity(child); err != nil {
			return err
		}
		if index > 0 && value.Children[index-1].Path >= child.Path {
			return fmt.Errorf("catalogjson: manifest children are not strictly sorted")
		}
	}
	return nil
}

func ValidateCatalogManifest(directory catalog.CatalogArtifactV1, manifest catalog.ManifestV1) error {
	if err := validateCatalog(directory); err != nil {
		return err
	}
	children := make(map[string]catalog.ChildIdentityV1, len(manifest.Children))
	for _, child := range manifest.Children {
		children[child.Path] = child
	}
	require := func(pathValue string) error {
		if _, exists := children[pathValue]; !exists {
			return fmt.Errorf("catalogjson: referenced child %q is undeclared", pathValue)
		}
		return nil
	}
	if err := require("catalog.json"); err != nil {
		return err
	}
	for _, document := range directory.Documents {
		if err := require(document.SourceChild); err != nil {
			return err
		}
		for _, operation := range document.Operations {
			if err := require(operation.DetailChild); err != nil {
				return err
			}
		}
		for _, schema := range document.Schemas {
			if err := require(schema.DetailChild); err != nil {
				return err
			}
		}
		for _, shard := range document.SchemaNodeShards {
			child, exists := children[shard.Path]
			if !exists || child.Length != shard.Length || child.SHA256 != shard.SHA256 {
				return fmt.Errorf("catalogjson: schema-node child %q is undeclared or changed", shard.Path)
			}
		}
	}
	return nil
}

func validateDetailID(value domain.DetailID) error {
	text := string(value)
	const prefix = "detail-sha256-"
	if !strings.HasPrefix(text, prefix) || !lowerHex(strings.TrimPrefix(text, prefix), 64) {
		return fmt.Errorf("catalogjson: detail ID %q is invalid", value)
	}
	return nil
}

func validateShardReference(value catalog.ShardReferenceV1) error {
	if err := validateChildPath(value.Path); err != nil {
		return err
	}
	if value.Records == 0 || value.LastOrdinal < value.FirstOrdinal || value.LastOrdinal-value.FirstOrdinal+1 != value.Records || value.Length == 0 || !lowerHex(value.SHA256, 64) {
		return fmt.Errorf("catalogjson: shard reference %q is invalid", value.Path)
	}
	return nil
}

func validateChildIdentity(value catalog.ChildIdentityV1) error {
	if err := validateChildPath(value.Path); err != nil {
		return err
	}
	if value.Kind == "" || value.Length == 0 || !lowerHex(value.SHA256, 64) {
		return fmt.Errorf("catalogjson: child identity %q is invalid", value.Path)
	}
	return nil
}

func validateChildPath(value string) error {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, `\`) || value == "." || path.Clean(value) != value || strings.HasPrefix(value, "../") {
		return fmt.Errorf("catalogjson: child path %q is invalid", value)
	}
	return nil
}

func lowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validSnapshotID(value catalog.SnapshotID) bool {
	const prefix = "snapshot-sha256-"
	return strings.HasPrefix(string(value), prefix) && lowerHex(strings.TrimPrefix(string(value), prefix), 64)
}

func equalChildren(left, right []catalog.ChildIdentityV1) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
