package webassets

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNoticeIncludesEveryShippedPackage(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bundles, err := LoadRepositoryMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := buildArtifacts(root)
	if err != nil {
		t.Fatal(err)
	}
	notice, err := renderNotices(bundles, report)
	if err != nil {
		t.Fatal(err)
	}
	text := string(notice)
	for _, pkg := range allPackages(bundles) {
		for _, want := range []string{pkg.Name, pkg.Version, pkg.SPDX, pkg.ArchiveHash, pkg.LicensePath} {
			if want == "" || !strings.Contains(text, want) {
				t.Fatalf("notice missing %s value %q", pkg.Name, want)
			}
		}
	}
}

func TestNoticeRejectsIncompleteReport(t *testing.T) {
	bundles, err := LoadRepositoryMetadata("../..")
	if err != nil {
		t.Fatal(err)
	}
	report := Report{Bundles: []BundleReport{
		{Name: "schema-example", Packages: []string{"openapi-sampler"}, Inputs: []string{"openapi-sampler/dist/openapi-sampler.js"}},
		{Name: "request-composer"},
	}}
	if _, err := renderNotices(bundles, report); err == nil {
		t.Fatal("incomplete report accepted")
	}
}
