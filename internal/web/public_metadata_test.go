package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image/png"
	"io"
	"io/fs"
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
	handler := NewPublicServerWithOptions(idx, PublicOptions{PublicOrigin: publicDocsTestOrigin, StaticDir: "static"})

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

func TestPublicDocsMetadataRequiresTrustedAuthority(t *testing.T) {
	t.Parallel()

	handler := NewPublicServerWithOptions(core.SpecIndex{Title: "Petstore"}, PublicOptions{StaticDir: "static"})
	for _, test := range []struct {
		name    string
		host    string
		headers map[string]string
	}{
		{name: "attacker host", host: "attacker.example"},
		{name: "public IP", host: "203.0.113.7:8080"},
		{name: "forwarded authority", host: "attacker.example", headers: map[string]string{
			"Forwarded":         "proto=https;host=docs.example.test",
			"X-Forwarded-Host":  "docs.example.test",
			"X-Forwarded-Proto": "https",
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "http://internal.invalid/", nil)
			request.Host = test.host
			for name, value := range test.headers {
				request.Header.Set(name, value)
			}
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			body := response.Body.String()
			for _, forbidden := range []string{
				`<link rel="canonical"`,
				`<meta property="og:url"`,
				`<meta property="og:image"`,
				`<meta name="twitter:image"`,
			} {
				if strings.Contains(body, forbidden) {
					t.Errorf("untrusted request authority emitted %s", forbidden)
				}
			}
		})
	}
}

func TestPublicDocsMetadataUsesConfiguredOriginInsteadOfRequestHeaders(t *testing.T) {
	t.Parallel()

	opts := publicOptionsWithOrigin(t, "https://docs.example.test/")
	handler := NewPublicServerWithOptions(core.SpecIndex{Title: "Petstore"}, opts)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://internal.invalid/", nil)
	request.Host = "attacker.example"
	request.Header.Set("Forwarded", "proto=http;host=forwarded-attacker.example")
	request.Header.Set("X-Forwarded-Host", "forwarded-attacker.example")
	request.Header.Set("X-Forwarded-Proto", "http")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	for _, want := range []string{
		`<link rel="canonical" href="https://docs.example.test/">`,
		`<meta property="og:url" content="https://docs.example.test/">`,
		`<meta property="og:image" content="https://docs.example.test/manja-assets/manja-social.png">`,
		`<meta name="twitter:image" content="https://docs.example.test/manja-assets/manja-social.png">`,
	} {
		if count := strings.Count(body, want); count != 1 {
			t.Errorf("trusted metadata %q count = %d, want 1", want, count)
		}
	}
	for _, forbidden := range []string{"attacker.example", "forwarded-attacker.example", "http://docs.example.test"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("configured metadata contains untrusted authority %q", forbidden)
		}
	}
}

func TestPublicDocsMetadataRejectsInvalidConfiguredOrigins(t *testing.T) {
	t.Parallel()

	for _, configuredOrigin := range []string{
		"http://docs.example.test",
		"https://docs.example.test/public",
		"https://user@docs.example.test",
		"https://docs.example.test?tenant=acme",
		"https://docs.example.test#fragment",
		" https://docs.example.test",
		"//docs.example.test",
	} {
		configuredOrigin := configuredOrigin
		t.Run(configuredOrigin, func(t *testing.T) {
			t.Parallel()

			handler := NewPublicServerWithOptions(core.SpecIndex{Title: "Petstore"}, publicOptionsWithOrigin(t, configuredOrigin))
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "http://internal.invalid/", nil)
			request.Host = "localhost:8080"
			handler.ServeHTTP(response, request)
			body := response.Body.String()
			for _, forbidden := range []string{
				`<link rel="canonical"`,
				`<meta property="og:url"`,
				`<meta property="og:image"`,
				`<meta name="twitter:image"`,
			} {
				if strings.Contains(body, forbidden) {
					t.Errorf("invalid configured origin %q emitted %s", configuredOrigin, forbidden)
				}
			}
		})
	}
}

func TestPublicDocsMetadataAllowsOnlyExplicitLoopbackDevelopmentAuthority(t *testing.T) {
	t.Parallel()

	handler := NewPublicServerWithOptions(core.SpecIndex{Title: "Petstore"}, PublicOptions{StaticDir: "static"})
	for _, test := range []struct {
		name      string
		host      string
		canonical string
	}{
		{name: "localhost", host: "localhost:8080", canonical: "http://localhost:8080/"},
		{name: "IPv4 loopback", host: "127.0.0.1:8080", canonical: "http://127.0.0.1:8080/"},
		{name: "IPv6 loopback", host: "[::1]:8080", canonical: "http://[::1]:8080/"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "http://internal.invalid/", nil)
			request.Host = test.host
			handler.ServeHTTP(response, request)
			body := response.Body.String()
			for _, want := range []string{
				`<link rel="canonical" href="` + test.canonical + `">`,
				`<meta property="og:url" content="` + test.canonical + `">`,
			} {
				if count := strings.Count(body, want); count != 1 {
					t.Errorf("loopback metadata %q count = %d, want 1", want, count)
				}
			}
		})
	}
}

