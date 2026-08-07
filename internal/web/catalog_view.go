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

	"github.com/araihu/goshtoso/components/icon/heroicons"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
	"github.com/araihu/manja/internal/web/templates"
)

var errCatalogPageNotFound = errors.New("catalog page not found")

const catalogSidebarPageSize = 100

type catalogSidebarQuery struct {
	groups   []string
	explicit bool
	pages    []string
}

func (handler *CatalogHandler) catalogPageData(
	ctx context.Context,
	snapshot catalog.RuntimeSnapshot,
	mount, documentKey, selectedID, expandedGroup, selectedNode, groupPage string,
) (templates.CatalogPageData, error) {
	sidebarQuery := catalogSidebarQuery{}
	if expandedGroup != "" {
		sidebarQuery.explicit = true
		sidebarQuery.groups = []string{expandedGroup}
	}
	if groupPage != "" {
		sidebarQuery.pages = []string{groupPage}
	}
	return handler.catalogPageDataWithSidebarQuery(ctx, snapshot, mount, documentKey, selectedID, selectedNode, sidebarQuery)
}

func (handler *CatalogHandler) catalogPageDataWithSidebarQuery(
	ctx context.Context,
	snapshot catalog.RuntimeSnapshot,
	mount, documentKey, selectedID, selectedNode string,
	sidebarQuery catalogSidebarQuery,
) (templates.CatalogPageData, error) {
	data := templates.CatalogPageData{
		Mount: mount, SnapshotID: snapshot.ID,
		RevisionID: snapshot.Manifest.Identity.RevisionID, CommitSHA: snapshot.Manifest.Identity.CommitSHA,
		Directory: snapshot.Directory,
		// A one-document mount is the renderer's standalone-spec shape. Its
		// sidebar should return to the organization landing page, not imply a
		// catalog overview that does not exist.
		StandaloneSpec: len(snapshot.Directory.Documents) == 1,
		Documents:      make([]templates.CatalogDocumentOption, 0, len(snapshot.Directory.Documents)),
	}
	presentation := handler.presentation[mount]
	data.CatalogReadme = strings.TrimSpace(presentation.Readme)
	data.CatalogLicense = templates.CatalogOrganizationLicenseData{
		Name: strings.TrimSpace(presentation.License.Name),
		URL:  strings.TrimSpace(presentation.License.URL),
	}
	data.SearchScopeLabel = strings.TrimSpace(snapshot.Directory.Title)
	// The organization navigation is reserved for the synthetic Manja root.
	// Catalog pages keep their sidebar focused on the selected spec's API
	// sections so a large catalog never duplicates its entire directory in the
	// browser.
	data.OrganizationNav = handler.catalogOrganizationNav(mount, false)
	data.SearchMount = mount
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
		data.Documents = append(data.Documents, templates.CatalogDocumentOption{
			Key: document.Key, Label: catalogDocumentLabel(document), Version: document.APIVersion,
			Operations: len(document.Operations), Schemas: len(document.Schemas),
			SearchText: strings.ToLower(document.Key + " " + document.APIVersion),
			Href:       href + "/", AvatarSrc: document.Branding.LogoSrc, AvatarAlt: document.Branding.LogoAlt,
			Selected: document.Key == documentKey,
		})
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
	data.DocumentHref = documentHref
	data.SchemaLinks = make(map[string]string, len(document.Schemas))
	for _, schema := range document.Schemas {
		name := strings.TrimSpace(schema.Name)
		if name == "" {
			continue
		}
		if _, exists := data.SchemaLinks[name]; !exists {
			data.SchemaLinks[name] = catalogDetailHref(documentHref, schema.DetailID)
		}
	}
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
	validGroupIDs := make(map[string]struct{}, len(operationGroups)+1)
	for _, grouped := range operationGroups {
		validGroupIDs[catalogGroupID("operations-"+grouped.label)] = struct{}{}
	}
	if len(document.Schemas) > 0 {
		validGroupIDs[catalogGroupID("schemas")] = struct{}{}
	}
	if len(document.SecuritySchemes) > 0 {
		validGroupIDs[catalogGroupID("security-schemes")] = struct{}{}
	}
	openGroups := make(map[string]struct{}, len(sidebarQuery.groups)+1)
	if sidebarQuery.explicit {
		for _, groupID := range sidebarQuery.groups {
			if _, valid := validGroupIDs[groupID]; valid {
				openGroups[groupID] = struct{}{}
			}
		}
	} else {
		// The initial document view should expose its navigation immediately.
		// An explicit group query remains authoritative so a user can collapse
		// any section without forcing all of its siblings open again.
		for groupID := range validGroupIDs {
			openGroups[groupID] = struct{}{}
		}
		if selectedGroup != "" {
			openGroups[selectedGroup] = struct{}{}
		}
	}
	// Render every item in an expanded API section. The largest current
	// Kubernetes section has 248 operations, and keeping that section as one
	// continuous list is easier to scan than introducing sidebar pagination.
	// Keep the legacy page helpers below for compatibility with older tests and
	// incoming URLs, but do not emit or apply page state here.
	groupPages := map[string]int{}
	for _, grouped := range operationGroups {
		groupID := catalogGroupID("operations-" + grouped.label)
		_, groupOpen := openGroups[groupID]
		group := templates.CatalogSidebarGroupData{
			ID: groupID, Kind: "operations", Label: grouped.label, Count: len(grouped.operations), Open: groupOpen,
			Href: catalogSidebarToggleHref(documentHref, selectedID, selectedNode, openGroups, groupPages, groupID),
		}
		if group.Open {
			group.Items = make([]templates.CatalogSidebarItemData, 0, len(grouped.operations))
			for _, operation := range grouped.operations {
				href := catalogDetailHref(documentHref, operation.DetailID)
				group.Items = append(group.Items, templates.CatalogSidebarItemData{
					ID: "sidebar-" + string(operation.DetailID), Label: operation.Title, Href: href,
					Method: operation.Method, Active: operation.DetailID == selectedDetailID,
				})
			}
		}
		data.Groups = append(data.Groups, group)
	}
	if len(document.Schemas) > 0 {
		groupID := catalogGroupID("schemas")
		_, groupOpen := openGroups[groupID]
		group := templates.CatalogSidebarGroupData{
			ID: groupID, Kind: "schemas", Label: "Schemas", Count: len(document.Schemas), Open: groupOpen,
			Href: catalogSidebarToggleHref(documentHref, selectedID, selectedNode, openGroups, groupPages, groupID),
		}
		if group.Open {
			group.Items = make([]templates.CatalogSidebarItemData, 0, len(document.Schemas))
			for _, schema := range document.Schemas {
				group.Items = append(group.Items, templates.CatalogSidebarItemData{
					ID: "sidebar-" + string(schema.DetailID), Label: schema.Name,
					Href: catalogDetailHref(documentHref, schema.DetailID), Active: schema.DetailID == selectedDetailID,
				})
			}
		}
		data.Groups = append(data.Groups, group)
	}
	if len(document.SecuritySchemes) > 0 {
		groupID := catalogGroupID("security-schemes")
		_, groupOpen := openGroups[groupID]
		group := templates.CatalogSidebarGroupData{
			ID: groupID, Kind: "security-schemes", Label: "Security Schemes", Count: len(document.SecuritySchemes), Open: groupOpen,
			Href: catalogSidebarToggleHref(documentHref, selectedID, selectedNode, openGroups, groupPages, groupID),
		}
		if group.Open {
			for _, scheme := range document.SecuritySchemes {
				group.Items = append(group.Items, templates.CatalogSidebarItemData{
					ID: "sidebar-" + scheme.Anchor, Label: scheme.Name, Href: documentHref + "#" + scheme.Anchor,
				})
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
			data.OperationNavigation = catalogOperationNavigation(documentHref, document.Operations, detail.ID, openGroups, groupPages)
			data.CurrentVisit = &templates.CatalogSearchItemData{
				ID: string(detail.ID), Title: detail.Operation.Heading, Description: detail.Operation.Description,
				Href: catalogDetailHref(documentHref, detail.ID), Kind: "Operation", Method: detail.Operation.Method,
				Path: detail.Operation.Path, Section: document.Key,
			}
		} else if detail.Schema != nil {
			schema, err := handler.catalogSchemaView(ctx, snapshot, document, *detail.Schema)
			if err != nil {
				return templates.CatalogPageData{}, err
			}
			data.SchemaView = schema
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
	if handler == nil || handler.runtime == nil || !rootView {
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
			AvatarSymbol: string(heroicons.Icon16SolidCube),
			Active:       mount == activeMount,
		})
		for _, document := range directory.Documents {
			documentHref, err := catalogURL(mount, "documents", document.Key)
			if err != nil {
				continue
			}
			data.Specs = append(data.Specs, templates.CatalogOrganizationItem{
				ID:    "spec-" + string(directory.CatalogID) + "-" + document.Key,
				Label: catalogDocumentLabel(document), Description: catalogLabel,
				Href: documentHref + "/",
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

func catalogSidebarPages(rawPages []string, openGroups map[string]struct{}) (map[string]int, error) {
	pages := make(map[string]int, len(rawPages))
	for _, raw := range rawPages {
		if raw == "" {
			continue
		}
		groupID, pageValue, scoped := strings.Cut(raw, ":")
		if !scoped {
			pageValue = raw
			if len(openGroups) > 1 {
				return nil, fmt.Errorf("catalog sidebar page %q is ambiguous", raw)
			}
			for openGroupID := range openGroups {
				groupID = openGroupID
			}
		} else if groupID == "" {
			return nil, fmt.Errorf("catalog sidebar page %q has no group", raw)
		}
		page, err := strconv.Atoi(pageValue)
		if err != nil || page < 1 {
			return nil, fmt.Errorf("catalog sidebar page %q is invalid", raw)
		}
		if _, open := openGroups[groupID]; open {
			catalogSetSidebarPage(pages, groupID, page)
		}
	}
	return pages, nil
}

func catalogSetSidebarPage(pages map[string]int, groupID string, page int) {
	if page <= 1 {
		delete(pages, groupID)
		return
	}
	pages[groupID] = page
}

func catalogSidebarToggleHref(
	documentHref, selectedID, selectedNode string,
	openGroups map[string]struct{}, pages map[string]int,
	groupID string,
) string {
	toggledGroups := cloneCatalogOpenGroups(openGroups)
	toggledPages := cloneCatalogSidebarPages(pages)
	if _, open := toggledGroups[groupID]; open {
		delete(toggledGroups, groupID)
		delete(toggledPages, groupID)
	} else {
		toggledGroups[groupID] = struct{}{}
	}
	return catalogSidebarHref(documentHref, selectedID, selectedNode, toggledGroups, toggledPages)
}

func catalogGroupPageHref(
	documentHref, selectedID, selectedNode string,
	openGroups map[string]struct{}, pages map[string]int,
	groupID string, page int,
) string {
	nextPages := cloneCatalogSidebarPages(pages)
	catalogSetSidebarPage(nextPages, groupID, page)
	return catalogSidebarHref(documentHref, selectedID, selectedNode, openGroups, nextPages)
}

func catalogSidebarHref(
	documentHref, selectedID, selectedNode string,
	openGroups map[string]struct{}, pages map[string]int,
) string {
	query := url.Values{}
	groupIDs := make([]string, 0, len(openGroups))
	for groupID := range openGroups {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)
	if len(groupIDs) == 0 {
		query.Add("group", "")
	} else {
		for _, groupID := range groupIDs {
			query.Add("group", groupID)
		}
	}
	pageGroupIDs := make([]string, 0, len(pages))
	for groupID, page := range pages {
		if page > 1 {
			if _, open := openGroups[groupID]; open {
				pageGroupIDs = append(pageGroupIDs, groupID)
			}
		}
	}
	sort.Strings(pageGroupIDs)
	for _, groupID := range pageGroupIDs {
		query.Add("page", groupID+":"+strconv.Itoa(pages[groupID]))
	}
	if selectedNode != "" {
		query.Set("node", selectedNode)
	}
	if selectedID != "" {
		query.Set("selected", selectedID)
	}
	href := documentHref + "?" + query.Encode()
	if selectedNode != "" {
		return href + "#schema-node-panel"
	}
	if selectedID != "" {
		return href + "#" + url.PathEscape(selectedID)
	}
	return href
}

func catalogOperationNavigation(
	documentHref string,
	operations []catalog.OperationDirectoryV1,
	selectedID domain.DetailID,
	openGroups map[string]struct{},
	pages map[string]int,
) templates.OperationNavigationData {
	selectedIndex := -1
	group := ""
	for index, operation := range operations {
		if operation.DetailID == selectedID {
			selectedIndex = index
			group = catalogOperationGroupLabel(operation)
			break
		}
	}
	navigation := templates.OperationNavigationData{Group: group, Catalog: true}
	if selectedIndex < 0 {
		return navigation
	}
	for index := selectedIndex - 1; index >= 0; index-- {
		operation := operations[index]
		if catalogOperationGroupLabel(operation) != group {
			continue
		}
		navigation.Previous = catalogOperationNavigationItem(documentHref, operation, openGroups, pages)
		break
	}
	for index := selectedIndex + 1; index < len(operations); index++ {
		operation := operations[index]
		if catalogOperationGroupLabel(operation) != group {
			continue
		}
		navigation.Next = catalogOperationNavigationItem(documentHref, operation, openGroups, pages)
		break
	}
	return navigation
}

func catalogOperationNavigationItem(
	documentHref string,
	operation catalog.OperationDirectoryV1,
	openGroups map[string]struct{},
	pages map[string]int,
) *templates.OperationNavigationItem {
	title := firstNonEmpty(strings.TrimSpace(operation.Title), strings.TrimSpace(operation.OperationID), strings.TrimSpace(operation.Method+" "+operation.Path))
	return &templates.OperationNavigationItem{
		Title:  title,
		Method: operation.Method,
		Href:   catalogSidebarHref(documentHref, string(operation.DetailID), "", openGroups, pages),
	}
}

func catalogOperationGroupLabel(operation catalog.OperationDirectoryV1) string {
	if len(operation.Tags) > 0 {
		if label := strings.TrimSpace(operation.Tags[0]); label != "" {
			return label
		}
	}
	return "Untagged"
}

func cloneCatalogOpenGroups(groups map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(groups)+1)
	for groupID := range groups {
		cloned[groupID] = struct{}{}
	}
	return cloned
}

func cloneCatalogSidebarPages(pages map[string]int) map[string]int {
	cloned := make(map[string]int, len(pages)+1)
	for groupID, page := range pages {
		cloned[groupID] = page
	}
	return cloned
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
		EnumValues: append([]string(nil), node.Enum...), Constraints: catalogSchemaConstraints(node.Constraints),
		Nullable: node.Nullable, Deprecated: node.Deprecated,
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
		defaultValue := ""
		exampleText := ""
		enumValues := []string(nil)
		constraints := []templates.CatalogSchemaConstraintData(nil)
		nullable := false
		deprecated := false
		if local {
			defaultValue = catalogSchemaText(target.DefaultValue)
			exampleText = catalogSchemaText(target.ExampleText)
			enumValues = append([]string(nil), target.Enum...)
			constraints = catalogSchemaConstraints(target.Constraints)
			nullable = target.Nullable
			deprecated = target.Deprecated
		}
		result.Edges = append(result.Edges, templates.CatalogSchemaEdgeData{
			Name: name, Description: catalogSchemaText(description), Required: required,
			Type: typeLabel, DefaultValue: defaultValue, ExampleText: exampleText,
			EnumValues: enumValues, Constraints: constraints, Nullable: nullable, Deprecated: deprecated,
			Href: catalogSchemaNodeHref(documentHref, detailID, uint32(ref)),
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

func catalogSchemaConstraints(constraints []projection.SchemaConstraint) []templates.CatalogSchemaConstraintData {
	result := make([]templates.CatalogSchemaConstraintData, 0, len(constraints))
	for _, constraint := range constraints {
		result = append(result, templates.CatalogSchemaConstraintData{Name: constraint.Name, Value: constraint.Value})
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
