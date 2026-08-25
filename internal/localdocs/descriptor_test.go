package localdocs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/internal/adapters/catalogjson"
)

func TestPrepareDescriptorBindsPublicEligibilityAndCanonicalResourceRoutes(t *testing.T) {
	snapshot := descriptorSnapshot(t)
	descriptor, ok := PrepareDescriptor(PublicEligibility{
		CatalogID: "kubernetes", PublicationKey: "public-kubernetes", Public: true, Anonymous: true,
	}, snapshot, "/kubernetes")
	if !ok {
		t.Fatal("PrepareDescriptor rejected eligible immutable snapshot")
	}
	wantBase := "/kubernetes/"
	wantSnapshot := string(snapshot.ID)
	if descriptor.CatalogID != "kubernetes" || descriptor.PublicationKey != "public-kubernetes" || descriptor.PublicationBase != wantBase || descriptor.SnapshotID != wantSnapshot || descriptor.RevisionID != "git-0123456789abcdef" || descriptor.ProjectionFormat != "projection-v2" {
		t.Fatalf("descriptor identity = %#v", descriptor)
	}
	if descriptor.ProjectionManifestURL != wantBase+"snapshots/"+wantSnapshot+"/manifest.json" {
		t.Fatalf("manifest URL = %q", descriptor.ProjectionManifestURL)
	}
	if descriptor.CatalogURL != wantBase+"snapshots/"+wantSnapshot+"/catalog.json" {
		t.Fatalf("catalog URL = %q", descriptor.CatalogURL)
	}
	if descriptor.SearchDataBase != wantBase+"snapshots/"+wantSnapshot+"/search-data/" {
		t.Fatalf("search data base = %q", descriptor.SearchDataBase)
	}
	if descriptor.ProjectionDataBase != wantBase+"snapshots/"+wantSnapshot+"/projection-data/" {
		t.Fatalf("projection data base = %q", descriptor.ProjectionDataBase)
	}
}

func TestPrepareDescriptorFailsClosedForIneligibleOrMismatchedInputs(t *testing.T) {
	snapshot := descriptorSnapshot(t)
	tests := []struct {
		name        string
		eligibility PublicEligibility
		mount       string
		mutate      func(*catalog.RuntimeSnapshot)
	}{
		{name: "private", eligibility: PublicEligibility{CatalogID: "kubernetes", PublicationKey: "public-kubernetes", Anonymous: true}},
		{name: "not anonymous", eligibility: PublicEligibility{CatalogID: "kubernetes", PublicationKey: "public-kubernetes", Public: true}},
		{name: "catalog mismatch", eligibility: PublicEligibility{CatalogID: "other", PublicationKey: "public-kubernetes", Public: true, Anonymous: true}},
		{name: "invalid publication key", eligibility: PublicEligibility{CatalogID: "kubernetes", PublicationKey: "Public-Kubernetes", Public: true, Anonymous: true}},
		{name: "non canonical mount", eligibility: PublicEligibility{CatalogID: "kubernetes", PublicationKey: "public-kubernetes", Public: true, Anonymous: true}, mount: "/kubernetes/%2e%2e/"},
		{name: "snapshot identity mismatch", eligibility: PublicEligibility{CatalogID: "kubernetes", PublicationKey: "public-kubernetes", Public: true, Anonymous: true}, mutate: func(value *catalog.RuntimeSnapshot) { value.Manifest.SnapshotID = "wrong" }},
		{name: "catalog identity mismatch", eligibility: PublicEligibility{CatalogID: "kubernetes", PublicationKey: "public-kubernetes", Public: true, Anonymous: true}, mutate: func(value *catalog.RuntimeSnapshot) { value.Directory.CatalogID = "other" }},
		{name: "children drift", eligibility: PublicEligibility{CatalogID: "kubernetes", PublicationKey: "public-kubernetes", Public: true, Anonymous: true}, mutate: func(value *catalog.RuntimeSnapshot) {
			value.Manifest.Identity.Children[0].SHA256 = strings.Repeat("f", 64)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := snapshot
			candidate.Manifest.Identity.Children = append([]catalog.ChildIdentityV1(nil), snapshot.Manifest.Identity.Children...)
			candidate.Manifest.Children = append([]catalog.ChildIdentityV1(nil), snapshot.Manifest.Children...)
			if test.mutate != nil {
				test.mutate(&candidate)
			}
			mount := test.mount
			if mount == "" {
				mount = "/kubernetes"
			}
			if descriptor, ok := PrepareDescriptor(test.eligibility, candidate, mount); ok || descriptor != (DescriptorV1{}) {
				t.Fatalf("PrepareDescriptor admitted invalid input: ok=%t descriptor=%#v", ok, descriptor)
			}
		})
	}
}

