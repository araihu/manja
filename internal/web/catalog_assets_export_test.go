package web

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestCatalogAssetPathsAreSortedAndPublic(t *testing.T) {
	paths := CatalogAssetPaths()
	if len(paths) == 0 || !slices.IsSorted(paths) {
		t.Fatalf("asset paths are empty or unsorted: %v", paths)
	}
	handler := NewCatalogAssetsHandler()
	for _, assetPath := range paths {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, assetPath, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", assetPath, response.Code)
		}
	}
	copyOfPaths := CatalogAssetPaths()
	copyOfPaths[0] = "changed"
	if CatalogAssetPaths()[0] == "changed" {
		t.Fatal("CatalogAssetPaths returned shared storage")
	}
}
