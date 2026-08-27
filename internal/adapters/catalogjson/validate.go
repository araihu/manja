package catalogjson

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"unicode"

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
	if err := validateChildPath(value.SearchChild); err != nil {
		return err
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
			if !lowerHex(schema.CanonicalSHA256, 64) || !lowerHex(schema.ProjectionSHA256, 64) {
				return fmt.Errorf("catalogjson: schema %q semantic digests are invalid", schema.Name)
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

func validateSearchDirectory(value catalog.SearchDirectoryV1) error {
	if value.SchemaVersion != 1 || value.SearchVersion != 1 {
		return fmt.Errorf("catalogjson: search directory version is unsupported")
	}
	for index, bucket := range value.ExactBuckets {
		if len(bucket.Prefix) != 1 || !lowerHex(bucket.Prefix, 1) || index > 0 && value.ExactBuckets[index-1].Prefix >= bucket.Prefix {
			return fmt.Errorf("catalogjson: exact search buckets are not strictly sorted")
		}
		if err := validateSearchSegmentReference(bucket.SearchSegmentReferenceV1); err != nil {
			return err
		}
	}
	if err := validateSearchRoutes(value.TokenRoutes, value.PostingSegments); err != nil {
		return err
	}
	if err := validateSearchRoutes(value.TrigramRoutes, value.TrigramSegments); err != nil {
		return err
	}
	for _, reference := range value.PostingSegments {
		if err := validateSearchSegmentReference(reference); err != nil {
			return err
		}
	}
	for _, reference := range value.TrigramSegments {
		if err := validateSearchSegmentReference(reference); err != nil {
			return err
		}
	}
	var nextRecord uint64
	for _, reference := range value.RecordSegments {
		if err := validateChildPath(reference.Path); err != nil {
			return err
		}
		if uint64(reference.FirstRecord) != nextRecord || reference.Records == 0 || reference.Length == 0 || reference.Length > maxSearchSegmentBytes || !lowerHex(reference.SHA256, 64) {
			return fmt.Errorf("catalogjson: search record reference %q is invalid", reference.Path)
		}
		nextRecord += uint64(reference.Records)
	}
	if nextRecord != uint64(len(value.Ranks)) {
		return fmt.Errorf("catalogjson: search rank record count is invalid")
	}
	for _, rank := range value.Ranks {
		if rank.Title == "" {
			return fmt.Errorf("catalogjson: search rank record title is empty")
		}
		if rank.Kind != "" && rank.Kind != "operation" && rank.Kind != "schema" {
			return fmt.Errorf("catalogjson: search rank record kind is invalid")
		}
	}
	return nil
}

func validateSearchExactSegment(value catalog.SearchExactSegmentV1) error {
	if value.SchemaVersion != 1 || value.SearchVersion != 1 || len(value.Entries) == 0 {
		return fmt.Errorf("catalogjson: exact search segment is invalid")
	}
	for index, entry := range value.Entries {
		if err := validateSearchKey(entry.Key); err != nil {
			return err
		}
		if index > 0 && value.Entries[index-1].Key >= entry.Key {
			return fmt.Errorf("catalogjson: exact search entries are not strictly sorted")
		}
		if len(entry.Matches) == 0 {
			return fmt.Errorf("catalogjson: exact search entry %q has no matches", entry.Key)
		}
		seenRecords := make(map[uint32]struct{}, len(entry.Matches))
		for matchIndex, match := range entry.Matches {
			if match.Priority < 1 || match.Priority > 3 {
				return fmt.Errorf("catalogjson: exact search priority is invalid")
			}
			if _, duplicate := seenRecords[match.Record]; duplicate {
				return fmt.Errorf("catalogjson: exact search record is duplicated")
			}
			seenRecords[match.Record] = struct{}{}
			if matchIndex > 0 {
				previous := entry.Matches[matchIndex-1]
				if previous.Priority > match.Priority || previous.Priority == match.Priority && previous.Record >= match.Record {
					return fmt.Errorf("catalogjson: exact search matches are not strictly sorted")
				}
			}
		}
	}
	return nil
}

func validateSearchPostingSegment(value catalog.SearchPostingSegmentV1) error {
	if value.SchemaVersion != 1 || value.SearchVersion != 1 || len(value.Entries) == 0 {
		return fmt.Errorf("catalogjson: posting search segment is invalid")
	}
	for index, entry := range value.Entries {
		if err := validateSearchKey(entry.Key); err != nil {
			return err
		}
		if index > 0 && value.Entries[index-1].Key >= entry.Key {
			return fmt.Errorf("catalogjson: posting search entries are not strictly sorted")
		}
		if len(entry.Records) == 0 {
			return fmt.Errorf("catalogjson: posting search entry %q has no records", entry.Key)
		}
		for recordIndex := 1; recordIndex < len(entry.Records); recordIndex++ {
			if entry.Records[recordIndex-1] >= entry.Records[recordIndex] {
				return fmt.Errorf("catalogjson: posting search records are not strictly sorted")
			}
		}
	}
	return nil
}

func validateSearchRecordSegment(value catalog.SearchRecordSegmentV1) error {
	if value.SchemaVersion != 1 || value.SearchVersion != 1 || len(value.Records) == 0 {
		return fmt.Errorf("catalogjson: search record segment is invalid")
	}
	for index, record := range value.Records {
		if err := validateDetailID(record.DetailID); err != nil {
			return err
		}
		if err := domain.ValidateCatalogDocumentKey(record.DocumentKey); err != nil {
			return fmt.Errorf("catalogjson: %w", err)
		}
		if index > 0 && value.Records[index-1].DetailID >= record.DetailID {
			return fmt.Errorf("catalogjson: search records are not strictly sorted")
		}
		if record.Title == "" || record.Href == "" || record.Occurrences == 0 || record.Occurrences != uint32(len(record.Documents)) {
			return fmt.Errorf("catalogjson: search record %q metadata is invalid", record.DetailID)
		}
		for documentIndex, documentKey := range record.Documents {
			if err := domain.ValidateCatalogDocumentKey(documentKey); err != nil {
				return fmt.Errorf("catalogjson: %w", err)
			}
			if documentIndex > 0 && record.Documents[documentIndex-1] >= documentKey {
				return fmt.Errorf("catalogjson: search record documents are not strictly sorted")
			}
		}
		switch record.Kind {
		case "operation":
			if record.Method == "" || record.Path == "" || record.SchemaName != "" {
				return fmt.Errorf("catalogjson: operation search record %q is malformed", record.DetailID)
			}
		case "schema":
			if record.SchemaName == "" || record.Method != "" || record.Path != "" || record.OperationID != "" {
				return fmt.Errorf("catalogjson: schema search record %q is malformed", record.DetailID)
			}
		default:
			return fmt.Errorf("catalogjson: search record kind %q is unsupported", record.Kind)
		}
	}
	return nil
}

func validateSearchRoutes(routes []catalog.SearchPostingRouteV1, segments []catalog.SearchSegmentReferenceV1) error {
	for index, route := range routes {
		if err := validateSearchKey(route.Key); err != nil {
			return err
		}
		if index > 0 && routes[index-1].Key >= route.Key {
			return fmt.Errorf("catalogjson: search routes are not strictly sorted")
		}
		if int(route.Segment) >= len(segments) {
			return fmt.Errorf("catalogjson: search route segment is invalid")
		}
	}
	return nil
}

func validateSearchSegmentReference(value catalog.SearchSegmentReferenceV1) error {
	if err := validateChildPath(value.Path); err != nil {
		return err
	}
	if value.Entries == 0 || value.Postings == 0 || value.Length == 0 || value.Length > maxSearchSegmentBytes || !lowerHex(value.SHA256, 64) {
		return fmt.Errorf("catalogjson: search segment reference %q is invalid", value.Path)
	}
	return nil
}

func validateSearchKey(value string) error {
	if value == "" || strings.TrimSpace(value) != value {
		return fmt.Errorf("catalogjson: search key is invalid")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("catalogjson: search key is invalid")
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
	if err := require(directory.SearchChild); err != nil {
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

func ValidateSearchManifest(directory catalog.SearchDirectoryV1, manifest catalog.ManifestV1) error {
	if err := validateSearchDirectory(directory); err != nil {
		return err
	}
	children := make(map[string]catalog.ChildIdentityV1, len(manifest.Children))
	for _, child := range manifest.Children {
		children[child.Path] = child
	}
	require := func(pathValue string, length uint64, digest string) error {
		child, exists := children[pathValue]
		if !exists || child.Length != length || child.SHA256 != digest {
			return fmt.Errorf("catalogjson: search child %q is undeclared or changed", pathValue)
		}
		return nil
	}
	for _, bucket := range directory.ExactBuckets {
		if err := require(bucket.Path, bucket.Length, bucket.SHA256); err != nil {
			return err
		}
	}
	for _, reference := range directory.PostingSegments {
		if err := require(reference.Path, reference.Length, reference.SHA256); err != nil {
			return err
		}
	}
	for _, reference := range directory.TrigramSegments {
		if err := require(reference.Path, reference.Length, reference.SHA256); err != nil {
			return err
		}
	}
	for _, reference := range directory.RecordSegments {
		if err := require(reference.Path, reference.Length, reference.SHA256); err != nil {
			return err
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
