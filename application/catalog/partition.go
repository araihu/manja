package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

type PartitionLimits struct {
	DetailRecords     uint64
	DetailBytes       uint64
	SchemaNodeRecords uint64
	SchemaNodeBytes   uint64
}

func DefaultPartitionLimits(bounds Bounds) PartitionLimits {
	return PartitionLimits{
		DetailRecords: bounds.DetailShardRecords, DetailBytes: bounds.DetailShardBytes,
		SchemaNodeRecords: bounds.SchemaNodeShardRecords, SchemaNodeBytes: bounds.SchemaNodeShardBytes,
	}
}

func PartitionDocument(documentKey string, document projection.Document, directory DocumentDirectoryV1, limits PartitionLimits) (DocumentArtifacts, error) {
	if err := domain.ValidateCatalogDocumentKey(documentKey); err != nil {
		return DocumentArtifacts{}, err
	}
	if limits.DetailRecords == 0 || limits.DetailBytes == 0 || limits.SchemaNodeRecords == 0 || limits.SchemaNodeBytes == 0 {
		return DocumentArtifacts{}, fmt.Errorf("partition limits must be nonzero")
	}
	records, err := catalogDetailRecords(document, directory)
	if err != nil {
		return DocumentArtifacts{}, err
	}
	detailChildren, detailPathByID, usage, err := partitionDetailRecords(documentKey, records, limits)
	if err != nil {
		return DocumentArtifacts{}, err
	}
	for index := range directory.Operations {
		childPath, exists := detailPathByID[directory.Operations[index].DetailID]
		if !exists {
			return DocumentArtifacts{}, fmt.Errorf("operation detail %q has no shard", directory.Operations[index].DetailID)
		}
		directory.Operations[index].DetailChild = childPath
	}
	for index := range directory.Schemas {
		childPath, exists := detailPathByID[directory.Schemas[index].DetailID]
		if !exists {
			return DocumentArtifacts{}, fmt.Errorf("schema detail %q has no shard", directory.Schemas[index].DetailID)
		}
		directory.Schemas[index].DetailChild = childPath
	}
	nodeChildren, nodeReferences, nodeUsage, err := partitionSchemaNodes(documentKey, document.SchemaNodes, limits)
	if err != nil {
		return DocumentArtifacts{}, err
	}
	directory.SchemaNodeShards = nodeReferences
	children := append(detailChildren, nodeChildren...)
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	usage.ProjectionBytes += nodeUsage.ProjectionBytes
	usage.Children += nodeUsage.Children
	usage.SchemaNodeShardRecords = nodeUsage.SchemaNodeShardRecords
	usage.SchemaNodeShardBytes = nodeUsage.SchemaNodeShardBytes
	return DocumentArtifacts{Directory: directory, Children: children, Usage: usage}, nil
}

