package web

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/araihu/manja/internal/core"
)

type sitemapTestURLSet struct {
	URLs []sitemapTestURL `xml:"url"`
}

type sitemapTestURL struct {
	Loc string `xml:"loc"`
}

func TestSitemapUsesPublicRoutes(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Petstore",
		PublicRoutes: []core.PublicRoute{
			{Path: "/", Title: "Petstore"},
			{Path: "/operations/listPets", Title: "GET /pets"},
			{Path: "#operation-listpets", Title: "GET /pets anchor"},
			{Path: "", Title: "empty route"},
		},
	}
	srv := NewPublicServer(idx)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	req.Host = "docs.example.test"

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
		"https://docs.example.test/operations/listPets",
	}
	if len(locs) != len(want) {
		t.Fatalf("locs = %#v, want %#v", locs, want)
	}
	for i := range want {
		if locs[i] != want[i] {
			t.Fatalf("locs = %#v, want %#v", locs, want)
		}
	}
}
