package domain

import (
	"strings"
	"testing"
)

func TestNewOperationDetailIDUsesCanonicalTuple(t *testing.T) {
	t.Parallel()

	id, err := NewOperationDetailID("kubernetes", "core-v1", "get", "/api/v1/pods")
	if err != nil {
		t.Fatal(err)
	}
	const want = DetailID("detail-sha256-976e194a0d3442bcf30a41f2f2723c99e2f7e84cc3e0f6d869f3cbacd7eabec2")
	if id != want {
		t.Fatalf("operation detail ID = %q, want %q", id, want)
	}
}

func TestNewSchemaDetailIDUsesLiteralCaseSensitiveName(t *testing.T) {
	t.Parallel()

	id, err := NewSchemaDetailID("kubernetes", "core-v1", "io.k8s.api.core.v1.PodSpec")
	if err != nil {
		t.Fatal(err)
	}
	const want = DetailID("detail-sha256-774b580479a7412442de22676ac241ca556cbe137d33a6fd9fddfaff5ea881ee")
	if id != want {
		t.Fatalf("schema detail ID = %q, want %q", id, want)
	}

	lower, err := NewSchemaDetailID("kubernetes", "core-v1", "io.k8s.api.core.v1.podspec")
	if err != nil {
		t.Fatal(err)
	}
	if lower == id {
		t.Fatal("case-distinct literal schema names produced the same detail ID")
	}
}

func TestDetailIdentityIgnoresOperationID(t *testing.T) {
	t.Parallel()

	first, err := NewOperationDetailID("kubernetes", "core-v1", "GET", "/api/v1/pods")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewOperationDetailID("kubernetes", "apps-v1", "GET", "/apis/apps/v1/deployments")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("operations with different document/method/path identities collided")
	}
}

func TestDetailIdentityRejectsNoncanonicalInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func() error
	}{
		{name: "empty catalog", call: func() error { _, err := NewOperationDetailID("", "core-v1", "GET", "/api/v1/pods"); return err }},
		{name: "uppercase catalog", call: func() error {
			_, err := NewOperationDetailID("Kubernetes", "core-v1", "GET", "/api/v1/pods")
			return err
		}},
		{name: "invalid document key", call: func() error {
			_, err := NewOperationDetailID("kubernetes", "core/v1", "GET", "/api/v1/pods")
			return err
		}},
		{name: "invalid method", call: func() error {
			_, err := NewOperationDetailID("kubernetes", "core-v1", "G ET", "/api/v1/pods")
			return err
		}},
		{name: "relative path", call: func() error {
			_, err := NewOperationDetailID("kubernetes", "core-v1", "GET", "api/v1/pods")
			return err
		}},
		{name: "dot segment path", call: func() error {
			_, err := NewOperationDetailID("kubernetes", "core-v1", "GET", "/api/../pods")
			return err
		}},
		{name: "query path", call: func() error {
			_, err := NewOperationDetailID("kubernetes", "core-v1", "GET", "/api/v1/pods?watch=1")
			return err
		}},
		{name: "empty schema", call: func() error { _, err := NewSchemaDetailID("kubernetes", "core-v1", ""); return err }},
		{name: "padded schema", call: func() error { _, err := NewSchemaDetailID("kubernetes", "core-v1", " PodSpec"); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.call(); err == nil {
				t.Fatal("noncanonical detail identity was accepted")
			}
		})
	}
}

func TestOperationDetailIDAcceptsLiteralKubernetesDiscoveryTrailingSlash(t *testing.T) {
	t.Parallel()

	withoutSlash, err := NewOperationDetailID("kubernetes", "core-discovery", "GET", "/api")
	if err != nil {
		t.Fatal(err)
	}
	withSlash, err := NewOperationDetailID("kubernetes", "core-discovery", "GET", "/api/")
	if err != nil {
		t.Fatal(err)
	}
	if withSlash == withoutSlash {
		t.Fatal("literal trailing slash was normalized out of operation identity")
	}
}

func TestValidateCatalogIndexRejectsInjectedDetailHashCollision(t *testing.T) {
	t.Parallel()

	index := CatalogIndex{
		CatalogID:  "kubernetes",
		RevisionID: "file-manifest-a",
		ProfileID:  CompatibilityProfileStrict,
		Documents: []CatalogDocumentIndex{
			{Key: "apps-v1", SourcePath: "apis/apps/v1_openapi.json", Index: SpecIndex{Operations: []Operation{{Method: "GET", Path: "/apis/apps/v1/deployments"}}}},
			{Key: "core-v1", SourcePath: "api/v1_openapi.json", Index: SpecIndex{Operations: []Operation{{Method: "GET", Path: "/api/v1/pods"}}}},
		},
	}
	constantHash := func([]byte) [32]byte { return [32]byte{} }
	err := validateCatalogIndexWithDetailHasher(index, constantHash, ValidationOptions{ResourceLimits: true})
	if err == nil {
		t.Fatal("catalog index accepted different detail preimages with one digest")
	}
	if !strings.Contains(err.Error(), "collision") {
		t.Fatalf("collision error = %q, want collision diagnostic", err)
	}
}

func TestValidateCatalogIndexAcceptsDuplicateOperationIDsWithDistinctCanonicalIdentities(t *testing.T) {
	t.Parallel()

	index := CatalogIndex{
		CatalogID:  "kubernetes",
		RevisionID: "file-manifest-a",
		ProfileID:  CompatibilityProfileStrict,
		Documents: []CatalogDocumentIndex{
			{Key: "apps-v1", SourcePath: "apis/apps/v1_openapi.json", Index: SpecIndex{Operations: []Operation{{ID: "list", Method: "GET", Path: "/apis/apps/v1/deployments"}}}},
			{Key: "core-v1", SourcePath: "api/v1_openapi.json", Index: SpecIndex{Operations: []Operation{{ID: "list", Method: "GET", Path: "/api/v1/pods"}}}},
		},
	}
	if err := ValidateCatalogIndex(index); err != nil {
		t.Fatalf("valid catalog index: %v", err)
	}
}
