package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
	"github.com/araihu/manja/internal/web/templates"
)

var errCatalogPageNotFound = errors.New("catalog page not found")

const catalogSidebarPageSize = 100

func (handler *CatalogHandler) catalogPageData(
	ctx context.Context,
	snapshot catalog.RuntimeSnapshot,
	mount, documentKey, selectedID, expandedGroup, selectedNode, groupPage string,
) (templates.CatalogPageData, error) {
	data := templates.CatalogPageData{
		Mount: mount, SnapshotID: snapshot.ID, Directory: snapshot.Directory,
		Documents: make([]templates.CatalogDocumentOption, 0, len(snapshot.Directory.Documents)),
	}
	data.OrganizationNav = handler.catalogOrganizationNav(mount, documentKey == "")
	data.SearchHref, _ = catalogURL(mount, "search")
	data.DownloadHref, _ = catalogURL(mount, "catalog.json")
	searchIdentity, exists := catalogChildIdentity(snapshot.Manifest, snapshot.Directory.SearchChild)
	if !exists || searchIdentity.Kind != "search-directory" {
		return templates.CatalogPageData{}, fmt.Errorf("catalog search directory identity is missing")
	}
	data.SearchDirectoryPath = searchIdentity.Path
	data.SearchDirectoryLength = searchIdentity.Length
	data.SearchDirectorySHA256 = searchIdentity.SHA256
	data.SearchChildBase, _ = catalogURL(mount, "snapshots", string(snapshot.ID), "search-data")
	data.SearchChildBase += "/"
	for _, document := range snapshot.Directory.Documents {
		href, err := catalogURL(mount, "documents", document.Key)
		if err != nil {
			return templates.CatalogPageData{}, err
		}
		data.Documents = append(data.Documents, templates.CatalogDocumentOption{Key: document.Key, Label: catalogDocumentLabel(document), Href: href + "/", Selected: document.Key == documentKey})
	}
	if documentKey == "" {
		return data, nil
	}
	document, exists := catalogDocument(snapshot.Directory, documentKey)
	if !exists {
		return templates.CatalogPageData{}, errCatalogPageNotFound
	}
	data.Document = &document
	data.OperationServers = catalogOperationServers(document.Overview.Servers)
	extension := path.Ext(document.SourceChild)
	data.DownloadHref, _ = catalogURL(mount, "openapi", document.Key+extension)
	documentHref, _ := catalogURL(mount, "documents", document.Key)
	documentHref += "/"
	data.CurrentVisit = &templates.CatalogSearchItemData{
		ID: "document-" + document.Key, Title: document.Key, Description: document.Title,
		Href: documentHref, Kind: "Document", Section: snapshot.Directory.Title,
	}

	type operationGroup struct {
		label      string
		operations []catalog.OperationDirectoryV1
	}
	groupsByLabel := make(map[string][]catalog.OperationDirectoryV1)
	selectedGroup := ""
	selectedChild := ""
	selectedDetailID := domain.DetailID(selectedID)
	for _, operation := range document.Operations {
		label := "Untagged"
		if len(operation.Tags) > 0 && strings.TrimSpace(operation.Tags[0]) != "" {
			label = operation.Tags[0]
		}
		groupsByLabel[label] = append(groupsByLabel[label], operation)
		if operation.DetailID == selectedDetailID {
			selectedGroup = catalogGroupID("operations-" + label)
			selectedChild = operation.DetailChild
		}
	}
	operationGroups := make([]operationGroup, 0, len(groupsByLabel))
	for label, operations := range groupsByLabel {
		operationGroups = append(operationGroups, operationGroup{label: label, operations: operations})
	}
	sort.Slice(operationGroups, func(left, right int) bool { return operationGroups[left].label < operationGroups[right].label })
	if selectedID != "" && selectedChild == "" {
		for _, schema := range document.Schemas {
			if schema.DetailID == selectedDetailID {
				selectedGroup = catalogGroupID("schemas")
				selectedChild = schema.DetailChild
				break
			}
		}
		if selectedChild == "" {
			return templates.CatalogPageData{}, errCatalogPageNotFound
		}
	}
	if expandedGroup == "" {
		expandedGroup = selectedGroup
	}
	requestedPage := 1
	if groupPage != "" {
		parsed, err := strconv.Atoi(groupPage)
		if err != nil || parsed < 1 {
			return templates.CatalogPageData{}, errCatalogPageNotFound
		}
		requestedPage = parsed
	}
	for _, grouped := range operationGroups {
		groupID := catalogGroupID("operations-" + grouped.label)
		group := templates.CatalogSidebarGroupData{
			ID: groupID, Label: grouped.label, Count: len(grouped.operations), Open: groupID == expandedGroup,
			Href: documentHref + "?group=" + url.QueryEscape(groupID),
		}
		if group.Open {
			page := requestedPage
			if selectedGroup == groupID && selectedDetailID != "" {
				for index, operation := range grouped.operations {
					if operation.DetailID == selectedDetailID {
						page = index/catalogSidebarPageSize + 1
						break
					}
				}
			}
			start, end, ok := catalogSidebarPageWindow(len(grouped.operations), page)
			if !ok {
				return templates.CatalogPageData{}, errCatalogPageNotFound
			}
			group.Items = make([]templates.CatalogSidebarItemData, 0, end-start+2)
			if start > 0 {
				group.Items = append(group.Items, templates.CatalogSidebarItemData{ID: groupID + "-previous", Label: "Previous 100 operations", Href: catalogGroupPageHref(documentHref, groupID, page-1)})
			}
			for _, operation := range grouped.operations[start:end] {
				href := catalogDetailHref(documentHref, operation.DetailID)
				group.Items = append(group.Items, templates.CatalogSidebarItemData{
					ID: "sidebar-" + string(operation.DetailID), Label: operation.Title, Href: href,
					Method: operation.Method, Active: operation.DetailID == selectedDetailID,
				})
			}
			if end < len(grouped.operations) {
				group.Items = append(group.Items, templates.CatalogSidebarItemData{ID: groupID + "-next", Label: "Next 100 operations", Href: catalogGroupPageHref(documentHref, groupID, page+1)})
			}
		}
		data.Groups = append(data.Groups, group)
	}
	if len(document.Schemas) > 0 {
		groupID := catalogGroupID("schemas")
		group := templates.CatalogSidebarGroupData{
			ID: groupID, Label: "Schemas", Count: len(document.Schemas), Open: groupID == expandedGroup,
			Href: documentHref + "?group=" + url.QueryEscape(groupID),
		}
		if group.Open {
			page := requestedPage
			if selectedGroup == groupID && selectedDetailID != "" {
				for index, schema := range document.Schemas {
					if schema.DetailID == selectedDetailID {
						page = index/catalogSidebarPageSize + 1
						break
					}
				}
			}
			start, end, ok := catalogSidebarPageWindow(len(document.Schemas), page)
			if !ok {
				return templates.CatalogPageData{}, errCatalogPageNotFound
			}
			group.Items = make([]templates.CatalogSidebarItemData, 0, end-start+2)
			if start > 0 {
				group.Items = append(group.Items, templates.CatalogSidebarItemData{ID: groupID + "-previous", Label: "Previous 100 schemas", Href: catalogGroupPageHref(documentHref, groupID, page-1)})
			}
			for _, schema := range document.Schemas[start:end] {
				group.Items = append(group.Items, templates.CatalogSidebarItemData{
					ID: "sidebar-" + string(schema.DetailID), Label: schema.Name,
					Href: catalogDetailHref(documentHref, schema.DetailID), Active: schema.DetailID == selectedDetailID,
				})
			}
			if end < len(document.Schemas) {
				group.Items = append(group.Items, templates.CatalogSidebarItemData{ID: groupID + "-next", Label: "Next 100 schemas", Href: catalogGroupPageHref(documentHref, groupID, page+1)})
			}
		}
		data.Groups = append(data.Groups, group)
	}
	if selectedChild != "" {
		detail, err := handler.loadCatalogDetail(ctx, snapshot, selectedChild, selectedDetailID)
		if err != nil {
			return templates.CatalogPageData{}, err
		}
		data.Selected = &detail
		if detail.Operation != nil {
			operation, err := handler.catalogOperationView(ctx, snapshot, document, *detail.Operation)
			if err != nil {
				return templates.CatalogPageData{}, err
			}
			data.OperationView = operation
			data.CurrentVisit = &templates.CatalogSearchItemData{
				ID: string(detail.ID), Title: detail.Operation.Heading, Description: detail.Operation.Description,
				Href: catalogDetailHref(documentHref, detail.ID), Kind: "Operation", Method: detail.Operation.Method,
				Path: detail.Operation.Path, Section: document.Key,
			}
		} else if detail.Schema != nil {
			data.CurrentVisit = &templates.CatalogSearchItemData{
				ID: string(detail.ID), Title: detail.Schema.Heading, Description: detail.Schema.Description,
				Href: catalogDetailHref(documentHref, detail.ID), Kind: "Schema", Section: document.Key,
			}
		}
		if detail.Schema != nil {
			ordinal := uint64(detail.Schema.SchemaRef)
			if selectedNode != "" {
				ordinal, err = strconv.ParseUint(selectedNode, 10, 32)
				if err != nil {
					return templates.CatalogPageData{}, errCatalogPageNotFound
				}
			}
			node, shard, err := handler.loadCatalogSchemaNode(ctx, snapshot, document, uint32(ordinal))
			if err != nil {
				return templates.CatalogPageData{}, err
			}
			data.SchemaNode = catalogSchemaNodeData(node, shard, documentHref, selectedDetailID)
		} else if selectedNode != "" {
			return templates.CatalogPageData{}, errCatalogPageNotFound
		}
	}
	return data, nil
}

