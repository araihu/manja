package htmlartifact

import (
	"strings"
	"testing"
)

func TestFragmentLocationIsStableAtRootAndNestedCatalogMounts(t *testing.T) {
	t.Parallel()
	identity := FragmentIdentity{Format: FragmentFormatV2, Kind: FragmentOperation, Resource: "detail-sha256-0123456789abcdef"}
	rootHTML, rootSidecar, err := FragmentLocation("/", "vcenter", identity)
	if err != nil {
		t.Fatal(err)
	}
	const suffix = "documents/vcenter/_manja/fragments/operations/operation-sha256-"
	if !strings.HasPrefix(rootHTML, suffix) || !strings.HasSuffix(rootHTML, ".html") || rootSidecar != rootHTML+".meta.json" {
		t.Fatalf("root fragment location = %q, %q", rootHTML, rootSidecar)
	}
	nestedHTML, nestedSidecar, err := FragmentLocation("/catalogs/vmware-vsphere", "vcenter", identity)
	if err != nil {
		t.Fatal(err)
	}
	if nestedHTML != "catalogs/vmware-vsphere/"+rootHTML || nestedSidecar != nestedHTML+".meta.json" {
		t.Fatalf("nested fragment location = %q, %q", nestedHTML, nestedSidecar)
	}
	repeated, _, err := FragmentLocation("/catalogs/vmware-vsphere", "vcenter", identity)
	if err != nil || repeated != nestedHTML {
		t.Fatalf("repeated fragment location = %q, %v", repeated, err)
	}
}

func TestFragmentLocationSeparatesKindsAndRejectsUnsafeInputs(t *testing.T) {
	t.Parallel()
	operation := FragmentIdentity{Format: FragmentFormatV2, Kind: FragmentOperation, Resource: "same-resource"}
	schema := FragmentIdentity{Format: FragmentFormatV2, Kind: FragmentSchema, Resource: "same-resource"}
	operationPath, _, err := FragmentLocation("/catalogs/apis", "core-v1", operation)
	if err != nil {
		t.Fatal(err)
	}
	schemaPath, _, err := FragmentLocation("/catalogs/apis", "core-v1", schema)
	if err != nil {
		t.Fatal(err)
	}
	if operationPath == schemaPath || !strings.Contains(operationPath, "/operations/") || !strings.Contains(schemaPath, "/schemas/") {
		t.Fatalf("kind-separated paths = %q, %q", operationPath, schemaPath)
	}
	for _, test := range []struct {
		mount    string
		spec     string
		identity FragmentIdentity
	}{
		{mount: "catalogs/apis", spec: "core-v1", identity: operation},
		{mount: "/catalogs/apis/", spec: "core-v1", identity: operation},
		{mount: "/catalogs/apis", spec: "../escape", identity: operation},
		{mount: "/catalogs/apis", spec: "core-v1", identity: FragmentIdentity{Format: "old", Kind: FragmentOperation, Resource: "same-resource"}},
	} {
		if htmlPath, sidecarPath, err := FragmentLocation(test.mount, test.spec, test.identity); err == nil {
			t.Errorf("unsafe location accepted as %q, %q", htmlPath, sidecarPath)
		}
	}
}
