//go:build !manja_runtime

package selfhosted

import (
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/renderer"
)

func TestDeploymentSearchIncludesDocumentDescription(t *testing.T) {
	directory := catalog.CatalogArtifactV1{SearchChild: "search/directory.json", Documents: []catalog.DocumentDirectoryV1{{Key: "public", Title: "PoC API"}}}
	directory.Documents[0].Overview.Description = "Contrato administrativo fictício."
	manifest := catalog.ManifestV1{Children: []catalog.ChildIdentityV1{{Path: directory.SearchChild, Kind: "search-directory", Length: 1, SHA256: strings.Repeat("a", 64)}}}
	entry, err := deploymentSearchCatalog(renderer.ActivationReceipt{CatalogID: "poc", Mount: "/poc", SnapshotID: "snapshot"}, "/", directory, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Documents) != 1 || entry.Documents[0].Description != directory.Documents[0].Overview.Description {
		t.Fatalf("search documents = %#v", entry.Documents)
	}
}