// catalogOrganizationNav builds the small root navigation from immutable
// directory metadata only. It never reads source blobs or detail shards, so a
// root view can advertise every mounted catalog/spec without loading a whole
// collection into the browser.
func (handler *CatalogHandler) catalogOrganizationNav(activeMount string, rootView bool) templates.CatalogOrganizationNavData {
	if handler == nil || handler.runtime == nil || activeMount != "/" || !rootView {
		return templates.CatalogOrganizationNavData{}
	}
	mounts := handler.runtime.MountNames()
	sort.Strings(mounts)
	data := templates.CatalogOrganizationNavData{Visible: true}
	for _, mount := range mounts {
		admission, err := handler.runtime.Admit(mount)
		if err != nil {
			continue
		}
		directory := admission.Snapshot.Directory
		href, err := catalogURL(mount)
		if err != nil {
			admission.Release()
			continue
		}
		if href != "/" {
			href += "/"
		}
		catalogLabel := strings.TrimSpace(directory.Branding.DisplayName)
		if catalogLabel == "" {
			catalogLabel = strings.TrimSpace(directory.Title)
		}
		if catalogLabel == "" {
			catalogLabel = mount
		}
		data.Catalogs = append(data.Catalogs, templates.CatalogOrganizationItem{
			ID: "catalog-" + string(directory.CatalogID), Label: catalogLabel,
			Description: fmt.Sprintf("%d specs", len(directory.Documents)), Href: href,
			AvatarSrc: directory.Branding.LogoSrc, AvatarAlt: directory.Branding.LogoAlt,
			Active: mount == activeMount,
		})
		for _, document := range directory.Documents {
			documentHref, err := catalogURL(mount, "documents", document.Key)
			if err != nil {
				continue
			}
			data.Specs = append(data.Specs, templates.CatalogOrganizationItem{
				ID: "spec-" + string(directory.CatalogID) + "-" + document.Key,
				Label: catalogDocumentLabel(document), Description: catalogLabel,
				Href: documentHref + "/", AvatarSrc: directory.Branding.LogoSrc,
				AvatarAlt: directory.Branding.LogoAlt,
			})
		}
		admission.Release()
	}
	return data
}

