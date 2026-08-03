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
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
	"github.com/araihu/manja/internal/adapters/catalogjson"
	"github.com/araihu/manja/internal/web/templates"
)

var errCatalogPageNotFound = errors.New("catalog page not found")

func (handler *CatalogHandler) catalogPageData(
	ctx context.Context,
	snapshot catalog.RuntimeSnapshot,
	mount, documentKey, selectedID, expandedGroup string,
) (templates.CatalogPageData, error) {
	data := templates.CatalogPageData{
		Mount: mount, SnapshotID: snapshot.ID, Directory: snapshot.Directory,
		Documents: make([]templates.CatalogDocumentOption, 0, len(snapshot.Directory.Documents)),
	}
	data.SearchHref, _ = catalogURL(mount, "search")
	data.DownloadHref, _ = catalogURL(mount, "catalog.json")
	for _, document := range snapshot.Directory.Documents {
		href, err := catalogURL(mount, document.Key)
		if err != nil {
			return templates.CatalogPageData{}, err
		}
		data.Documents = append(data.Documents, templates.CatalogDocumentOption{Key: document.Key, Label: document.Title, Href: href + "/", Selected: document.Key == documentKey})
	}
	if documentKey == "" {
		return data, nil
	}
	document, exists := catalogDocument(snapshot.Directory, documentKey)
	if !exists {
		return templates.CatalogPageData{}, errCatalogPageNotFound
	}
	data.Document = &document
	extension := path.Ext(document.SourceChild)
	data.DownloadHref, _ = catalogURL(mount, "openapi", document.Key+extension)
	documentHref, _ := catalogURL(mount, document.Key)
	documentHref += "/"

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
	for _, grouped := range operationGroups {
		groupID := catalogGroupID("operations-" + grouped.label)
		group := templates.CatalogSidebarGroupData{
			ID: groupID, Label: grouped.label, Count: len(grouped.operations), Open: groupID == expandedGroup,
			Href: documentHref + "?group=" + url.QueryEscape(groupID),
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
		group := templates.CatalogSidebarGroupData{
			ID: groupID, Label: "Schemas", Count: len(document.Schemas), Open: groupID == expandedGroup,
			Href: documentHref + "?group=" + url.QueryEscape(groupID),
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
	if selectedChild != "" {
		detail, err := handler.loadCatalogDetail(ctx, snapshot, selectedChild, selectedDetailID)
		if err != nil {
			return templates.CatalogPageData{}, err
		}
		data.Selected = &detail
	}
	return data, nil
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
	value, err := handler.details.Load(ctx, catalog.CacheKey{SnapshotID: snapshot.ID, Digest: digest}, identity.Length,
		func(ctx context.Context) ([]byte, error) {
			data, _, err := handler.children.ReadChild(ctx, snapshot, childPath)
			return data, err
		},
		func(data []byte) (any, uint64, error) {
			shard, err := catalogjson.DecodeDetailShard(data)
			if err != nil {
				return nil, 0, err
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