func TestPrepareStaticDescriptorUsesCatalogIdentityAndDeploymentBase(t *testing.T) {
	snapshot := descriptorSnapshot(t)
	descriptor, ok := PrepareStaticDescriptor("kubernetes", snapshot, "/group/project/kubernetes/", "/group/project/")
	if !ok {
		t.Fatal("PrepareStaticDescriptor rejected valid export")
	}
	if descriptor.PublicationKey != "kubernetes" || !descriptor.Public || !descriptor.Anonymous || descriptor.Static == nil {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	want := StaticDescriptorV1{
		DeploymentBase: "/group/project/", WorkerURL: "/group/project/sw.js", WorkerScope: "/group/project/",
		OfflineShellURL: "/group/project/kubernetes/_manja/offline-shell/", ExportManifestURL: "/group/project/_manja/export.json",
	}
	if *descriptor.Static != want {
		t.Fatalf("static descriptor = %#v, want %#v", *descriptor.Static, want)
	}
	manifest, err := catalogjson.EncodeManifest(snapshot.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Admit(descriptor, manifest); err != nil {
		t.Fatalf("Admit static descriptor: %v", err)
	}
}

func TestPrepareStaticDescriptorRejectsEscapingOrInvalidDeploymentBase(t *testing.T) {
	snapshot := descriptorSnapshot(t)
	for _, test := range []struct{ publication, deployment string }{
		{publication: "/kubernetes/", deployment: "/group/project/"},
		{publication: "/group/project/kubernetes/", deployment: "/group//project/"},
		{publication: "/group/project/kubernetes/", deployment: "/group/project/%2e%2e/"},
	} {
		if descriptor, ok := PrepareStaticDescriptor("kubernetes", snapshot, test.publication, test.deployment); ok || !reflect.DeepEqual(descriptor, DescriptorV1{}) {
			t.Fatalf("PrepareStaticDescriptor(%q, %q) = %#v, %t", test.publication, test.deployment, descriptor, ok)
		}
	}
}

func TestAdmitBindsDescriptorCatalogIdentity(t *testing.T) {
	descriptor, manifest := activationFixture(t, "/kubernetes/")
	descriptor.CatalogID = "other"
	activation, err := Admit(descriptor, manifest)
	if err == nil {
		t.Fatalf("catalog-mismatched descriptor admitted activation: %#v", activation)
	}
	if !reflect.DeepEqual(activation, Activation{}) {
		t.Fatalf("catalog-mismatched descriptor returned activation: %#v", activation)
	}
}

func descriptorSnapshot(t *testing.T) catalog.RuntimeSnapshot {
	t.Helper()
	children := []catalog.ChildIdentityV1{
		{Path: "catalog.json", Kind: "catalog", Length: 89, SHA256: strings.Repeat("1", 64)},
		{Path: "details/core.json", Kind: "detail", Length: 137, SHA256: strings.Repeat("a", 64)},
		{Path: "schema-nodes/core-000000.json", Kind: "schema-node", Length: 251, SHA256: strings.Repeat("b", 64)},
		{Path: "search/directory.json", Kind: "search-directory", Length: 73, SHA256: strings.Repeat("2", 64)},
	}
	identity := catalog.SnapshotIdentityV1{
		SchemaVersion: 1, CatalogID: "kubernetes", RevisionID: "git-0123456789abcdef",
		SourceManifestSHA256: strings.Repeat("c", 64),
		Versions:             catalog.CompilerVersions{ProjectionFormat: "projection-v2"},
		Children:             append([]catalog.ChildIdentityV1(nil), children...),
	}
	identityBytes, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(identityBytes)
	snapshotID := catalog.SnapshotID("snapshot-sha256-" + hex.EncodeToString(digest[:]))
	manifest := catalog.ManifestV1{SchemaVersion: 1, SnapshotID: snapshotID, Identity: identity, Children: append([]catalog.ChildIdentityV1(nil), children...)}
	return catalog.RuntimeSnapshot{ID: snapshotID, Directory: catalog.CatalogArtifactV1{SchemaVersion: 1, CatalogID: "kubernetes"}, Manifest: manifest}
}