func catalogSidebarPageWindow(total, page int) (int, int, bool) {
	if total <= 0 || page < 1 {
		return 0, 0, false
	}
	maxPage := (total-1)/catalogSidebarPageSize + 1
	if page > maxPage {
		return 0, 0, false
	}
	start := (page - 1) * catalogSidebarPageSize
	end := total
	if total-start > catalogSidebarPageSize {
		end = start + catalogSidebarPageSize
	}
	return start, end, true
}

func catalogGroupPageHref(documentHref, groupID string, page int) string {
	return documentHref + "?group=" + url.QueryEscape(groupID) + "&page=" + strconv.Itoa(page)
}

func catalogOperationServers(servers []projection.Server) []domain.SpecServer {
	result := make([]domain.SpecServer, 0, len(servers))
	for _, server := range servers {
		converted := domain.SpecServer{URL: server.URL, Description: server.Description, Variables: make([]domain.SpecServerVariable, 0, len(server.Variables))}
		for _, variable := range server.Variables {
			converted.Variables = append(converted.Variables, domain.SpecServerVariable{
				Name: variable.Name, Default: variable.Default, Description: variable.Description, Enum: textRecordValues(variable.Enum),
			})
		}
		result = append(result, converted)
	}
	return result
}

func catalogDocumentLabel(document catalog.DocumentDirectoryV1) string {
	return document.Key
}

