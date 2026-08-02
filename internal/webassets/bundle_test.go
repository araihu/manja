package webassets

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestBuildArtifactsAreDeterministicAndComplete(t *testing.T) {
	root := filepath.Clean("../..")
	first, firstReport, err := buildArtifacts(root)
	if err != nil {
		t.Fatal(err)
	}
	second, secondReport, err := buildArtifacts(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstReport, secondReport) {
		t.Fatal("two builds differ")
	}
	schema := string(first["internal/web/static/schema-example.js"])
	if !strings.HasPrefix(schema, generatedHeader) || !strings.Contains(schema, "OpenAPISampler") || !strings.HasSuffix(schema, "\n") {
		t.Fatalf("invalid schema artifact framing")
	}
	composer := string(first["internal/web/static/request-composer.js"])
	for _, want := range []string{generatedHeader, "ManjaHTTPSnippet", "ManjaHighlight"} {
		if !strings.Contains(composer, want) {
			t.Fatalf("request composer missing %q", want)
		}
	}

	wantPackages := []string{
		"@readme/httpsnippet", "base64-js", "buffer", "call-bind-apply-helpers", "call-bound",
		"dunder-proto", "es-define-property", "es-errors", "es-object-atoms", "function-bind",
		"get-intrinsic", "get-own-enumerable-property-symbols", "get-proto", "gopd", "has-symbols",
		"hasown", "highlight.js", "ieee754", "is-obj", "is-regexp", "math-intrinsics",
		"object-inspect", "punycode", "qs", "side-channel", "side-channel-list", "side-channel-map",
		"side-channel-weakmap", "stringify-object", "url",
	}
	sort.Strings(wantPackages)
	composerReport := reportByName(t, firstReport, "request-composer")
	if !reflect.DeepEqual(composerReport.Packages, wantPackages) {
		t.Fatalf("packages = %v, want %v", composerReport.Packages, wantPackages)
	}
	for _, input := range composerReport.Inputs {
		if strings.Contains(input, "..") || filepath.IsAbs(input) {
			t.Fatalf("unsafe report input %q", input)
		}
	}
}

func TestGenerateCheckReportsStaleWithoutWriting(t *testing.T) {
	repo := testGenerationRepo(t)
	if _, err := Generate(repo, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(repo, true); err != nil {
		t.Fatalf("fresh check: %v", err)
	}
	output := filepath.Join(repo, "internal/web/static/schema-example.js")
	if err := os.WriteFile(output, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(repo, true); err == nil || !strings.Contains(err.Error(), "schema-example.js is stale") {
		t.Fatalf("stale check error = %v", err)
	}
	contents, err := os.ReadFile(output)
	if err != nil || string(contents) != "stale\n" {
		t.Fatalf("check mode wrote output: %q, %v", contents, err)
	}
}

func reportByName(t *testing.T, report Report, name string) BundleReport {
	t.Helper()
	for _, bundle := range report.Bundles {
		if bundle.Name == name {
			return bundle
		}
	}
	t.Fatalf("missing report %q", name)
	return BundleReport{}
}

func testGenerationRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, relative := range []string{
		"internal/webassets/vendor.overlay.yaml",
		"internal/web/static/schema-example-hydrator.js",
		"internal/web/static/request-composer-hydrator.js",
	} {
		source := filepath.Join("../..", filepath.FromSlash(relative))
		contents, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
