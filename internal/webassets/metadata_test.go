package webassets

import "testing"

func TestLoadRepositoryMetadataMatchesBundles(t *testing.T) {
	bundles, err := LoadRepositoryMetadata("../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 2 {
		t.Fatalf("bundles = %d, want 2", len(bundles))
	}
	want := map[string]int{"schema-example": 1, "request-composer": 30}
	for _, bundle := range bundles {
		if len(bundle.Packages) != want[bundle.Name] {
			t.Fatalf("%s packages = %d, want %d", bundle.Name, len(bundle.Packages), want[bundle.Name])
		}
		for _, pkg := range bundle.Packages {
			if pkg.Name == "" || pkg.Version == "" || pkg.SPDX == "" || pkg.Homepage == "" {
				t.Fatalf("incomplete package: %+v", pkg)
			}
		}
	}
}
