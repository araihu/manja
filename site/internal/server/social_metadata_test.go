package server_test

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
)

const (
	productionSiteOrigin = "https://manja.araihu.com"
	socialPreviewURL     = productionSiteOrigin + "/manja-assets/manja-social.png"
)

func TestProductPagesEmitCompleteSocialMetadata(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	pages := []struct {
		path        string
		title       string
		description string
		canonical   string
	}{
		{
			path:        "/",
			title:       "Publish OpenAPI docs from source | Manja",
			description: "Connect Manja to an OpenAPI file or Git source, choose a revision, and publish read-only API documentation from a stable known-good version.",
			canonical:   productionSiteOrigin + "/",
		},
		{
			path:        "/docs",
			title:       "Run and publish Manja | OpenAPI docs",
			description: "Run Manja locally or with Docker, connect an OpenAPI source, choose a revision, and publish read-only documentation from a known-good version.",
			canonical:   productionSiteOrigin + "/docs",
		},
	}

	for _, page := range pages {
		page := page
		t.Run(page.path, func(t *testing.T) {
			t.Parallel()

			body := get(t, srv.URL+page.path, http.StatusOK)
			for _, value := range []string{page.canonical, socialPreviewURL} {
				parsed, err := url.Parse(value)
				if err != nil {
					t.Fatalf("parse metadata URL %q: %v", value, err)
				}
				if parsed.Scheme != "https" || parsed.Host != "manja.araihu.com" || !parsed.IsAbs() {
					t.Fatalf("metadata URL = %q, want absolute production HTTPS URL", value)
				}
			}

			exactTags := []string{
				`<title>` + page.title + `</title>`,
				`<meta name="description" content="` + page.description + `">`,
				`<link rel="canonical" href="` + page.canonical + `">`,
				`<meta property="og:url" content="` + page.canonical + `">`,
				`<meta property="og:type" content="website">`,
				`<meta property="og:title" content="` + page.title + `">`,
				`<meta property="og:description" content="` + page.description + `">`,
				`<meta property="og:site_name" content="Manja">`,
				`<meta property="og:image" content="` + socialPreviewURL + `">`,
				`<meta property="og:image:type" content="image/png">`,
				`<meta property="og:image:width" content="1280">`,
				`<meta property="og:image:height" content="640">`,
				`<meta property="og:image:alt" content="Manja OpenAPI documentation preview">`,
				`<meta name="twitter:card" content="summary_large_image">`,
				`<meta name="twitter:title" content="` + page.title + `">`,
				`<meta name="twitter:description" content="` + page.description + `">`,
				`<meta name="twitter:image" content="` + socialPreviewURL + `">`,
				`<meta name="twitter:image:alt" content="Manja OpenAPI documentation preview">`,
			}
			for _, tag := range exactTags {
				if count := strings.Count(body, tag); count != 1 {
					t.Errorf("%s metadata tag count = %d, want 1: %s", page.path, count, tag)
				}
			}

			for _, marker := range []string{
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
			} {
				if count := strings.Count(body, marker); count != 1 {
					t.Errorf("%s metadata marker %q count = %d, want 1", page.path, marker, count)
				}
			}
		})
	}
}

func TestProductSiteServesApprovedSocialPreview(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(newTestServer(t))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/manja-assets/manja-social.png")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("social preview status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/png" {
		t.Fatalf("social preview Content-Type = %q, want image/png", got)
	}

	asset, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read social preview: %v", err)
	}
	image, err := png.Decode(bytes.NewReader(asset))
	if err != nil {
		t.Fatalf("decode social preview: %v", err)
	}
	bounds := image.Bounds()
	if bounds.Dx() != 1280 || bounds.Dy() != 640 {
		t.Fatalf("social preview dimensions = %dx%d, want 1280x640", bounds.Dx(), bounds.Dy())
	}

	if len(asset) >= 1<<20 {
		t.Fatalf("social preview size = %d, want < 1 MiB", len(asset))
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(asset)); got != "7234c9a20fc3a4a44364b8f9d544ddae5aba8c2b6a418b26ad5a930d2d0ab0bd" {
		t.Fatalf("social preview SHA-256 = %s, want approved Manja identity bytes", got)
	}
}