func catalogDetailRecords(document projection.Document, directory DocumentDirectoryV1) ([]DetailRecordV1, error) {
	operationByKey := make(map[string]OperationDirectoryV1, len(directory.Operations))
	for _, operation := range directory.Operations {
		key := operation.Method + "\x00" + operation.Path
		if _, duplicate := operationByKey[key]; duplicate {
			return nil, fmt.Errorf("operation directory key %q is duplicated", key)
		}
		operationByKey[key] = operation
	}
	schemaByName := make(map[string]SchemaDirectoryV1, len(directory.Schemas))
	for _, schema := range directory.Schemas {
		if _, duplicate := schemaByName[schema.Name]; duplicate {
			return nil, fmt.Errorf("schema directory name %q is duplicated", schema.Name)
		}
		schemaByName[schema.Name] = schema
	}
	records := make([]DetailRecordV1, 0, len(document.OperationDetails)+len(document.SchemaDetails))
	seen := make(map[domain.DetailID]struct{}, cap(records))
	for _, detail := range document.OperationDetails {
		directoryRecord, exists := operationByKey[detail.Method+"\x00"+detail.Path]
		if !exists {
			return nil, fmt.Errorf("projection operation %s %s has no catalog directory record", detail.Method, detail.Path)
		}
		if _, duplicate := seen[directoryRecord.DetailID]; duplicate {
			return nil, fmt.Errorf("detail ID %q is duplicated", directoryRecord.DetailID)
		}
		seen[directoryRecord.DetailID] = struct{}{}
		detail.ID = string(directoryRecord.DetailID)
		detail.Anchor = string(directoryRecord.DetailID)
		detail.Href = directoryRecord.Href
		detail.HeadingID = string(directoryRecord.DetailID)
		copied := detail
		records = append(records, DetailRecordV1{ID: directoryRecord.DetailID, Kind: "operation", Operation: &copied})
	}
	for _, detail := range document.SchemaDetails {
		directoryRecord, exists := schemaByName[detail.Heading]
		if !exists {
			return nil, fmt.Errorf("projection schema %q has no catalog directory record", detail.Heading)
		}
		if _, duplicate := seen[directoryRecord.DetailID]; duplicate {
			return nil, fmt.Errorf("detail ID %q is duplicated", directoryRecord.DetailID)
		}
		seen[directoryRecord.DetailID] = struct{}{}
		detail.ID = string(directoryRecord.DetailID)
		detail.Anchor = string(directoryRecord.DetailID)
		detail.Href = directoryRecord.Href
		detail.HeadingID = string(directoryRecord.DetailID)
		copied := detail
		records = append(records, DetailRecordV1{ID: directoryRecord.DetailID, Kind: "schema", Schema: &copied})
	}
	if len(records) != len(directory.Operations)+len(directory.Schemas) {
		return nil, fmt.Errorf("projection detail coverage is incomplete")
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records, nil
}

func partitionDetailRecords(documentKey string, records []DetailRecordV1, limits PartitionLimits) ([]ChildArtifact, map[domain.DetailID]string, BudgetUsage, error) {
	children := make([]ChildArtifact, 0)
	pathByID := make(map[domain.DetailID]string, len(records))
	usage := BudgetUsage{}
	for start := 0; start < len(records); {
		end := start
		var encoded []byte
		for end < len(records) && uint64(end-start) < limits.DetailRecords {
			candidate, err := json.Marshal(DetailShardV1{SchemaVersion: 1, DocumentKey: documentKey, Records: records[start : end+1]})
			if err != nil {
				return nil, nil, BudgetUsage{}, fmt.Errorf("encode detail shard: %w", err)
			}
			if uint64(len(candidate)) > limits.DetailBytes {
				if end == start {
					return nil, nil, BudgetUsage{}, fmt.Errorf("detail record %q exceeds %d bytes", records[start].ID, limits.DetailBytes)
				}
				break
			}
			encoded = candidate
			end++
		}
		if end == start || len(encoded) == 0 {
			return nil, nil, BudgetUsage{}, fmt.Errorf("detail partition made no progress")
		}
		child, err := contentAddressedChild("details/"+documentKey, "detail", encoded)
		if err != nil {
			return nil, nil, BudgetUsage{}, err
		}
		children = append(children, child)
		for _, record := range records[start:end] {
			pathByID[record.ID] = child.Path
		}
		usage.Children++
		usage.ProjectionBytes += child.Length
		if uint64(end-start) > usage.DetailShardRecords {
			usage.DetailShardRecords = uint64(end - start)
		}
		if child.Length > usage.DetailShardBytes {
			usage.DetailShardBytes = child.Length
		}
		start = end
	}
	return children, pathByID, usage, nil
}

func partitionSchemaNodes(documentKey string, nodes []projection.SchemaNode, limits PartitionLimits) ([]ChildArtifact, []ShardReferenceV1, BudgetUsage, error) {
	children := make([]ChildArtifact, 0)
	references := make([]ShardReferenceV1, 0)
	usage := BudgetUsage{}
	for index, node := range nodes {
		if node.Ordinal != uint32(index) {
			return nil, nil, BudgetUsage{}, fmt.Errorf("schema node ordinal %d is not contiguous at %d", node.Ordinal, index)
		}
	}
	for start := 0; start < len(nodes); {
		end := start
		var encoded []byte
		for end < len(nodes) && uint64(end-start) < limits.SchemaNodeRecords {
			candidate, err := json.Marshal(SchemaNodeShardV1{
				SchemaVersion: 1, DocumentKey: documentKey, FirstOrdinal: uint32(start), Nodes: nodes[start : end+1],
			})
			if err != nil {
				return nil, nil, BudgetUsage{}, fmt.Errorf("encode schema-node shard: %w", err)
			}
			if uint64(len(candidate)) > limits.SchemaNodeBytes {
				if end == start {
					return nil, nil, BudgetUsage{}, fmt.Errorf("schema node %d exceeds %d bytes", start, limits.SchemaNodeBytes)
				}
				break
			}
			encoded = candidate
			end++
		}
		if end == start || len(encoded) == 0 {
			return nil, nil, BudgetUsage{}, fmt.Errorf("schema-node partition made no progress")
		}
		child, err := contentAddressedChild("schema-nodes/"+documentKey, "schema-node", encoded)
		if err != nil {
			return nil, nil, BudgetUsage{}, err
		}
		children = append(children, child)
		references = append(references, ShardReferenceV1{
			Path: child.Path, FirstOrdinal: uint32(start), LastOrdinal: uint32(end - 1),
			Records: uint32(end - start), Length: child.Length, SHA256: child.SHA256,
		})
		usage.Children++
		usage.ProjectionBytes += child.Length
		if uint64(end-start) > usage.SchemaNodeShardRecords {
			usage.SchemaNodeShardRecords = uint64(end - start)
		}
		if child.Length > usage.SchemaNodeShardBytes {
			usage.SchemaNodeShardBytes = child.Length
		}
		start = end
	}
	return children, references, usage, nil
}

func contentAddressedChild(prefix, kind string, data []byte) (ChildArtifact, error) {
	digest := sha256.Sum256(data)
	return newChild(prefix+"/"+hex.EncodeToString(digest[:])+".json", kind, data)
}