func (handler *CatalogHandler) loadCatalogSchemaNode(
	ctx context.Context,
	snapshot catalog.RuntimeSnapshot,
	document catalog.DocumentDirectoryV1,
	ordinal uint32,
) (projection.SchemaNode, catalog.SchemaNodeShardV1, error) {
	index := sort.Search(len(document.SchemaNodeShards), func(index int) bool {
		return document.SchemaNodeShards[index].LastOrdinal >= ordinal
	})
	if index == len(document.SchemaNodeShards) || ordinal < document.SchemaNodeShards[index].FirstOrdinal {
		return projection.SchemaNode{}, catalog.SchemaNodeShardV1{}, errCatalogPageNotFound
	}
	reference := document.SchemaNodeShards[index]
	identity, exists := catalogChildIdentity(snapshot.Manifest, reference.Path)
	if !exists || identity.Kind != "schema-node" || identity.Length != reference.Length || identity.SHA256 != reference.SHA256 {
		return projection.SchemaNode{}, catalog.SchemaNodeShardV1{}, fmt.Errorf("catalog schema-node child %q metadata differs", reference.Path)
	}
	digestBytes, err := hex.DecodeString(reference.SHA256)
	if err != nil || len(digestBytes) != sha256.Size {
		return projection.SchemaNode{}, catalog.SchemaNodeShardV1{}, fmt.Errorf("catalog schema-node child %q digest is invalid", reference.Path)
	}
	var digest [sha256.Size]byte
	copy(digest[:], digestBytes)
	maximumDecodedWeight, err := catalog.DecodedWeightV1(reference.Length*2, uint64(reference.Records)*2*256, uint64(reference.Records)*256)
	if err != nil {
		return projection.SchemaNode{}, catalog.SchemaNodeShardV1{}, err
	}
	value, err := handler.details.Load(ctx, catalog.CacheKey{SnapshotID: snapshot.ID, Digest: digest}, reference.Length, maximumDecodedWeight,
		func(loadContext context.Context) ([]byte, error) {
			data, loadedIdentity, err := handler.children.ReadChild(loadContext, snapshot, reference.Path)
			if err != nil {
				return nil, err
			}
			if loadedIdentity != identity {
				return nil, fmt.Errorf("catalog schema-node child %q identity changed", reference.Path)
			}
			return data, nil
		},
		func(data []byte) (any, uint64, error) {
			shard, err := catalogjson.DecodeSchemaNodeShard(data)
			if err != nil {
				return nil, 0, err
			}
			weight, err := catalog.DecodedWeightV1(uint64(len(data))*2, uint64(cap(shard.Nodes))*256, uint64(len(shard.Nodes))*256)
			return shard, weight, err
		},
	)
	if err != nil {
		return projection.SchemaNode{}, catalog.SchemaNodeShardV1{}, err
	}
	shard, ok := value.(catalog.SchemaNodeShardV1)
	if !ok || shard.DocumentKey != document.Key || shard.FirstOrdinal != reference.FirstOrdinal || len(shard.Nodes) != int(reference.Records) {
		return projection.SchemaNode{}, catalog.SchemaNodeShardV1{}, fmt.Errorf("catalog schema-node shard %q is invalid", reference.Path)
	}
	offset := uint64(ordinal) - uint64(shard.FirstOrdinal)
	if offset >= uint64(len(shard.Nodes)) || shard.Nodes[offset].Ordinal != ordinal {
		return projection.SchemaNode{}, catalog.SchemaNodeShardV1{}, errCatalogPageNotFound
	}
	return shard.Nodes[offset], shard, nil
}

