package server_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	app "github.com/araihu/manja/internal/selfhosted"
	"github.com/araihu/manja/site/internal/server"
)

func TestStaticSiteAndDemoRunAsSeparateFileTrees(t *testing.T) {
	root := t.TempDir()
	siteRoot, demoRoot := filepath.Join(root, "site"), filepath.Join(root, "demo")
	if err := server.ExportSite(siteRoot); err != nil {
		t.Fatal(err)
	}
	if err := server.ExportDemo(t.Context(), demoRoot); err != nil {
		t.Fatal(err)
	}
	receipt, err := app.VerifyExport(t.Context(), demoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Catalogs) != 3 || receipt.BasePath != "/" {
		t.Fatalf("demo receipt = %#v", receipt)
	}
	site := httptest.NewServer(http.FileServer(http.Dir(siteRoot)))
	defer site.Close()
	demo := httptest.NewServer(http.FileServer(http.Dir(demoRoot)))
	defer demo.Close()
	for _, path := range []string{"/", "/docs/"} {
		body := get(t, site.URL+path, http.StatusOK)
		if !strings.Contains(body, `href="https://demo.manja.araihu.com/"`) {
			t.Fatalf("%s does not link to the independent demo", path)
		}
	}
	get(t, site.URL+"/static/theme.js", http.StatusOK)
	get(t, site.URL+"/manja-assets/manja-social.png", http.StatusOK)
	get(t, site.URL+"/payments/v1/", http.StatusNotFound)
	get(t, demo.URL+"/", http.StatusOK)
	get(t, demo.URL+"/search/", http.StatusOK)
	get(t, demo.URL+"/_manja/search/directory.json", http.StatusOK)
	for _, catalog := range []struct{ mount, document, title string }{
		{"/payments/v1", "payments-api", "Acme Payments API"},
		{"/identity/v2", "identity-api", "Acme Identity API"},
		{"/notifications/v1", "notifications-api", "Acme Notifications API"},
	} {
		body := get(t, demo.URL+catalog.mount+"/documents/"+catalog.document+"/", http.StatusOK)
		if !strings.Contains(body, catalog.title) || !strings.Contains(body, `"htmlOnly":true`) {
			t.Fatalf("%s is not a static document shell", catalog.mount)
		}
	}
	for _, path := range []string{"/docs/", "/manage/", "/api/", "/missing.json", "/payments/v1/documents/missing/"} {
		get(t, demo.URL+path, http.StatusNotFound)
	}
	if err := filepath.WalkDir(demoRoot, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == ".git" || entry.Name() == "repos" || strings.HasSuffix(name, ".wasm") {
			t.Errorf("build state was exported: %s", name)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(siteRoot, "_manja")); !os.IsNotExist(err) {
		t.Fatal("demo output leaked into the product site")
	}
}
