package web

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "github.com/araihu/manja/domain"
	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
)

type sitemapTestURLSet struct {
	URLs []sitemapTestURL `xml:"url"`
}

type sitemapTestURL struct {
	Loc string `xml:"loc"`
}

func TestSitemapUsesPublicRoutes(t *testing.T) {
	spec := []byte(`
openapi: 3.1.0
info:
  title: Widget API
  version: 1.0.0
paths:
  /widgets:
    get:
      operationId: getWidgets
      summary: List widgets
      responses:
        "200":
          description: ok
components:
  schemas:
    Widget:
      type: object
      description: A widget resource.
`)
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(context.Background(), core.SpecFile{
		SourceID: "src1",
		Path:     "widgets.yaml",
		Format:   "yaml",
		Bytes:    spec,
	}, core.Revision{ID: "rev1", SourceID: "src1", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewPublicServerWithOptions(idx, PublicOptions{PublicOrigin: "https://docs.example.test"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	req.Host = "attacker.example"
	req.Header.Set("Forwarded", "proto=http;host=forwarded-attacker.example")

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var sitemap sitemapTestURLSet
	if err := xml.Unmarshal(rec.Body.Bytes(), &sitemap); err != nil {
		t.Fatalf("parse sitemap: %v\n%s", err, rec.Body.String())
	}
	locs := make([]string, 0, len(sitemap.URLs))
	for _, u := range sitemap.URLs {
		locs = append(locs, u.Loc)
	}

	want := []string{
		"https://docs.example.test/",
		"https://docs.example.test/?selected=operation-getwidgets#operation-getwidgets",
		"https://docs.example.test/?selected=schema-widget#schema-widget",
	}
	if len(locs) != len(want) {
		t.Fatalf("locs = %#v, want %#v", locs, want)
	}
	for i := range want {
		if locs[i] != want[i] {
			t.Fatalf("locs = %#v, want %#v", locs, want)
		}
	}
	for _, loc := range locs {
		if strings.Contains(loc, "#schema-") || strings.Contains(loc, "#operation-") {
			continue
		}
		if loc != "https://docs.example.test/" {
			t.Fatalf("sitemap loc is not a canonical docs URL: %q", loc)
		}
	}
}

func TestSitemapFailsClosedWithoutTrustedRemoteOrigin(t *testing.T) {
	t.Parallel()

	srv := NewPublicServer(core.SpecIndex{PublicRoutes: []core.PublicRoute{{Path: "/", Title: "Overview"}}})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	req.Host = "attacker.example"
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(rec.Body.String(), "attacker.example") {
		t.Fatalf("untrusted request Host reached sitemap: %s", rec.Body.String())
	}
}