const maxCatalogSchemaEdges = 100

func catalogSchemaNodeData(node projection.SchemaNode, shard catalog.SchemaNodeShardV1, documentHref string, detailID domain.DetailID) *templates.CatalogSchemaNodeData {
	known := make(map[projection.SchemaRef]projection.SchemaNode, len(shard.Nodes))
	for _, candidate := range shard.Nodes {
		known[projection.SchemaRef(candidate.Ordinal)] = candidate
	}
	result := &templates.CatalogSchemaNodeData{
		Ordinal: node.Ordinal, Name: firstNonEmpty(node.Name, "Schema node "+strconv.FormatUint(uint64(node.Ordinal), 10)),
		Type: node.Type, Format: node.Format, Description: catalogSchemaText(node.Description),
		DefaultValue: catalogSchemaText(node.DefaultValue), ExampleText: catalogSchemaText(node.ExampleText),
	}
	appendEdge := func(name, description string, required bool, ref projection.SchemaRef) {
		if len(result.Edges) == maxCatalogSchemaEdges {
			result.Truncated = true
			return
		}
		target, local := known[ref]
		typeLabel := "schema #" + strconv.FormatUint(uint64(ref), 10)
		if local {
			typeLabel = catalogSchemaNodeType(target)
		}
		result.Edges = append(result.Edges, templates.CatalogSchemaEdgeData{
			Name: name, Description: catalogSchemaText(description), Required: required,
			Type: typeLabel, Href: catalogSchemaNodeHref(documentHref, detailID, uint32(ref)),
		})
	}
	for _, item := range node.Items {
		appendEdge("items", "", false, item.SchemaRef)
	}
	for _, property := range node.Properties {
		appendEdge(property.Name, property.Description, property.Required, property.SchemaRef)
	}
	return result
}

func catalogSchemaNodeHref(documentHref string, detailID domain.DetailID, ordinal uint32) string {
	return documentHref + "?selected=" + url.QueryEscape(string(detailID)) + "&node=" + strconv.FormatUint(uint64(ordinal), 10) + "#schema-node-panel"
}

func catalogSchemaNodeType(node projection.SchemaNode) string {
	parts := make([]string, 0, 3)
	if node.Name != "" {
		parts = append(parts, node.Name)
	}
	if node.Type != "" {
		parts = append(parts, node.Type)
	}
	if node.Format != "" {
		parts = append(parts, "("+node.Format+")")
	}
	if len(parts) == 0 {
		return "schema #" + strconv.FormatUint(uint64(node.Ordinal), 10)
	}
	return strings.Join(parts, " ")
}

