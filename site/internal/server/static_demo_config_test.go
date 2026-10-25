package server

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	configadapter "github.com/araihu/manja/internal/adapters/config"
	"github.com/araihu/manja/site/internal/site"
)

func TestStaticDemoPreservesSelectedPinnedCatalogSources(t *testing.T) {
	sourcePath, err := filepath.Abs("../../demo/renderer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	original, err := configadapter.LoadRenderer(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	generatedPath, err := writeDemoConfig(sourcePath, root)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := configadapter.LoadRenderer(generatedPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated.Catalogs) != 3 {
		t.Fatalf("catalogs = %d, want GitHub, Stripe and Twilio", len(generated.Catalogs))
	}
	wantDocuments := map[string]int{"twilio": 1, "github": 1, "stripe": 1}
	for _, catalog := range generated.Catalogs {
		if catalog.ID == "kubernetes" {
			t.Fatal("Kubernetes must not be published in the static demo")
		}
	}
	for i, got := range generated.Catalogs {
		want := original.Catalogs[i]
		if got.ID != want.ID || got.Mount != want.Mount || got.ProfileID != want.ProfileID ||
			got.DefaultDocumentKey != want.DefaultDocumentKey || got.Source.Kind != want.Source.Kind ||
			!reflect.DeepEqual(got.Source.Include, want.Source.Include) ||
			!reflect.DeepEqual(got.Source.Documents, want.Source.Documents) ||
			got.Source.Repository != want.Source.Repository || got.Source.Ref != want.Source.Ref {
			t.Fatalf("catalog %s changed its pinned source", want.ID)
		}
		if len(got.Source.Documents) != wantDocuments[got.ID] {
			t.Fatalf("catalog %s documents = %d", got.ID, len(got.Source.Documents))
		}
		if got.Source.Kind == configadapter.RendererSourceFiles && got.Source.Root != filepath.Clean(filepath.Join(filepath.Dir(sourcePath), want.Source.Root)) {
			t.Fatalf("catalog %s no longer reads the original local specs", got.ID)
		}
		if got.Source.IntegrityReceipt != nil {
			before, err := os.ReadFile(filepath.Join(filepath.Dir(sourcePath), *want.Source.IntegrityReceipt))
			if err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(filepath.Join(root, *got.Source.IntegrityReceipt))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("catalog %s integrity receipt changed: %v", got.ID, err)
			}
		}
		if got.SEO.CanonicalBase != site.DemoOrigin+got.Mount {
			t.Fatalf("catalog %s canonical URL = %s", got.ID, got.SEO.CanonicalBase)
		}
	}
}

func TestStaticDemoKeepsOriginalGitHubAndStripeRevisions(t *testing.T) {
	original, err := configadapter.LoadRenderer("../../../internal/renderer/testdata/kubernetes/renderer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	demo, err := configadapter.LoadRenderer("../../demo/renderer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, before := range original.Catalogs {
		if before.ID == "kubernetes" {
			continue
		}
		var found bool
		for _, after := range demo.Catalogs {
			if after.ID != before.ID {
				continue
			}
			found = true
			if after.Source.Ref != before.Source.Ref || after.Source.Repository != before.Source.Repository ||
				!reflect.DeepEqual(after.Source.Documents, before.Source.Documents) || after.ProfileID != before.ProfileID {
				t.Fatalf("%s original revision changed", before.ID)
			}
			if before.Source.IntegrityReceipt != nil {
				want, err := os.ReadFile(filepath.Join("../../../internal/renderer/testdata/kubernetes", *before.Source.IntegrityReceipt))
				if err != nil {
					t.Fatal(err)
				}
				if after.Source.IntegrityReceipt == nil {
					t.Fatalf("%s integrity receipt missing", before.ID)
				}
				got, err := os.ReadFile(filepath.Join("../../demo", *after.Source.IntegrityReceipt))
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("%s integrity receipt changed: %v", before.ID, err)
				}
			}
		}
		if !found {
			t.Fatalf("original %s catalog missing", before.ID)
		}
	}
}
