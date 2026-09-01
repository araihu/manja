//go:build !manja_runtime

package selfhosted

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/renderer"
)

const deploymentSearchDirectoryPath = "_manja/search/directory.json"

type deploymentSearchDirectoryV1 struct {
	SchemaVersion uint32                      `json:"schemaVersion"`
	SearchVersion uint32                      `json:"searchVersion"`
	Catalogs      []deploymentSearchCatalogV1 `json:"catalogs"`
}

type deploymentSearchCatalogV1 struct {
	CatalogID       string                       `json:"catalogId"`
	Title           string                       `json:"title"`
	Mount           string                       `json:"mount"`
	ChildBase       string                       `json:"childBase"`
	DirectoryPath   string                       `json:"directoryPath"`
	DirectoryLength uint64                       `json:"directoryLength"`
	DirectorySHA256 string                       `json:"directorySha256"`
	Documents       []deploymentSearchDocumentV1 `json:"documents"`
}

type deploymentSearchDocumentV1 struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Href  string `json:"href"`
}

func deploymentSearchCatalog(active renderer.ActivationReceipt, basePath string, directory catalog.CatalogArtifactV1, manifest catalog.ManifestV1) (deploymentSearchCatalogV1, error) {
	searchChild, ok := manifestChild(manifest, directory.SearchChild)
	if !ok || searchChild.Kind != "search-directory" {
		return deploymentSearchCatalogV1{}, fmt.Errorf("catalog %q has no search directory", active.CatalogID)
	}
	mount := strings.TrimSuffix(prefixExportBase(basePath, active.Mount), "/")
	if mount == "" {
		mount = "/"
	}
	childBase := prefixExportBase(basePath, catalogRoute(active.Mount, "snapshots", active.SnapshotID, "search-data")) + "/"
	documents := make([]deploymentSearchDocumentV1, 0, len(directory.Documents))
	for _, document := range directory.Documents {
		title := strings.TrimSpace(document.Title)
		if title == "" {
			title = document.Key
		}
		documents = append(documents, deploymentSearchDocumentV1{
			Key: document.Key, Title: title,
			Href: prefixExportBase(basePath, catalogRoute(active.Mount, "documents", document.Key)) + "/",
		})
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].Key < documents[j].Key })
	title := strings.TrimSpace(directory.Title)
	if title == "" {
		title = active.CatalogID
	}
	return deploymentSearchCatalogV1{
		CatalogID: active.CatalogID, Title: title, Mount: mount, ChildBase: childBase,
		DirectoryPath: directory.SearchChild, DirectoryLength: searchChild.Length, DirectorySHA256: searchChild.SHA256,
		Documents: documents,
	}, nil
}

func encodeDeploymentSearchDirectory(catalogs []deploymentSearchCatalogV1) ([]byte, error) {
	ordered := append([]deploymentSearchCatalogV1(nil), catalogs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].CatalogID < ordered[j].CatalogID })
	value := deploymentSearchDirectoryV1{SchemaVersion: 1, SearchVersion: 1, Catalogs: ordered}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode deployment search directory: %w", err)
	}
	return append(data, '\n'), nil
}

func deploymentSearchDirectoryURL(basePath string) string {
	return prefixExportBase(basePath, "/"+path.Clean(deploymentSearchDirectoryPath))
}