func catalogSchemaText(value string) string {
	const maxRunes = 512
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (handler *CatalogHandler) loadCatalogDetail(ctx context.Context, snapshot catalog.RuntimeSnapshot, childPath string, detailID domain.DetailID) (catalog.DetailRecordV1, error) {
	identity, exists := catalogChildIdentity(snapshot.Manifest, childPath)
	if !exists {
		return catalog.DetailRecordV1{}, fmt.Errorf("catalog detail child %q is undeclared", childPath)
	}
	digestBytes, err := hex.DecodeString(identity.SHA256)
	if err != nil || len(digestBytes) != sha256.Size {
		return catalog.DetailRecordV1{}, fmt.Errorf("catalog detail child %q digest is invalid", childPath)
	}
	var digest [sha256.Size]byte
	copy(digest[:], digestBytes)
	bounds := catalog.DefaultBounds()
	maximumDecodedWeight, err := catalog.DecodedWeightV1(identity.Length*2, bounds.DetailShardRecords*2*64, bounds.DetailShardRecords*128)
	if err != nil {
		return catalog.DetailRecordV1{}, err
	}
	value, err := handler.details.Load(ctx, catalog.CacheKey{SnapshotID: snapshot.ID, Digest: digest}, identity.Length, maximumDecodedWeight,
		func(ctx context.Context) ([]byte, error) {
			data, _, err := handler.children.ReadChild(ctx, snapshot, childPath)
			return data, err
		},
		func(data []byte) (any, uint64, error) {
			shard, err := catalogjson.DecodeDetailShard(data)
			if err != nil {
				return nil, 0, err
			}
			if uint64(len(shard.Records)) > bounds.DetailShardRecords {
				return nil, 0, fmt.Errorf("catalog detail shard %q has %d records, limit %d", childPath, len(shard.Records), bounds.DetailShardRecords)
			}
			weight, err := catalog.DecodedWeightV1(uint64(len(data))*2, uint64(cap(shard.Records))*64, uint64(len(shard.Records))*128)
			return shard, weight, err
		},
	)
	if err != nil {
		return catalog.DetailRecordV1{}, err
	}
	shard, ok := value.(catalog.DetailShardV1)
	if !ok {
		return catalog.DetailRecordV1{}, fmt.Errorf("catalog detail cache type is invalid")
	}
	for _, record := range shard.Records {
		if record.ID == detailID {
			return record, nil
		}
	}
	return catalog.DetailRecordV1{}, errCatalogPageNotFound
}

func catalogDocument(directory catalog.CatalogArtifactV1, key string) (catalog.DocumentDirectoryV1, bool) {
	for _, document := range directory.Documents {
		if document.Key == key {
			return document, true
		}
	}
	return catalog.DocumentDirectoryV1{}, false
}

func catalogDocumentExists(directory catalog.CatalogArtifactV1, key string) bool {
	_, exists := catalogDocument(directory, key)
	return exists
}

func catalogChildIdentity(manifest catalog.ManifestV1, childPath string) (catalog.ChildIdentityV1, bool) {
	for _, child := range manifest.Children {
		if child.Path == childPath {
			return child, true
		}
	}
	return catalog.ChildIdentityV1{}, false
}

func catalogGroupID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "group-" + hex.EncodeToString(digest[:6])
}

func catalogDetailHref(documentHref string, detailID domain.DetailID) string {
	return documentHref + "?selected=" + url.QueryEscape(string(detailID)) + "#" + url.PathEscape(string(detailID))
}

func catalogSearchHref(mount, raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Path == "" || strings.HasPrefix(parsed.Path, "/") || strings.Contains(parsed.Path, `\`) {
		return "", fmt.Errorf("catalog search href %q is invalid", raw)
	}
	cleaned := path.Clean(parsed.Path)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("catalog search href %q escapes the mount", raw)
	}
	if strings.HasSuffix(parsed.Path, "/") {
		cleaned += "/"
	}
	if !strings.HasPrefix(cleaned, "documents/") {
		cleaned = "documents/" + cleaned
	}
	base, err := catalogURL(mount)
	if err != nil {
		return "", err
	}
	parsed.Path = strings.TrimSuffix(base, "/") + "/" + cleaned
	return parsed.String(), nil
}
