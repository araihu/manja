package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/araihu/manja/domain"
)

func TestBuildKubernetesDefaultAllowlistCapturesExactInvalidDefault(t *testing.T) {
	t.Parallel()

	document := domain.CatalogDocument{
		Key: "example-v1", SourcePath: "specs/example.json", Format: domain.CatalogFormatJSON,
		Bytes: []byte(`{
  "openapi":"3.0.3",
  "info":{"title":"Example","version":"v1"},
  "paths":{},
  "components":{"schemas":{"Thing":{"type":"object","required":["name"],"default":{}}}}
}`),
	}
	first, err := BuildKubernetesDefaultAllowlist(context.Background(), []domain.CatalogDocument{document})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildKubernetesDefaultAllowlist(context.Background(), []domain.CatalogDocument{document})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("default allowlist bytes are not deterministic")
	}
	var allowlist defaultAllowlist
	if err := json.Unmarshal(first, &allowlist); err != nil {
		t.Fatal(err)
	}
	if allowlist.SchemaVersion != 1 || len(allowlist.Diagnostics) != 1 {
		t.Fatalf("allowlist = %#v", allowlist)
	}
	want := defaultDiagnostic{
		DocumentPath: "specs/example.json", JSONPointer: "/components/schemas/Thing/default",
		Class:                defaultDiagnosticClass,
		OffendingValueSHA256: "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
		SchemaSHA256:         "ec76cb3bceb3d412514c82156dad1c4dd30ebe6a0abd80ae9bfa588b375fc911",
	}
	if allowlist.Diagnostics[0] != want {
		t.Fatalf("diagnostic = %#v, want %#v", allowlist.Diagnostics[0], want)
	}
}

func TestKubernetesDefaultAllowlistRejectsEveryDiagnosticDrift(t *testing.T) {
	t.Parallel()

	document := domain.CatalogDocument{
		Key: "example-v1", SourcePath: "specs/example.json", Format: domain.CatalogFormatJSON,
		Bytes: []byte(`{"openapi":"3.0.3","info":{"title":"Example","version":"v1"},"paths":{},"components":{"schemas":{"Thing":{"type":"object","required":["name"],"default":{}}}}}`),
	}
	allowlistBytes, err := BuildKubernetesDefaultAllowlist(context.Background(), []domain.CatalogDocument{document})
	if err != nil {
		t.Fatal(err)
	}
	candidate := catalogParserCandidate(document)
	candidate.ProfileID = domain.CompatibilityProfileKubernetes
	parser, err := NewCatalogParser(allowlistBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(context.Background(), candidate); err != nil {
		t.Fatalf("exact allowlist: %v", err)
	}

	var exact defaultAllowlist
	if err := json.Unmarshal(allowlistBytes, &exact); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*defaultAllowlist){
		"added": func(value *defaultAllowlist) {
			added := value.Diagnostics[0]
			added.JSONPointer = "/components/schemas/Other/default"
			value.Diagnostics = append(value.Diagnostics, added)
		},
		"removed":      func(value *defaultAllowlist) { value.Diagnostics = nil },
		"moved":        func(value *defaultAllowlist) { value.Diagnostics[0].JSONPointer = "/components/schemas/Other/default" },
		"reclassified": func(value *defaultAllowlist) { value.Diagnostics[0].Class = "other_validation" },
		"value changed": func(value *defaultAllowlist) {
			value.Diagnostics[0].OffendingValueSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
		"schema changed": func(value *defaultAllowlist) {
			value.Diagnostics[0].SchemaSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			changed := defaultAllowlist{SchemaVersion: exact.SchemaVersion, Diagnostics: append([]defaultDiagnostic(nil), exact.Diagnostics...)}
			mutate(&changed)
			data, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			changedParser, err := NewCatalogParser(data)
			if err == nil {
				_, err = changedParser.Parse(context.Background(), candidate)
			}
			if err == nil {
				t.Fatal("changed diagnostic set was accepted")
			}
		})
	}
}

func TestKubernetesProfileRetainsUnrelatedDocumentValidation(t *testing.T) {
	t.Parallel()

	document := domain.CatalogDocument{
		Key: "example-v1", SourcePath: "specs/example.json", Format: domain.CatalogFormatJSON,
		Bytes: []byte(`{"openapi":"3.0.3","info":{"title":"Example","version":"v1"},"paths":{"/broken":{"get":{"responses":{"200":{}}}}},"components":{"schemas":{"Thing":{"type":"object","required":["name"],"default":{}}}}}`),
	}
	allowlistBytes, err := BuildKubernetesDefaultAllowlist(context.Background(), []domain.CatalogDocument{document})
	if err != nil {
		t.Fatal(err)
	}
	candidate := catalogParserCandidate(document)
	candidate.ProfileID = domain.CompatibilityProfileKubernetes
	parser, err := NewCatalogParser(allowlistBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse(context.Background(), candidate); err == nil {
		t.Fatal("unrelated invalid response passed Kubernetes profile")
	}
}