func TestPublishedDocsMetadataPreservesExternalPublicationPath(t *testing.T) {
	t.Parallel()

	idx := publicDocsMetadataTestIndex()
	store := publicMetadataPublicationStore{publication: core.Publication{
		ProjectID: "payments", RevisionID: "revision-1", Public: true, Path: "/payments/v1",
	}}
	handler := NewServerWithOptions(idx, Options{
		Public:     PublicOptions{PublicOrigin: publicDocsTestOrigin, StaticDir: "static"},
		Management: ManagementOptions{Store: store},
	})
	for _, page := range []struct {
		name      string
		target    string
		canonical string
	}{
		{name: "overview", target: "/payments/v1", canonical: publicDocsTestOrigin + "/payments/v1"},
		{name: "operation", target: "/payments/v1?selected=operation-listPets", canonical: publicDocsTestOrigin + "/payments/v1?selected=operation-listPets"},
		{name: "schema", target: "/payments/v1?selected=schema-pet", canonical: publicDocsTestOrigin + "/payments/v1?selected=schema-pet"},
	} {
		t.Run(page.name, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, page.target, nil)
			request.Host = "docs.example.test"
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d", page.target, response.Code, http.StatusOK)
			}
			body := response.Body.String()
			for _, want := range []string{
				`<link rel="canonical" href="` + page.canonical + `">`,
				`<meta property="og:url" content="` + page.canonical + `">`,
			} {
				if count := strings.Count(body, want); count != 1 {
					t.Errorf("published metadata %q count = %d, want 1", want, count)
				}
			}
		})
	}
}

func TestPublicDocsMetadataCanonicalizesResolvedSelection(t *testing.T) {
	t.Parallel()

	handler := NewPublicServerWithOptions(publicDocsMetadataTestIndex(), PublicOptions{PublicOrigin: publicDocsTestOrigin, StaticDir: "static"})
	for _, test := range []struct {
		name      string
		target    string
		canonical string
		selected  string
		robots    string
	}{
		{name: "canonical operation", target: "/?selected=operation-listPets", canonical: publicDocsTestOrigin + "/?selected=operation-listPets", selected: "operation-listPets"},
		{name: "hash-prefixed operation", target: "/?selected=%23operation-listPets", canonical: publicDocsTestOrigin + "/?selected=operation-listPets", selected: "operation-listPets"},
		{name: "absent overview", target: "/", canonical: publicDocsTestOrigin + "/", selected: "overview"},
		{name: "named overview", target: "/?selected=overview", canonical: publicDocsTestOrigin + "/", selected: "overview"},
		{name: "hash-prefixed overview", target: "/?selected=%23overview", canonical: publicDocsTestOrigin + "/", selected: "overview"},
		{name: "invalid selection", target: "/?selected=operation-missing", canonical: publicDocsTestOrigin + "/", selected: "operation-missing", robots: "noindex,follow"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.target, nil)
			request.Host = "docs.example.test"
			handler.ServeHTTP(response, request)
			body := response.Body.String()
			for _, want := range []string{
				`<link rel="canonical" href="` + test.canonical + `">`,
				`<meta property="og:url" content="` + test.canonical + `">`,
			} {
				if count := strings.Count(body, want); count != 1 {
					t.Errorf("resolved selection marker %q count = %d, want 1", want, count)
				}
			}
			if want := `data-selected-doc="` + test.selected + `"`; !strings.Contains(body, want) {
				t.Errorf("resolved selection marker %q missing", want)
			}
			if test.robots != "" && !strings.Contains(body, `<meta name="robots" content="`+test.robots+`">`) {
				t.Errorf("invalid selection robots metadata missing")
			}
		})
	}
}

func publicDocsMetadataTestIndex() core.SpecIndex {
	return core.SpecIndex{
		Title:    "Petstore",
		Overview: core.SpecOverview{Description: "Petstore contract overview."},
		Operations: []core.Operation{{
			ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Description: "Lists every pet.",
		}},
		Schemas: []core.Schema{{Name: "Pet", Description: "A pet returned by the API."}},
	}
}

func publicOptionsWithOrigin(t *testing.T, origin string) PublicOptions {
	t.Helper()
	return PublicOptions{PublicOrigin: origin, StaticDir: "static"}
}

type publicMetadataPublicationStore struct {
	publication core.Publication
}

func (store publicMetadataPublicationStore) SavePublication(context.Context, core.Publication) error {
	return nil
}

func (store publicMetadataPublicationStore) PublicPublicationByPath(_ context.Context, publicPath string) (core.Publication, error) {
	if publicPath != store.publication.Path {
		return core.Publication{}, fs.ErrNotExist
	}
	return store.publication, nil
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
