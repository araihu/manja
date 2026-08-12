package web

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	core "github.com/araihu/manja/domain"
)

const (
	publicDocsTestOrigin  = "https://docs.example.test"
	publicDocsSocialImage = publicDocsTestOrigin + "/manja-assets/manja-social.png"
	publicDocsSocialAlt   = "Manja OpenAPI documentation preview"
)

func TestPublicDocsInitialHTMLIncludesCompleteRouteSocialMetadata(t *testing.T) {
	t.Parallel()

	idx := core.SpecIndex{
		Title: "Petstore",
		Overview: core.SpecOverview{
			Description: "Petstore contract overview.",
		},
		Operations: []core.Operation{{
			ID:          "listPets",
			Method:      "GET",
			Path:        "/pets",
			Summary:     "List pets",
			Description: "Lists every pet.",
		}},
		Schemas: []core.Schema{{
			Name:        "Pet",
			Description: "A pet returned by the API.",
		}},
	}
	handler := NewPublicServerWithOptions(idx, PublicOptions{StaticDir: "static"})

	pages := []struct {
		name        string
		target      string
		title       string
		description string
		canonical   string
	}{
		{
			name:        "overview",
			target:      "/?utm_source=ignored",
			title:       "Petstore · Manja",
			description: "Petstore contract overview.",
			canonical:   publicDocsTestOrigin + "/",
		},
		{
			name:        "operation",
			target:      "/?utm_source=ignored&selected=operation-listPets",
			title:       "List pets · Manja",
			description: "Lists every pet.",
			canonical:   publicDocsTestOrigin + "/?selected=operation-listPets",
		},
		{
			name:        "schema",
			target:      "/?selected=schema-pet&panel=ignored",
			title:       "Pet · Manja",
			description: "A pet returned by the API.",
			canonical:   publicDocsTestOrigin + "/?selected=schema-pet",
		},
	}

	for _, page := range pages {
		page := page
		t.Run(page.name, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, page.target, nil)
			request.Host = "docs.example.test"
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d; body: %s", page.target, response.Code, http.StatusOK, response.Body.String())
			}
			if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
				t.Fatalf("GET %s Content-Type = %q, want text/html; charset=utf-8", page.target, got)
			}
			for _, rawURL := range []string{page.canonical, publicDocsSocialImage} {
				parsed, err := url.Parse(rawURL)
				if err != nil {
					t.Fatalf("parse metadata URL %q: %v", rawURL, err)
				}
				if parsed.Scheme != "https" || parsed.Host != "docs.example.test" || !parsed.IsAbs() {
					t.Fatalf("metadata URL = %q, want absolute production HTTPS URL", rawURL)
				}
			}

			body := response.Body.String()
			exactTags := []string{
				`<title>` + page.title + `</title>`,
				`<meta name="description" content="` + page.description + `">`,
				`<link rel="canonical" href="` + page.canonical + `">`,
				`<meta property="og:url" content="` + page.canonical + `">`,
				`<meta property="og:type" content="website">`,
				`<meta property="og:title" content="` + page.title + `">`,
				`<meta property="og:description" content="` + page.description + `">`,
				`<meta property="og:site_name" content="Manja">`,
				`<meta property="og:image" content="` + publicDocsSocialImage + `">`,
				`<meta property="og:image:type" content="image/png">`,
				`<meta property="og:image:width" content="1280">`,
				`<meta property="og:image:height" content="640">`,
				`<meta property="og:image:alt" content="` + publicDocsSocialAlt + `">`,
				`<meta name="twitter:card" content="summary_large_image">`,
				`<meta name="twitter:title" content="` + page.title + `">`,
				`<meta name="twitter:description" content="` + page.description + `">`,
				`<meta name="twitter:image" content="` + publicDocsSocialImage + `">`,
				`<meta name="twitter:image:alt" content="` + publicDocsSocialAlt + `">`,
			}
			for _, tag := range exactTags {
				if count := strings.Count(body, tag); count != 1 {
					t.Errorf("%s metadata tag count = %d, want 1: %s", page.name, count, tag)
				}
			}
			for _, marker := range publicDocsMetadataMarkers() {
				if count := strings.Count(body, marker); count != 1 {
					t.Errorf("%s metadata marker %q count = %d, want 1", page.name, marker, count)
				}
			}
		})
	}
}

func TestPublicDocsServesApprovedSocialPreview(t *testing.T) {
	t.Parallel()

	handler := NewPublicServerWithOptions(core.SpecIndex{Title: "Petstore"}, PublicOptions{StaticDir: "static"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/manja-assets/manja-social.png", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("social preview status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("social preview Content-Type = %q, want image/png", got)
	}

	asset, err := io.ReadAll(response.Result().Body)
	if err != nil {
		t.Fatalf("read social preview: %v", err)
	}
	image, err := png.Decode(bytes.NewReader(asset))
	if err != nil {
		t.Fatalf("decode social preview: %v", err)
	}
	if width, height := image.Bounds().Dx(), image.Bounds().Dy(); width != 1280 || height != 640 {
		t.Fatalf("social preview dimensions = %dx%d, want 1280x640", width, height)
	}
	if len(asset) >= 1<<20 {
		t.Fatalf("social preview size = %d, want < 1 MiB", len(asset))
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(asset)); got != "7234c9a20fc3a4a44364b8f9d544ddae5aba8c2b6a418b26ad5a930d2d0ab0bd" {
		t.Fatalf("social preview SHA-256 = %s, want approved Manja identity bytes", got)
	}
}

func publicDocsMetadataMarkers() []string {
	return []string{
		`<title>`,
		`<meta name="description"`,
		`<link rel="canonical"`,
		`<meta property="og:url"`,
		`<meta property="og:type"`,
		`<meta property="og:title"`,
		`<meta property="og:description"`,
		`<meta property="og:site_name"`,
		`<meta property="og:image"`,
		`<meta property="og:image:type"`,
		`<meta property="og:image:width"`,
		`<meta property="og:image:height"`,
		`<meta property="og:image:alt"`,
		`<meta name="twitter:card"`,
		`<meta name="twitter:title"`,
		`<meta name="twitter:description"`,
		`<meta name="twitter:image"`,
		`<meta name="twitter:image:alt"`,
	}
}
