package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCatalogAssetsAllowOnlyCanonicalPublicAssetRequests(t *testing.T) {
	handler := NewCatalogAssetsHandler()

	tests := []struct {
		name   string
		method string
		target string
		want   int
	}{
		{name: "manja asset", method: http.MethodGet, target: "/manja-assets/manja.css", want: http.StatusOK},
		{name: "goshtoso asset", method: http.MethodGet, target: "/assets/styles.css", want: http.StatusOK},
		{name: "head asset", method: http.MethodHead, target: "/manja-assets/manja.css", want: http.StatusOK},
		{name: "unknown asset", method: http.MethodGet, target: "/manja-assets/not-embedded.js", want: http.StatusNotFound},
		{name: "unknown Goshtoso asset", method: http.MethodGet, target: "/assets/not-embedded.css", want: http.StatusNotFound},
		{name: "query", method: http.MethodGet, target: "/manja-assets/manja.css?cache=1", want: http.StatusNotFound},
		{name: "non-get", method: http.MethodPost, target: "/manja-assets/manja.css", want: http.StatusNotFound},
		{name: "absolute cross-origin", method: http.MethodGet, target: "https://attacker.example/manja-assets/manja.css", want: http.StatusNotFound},
		{name: "encoded path", method: http.MethodGet, target: "/manja-assets/manja%2ecss", want: http.StatusNotFound},
		{name: "dot segment", method: http.MethodGet, target: "/manja-assets/./manja.css", want: http.StatusNotFound},
		{name: "backslash", method: http.MethodGet, target: `/manja-assets/manja\\.css`, want: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(test.method, test.target, nil))
			if response.Code != test.want {
				t.Fatalf("%s %s = %d, want %d", test.method, test.target, response.Code, test.want)
			}
		})
	}
}

func TestCatalogAssetsDoNotExposeEmbeddedDirectories(t *testing.T) {
	response := httptest.NewRecorder()
	NewCatalogAssetsHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/manja-assets/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("GET /manja-assets/ = %d, want %d", response.Code, http.StatusNotFound)
	}
}
