package localdocs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
)

func TestAdmitProjectionInventoryCopiesCanonicalManifestState(t *testing.T) {
	descriptor, manifestBytes := activationFixture(t, "/kubernetes/")

	activation, err := Admit(descriptor, manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	wantInventory := []ProjectionArtifact{
		{Path: "details/core.json", Kind: "detail", Length: 137, SHA256: strings.Repeat("a", 64)},
		{Path: "schema-nodes/core-000000.json", Kind: "schema-node", Length: 251, SHA256: strings.Repeat("b", 64)},
	}
	if activation.CatalogID() != "kubernetes" || activation.PublicationKey() != "public-kubernetes" || activation.SnapshotID() != descriptor.SnapshotID || activation.RevisionID() != "git-0123456789abcdef" || !reflect.DeepEqual(activation.Inventory(), wantInventory) {
		t.Fatalf("activation = catalog=%q key=%q snapshot=%q revision=%q inventory=%#v", activation.CatalogID(), activation.PublicationKey(), activation.SnapshotID(), activation.RevisionID(), activation.Inventory())
	}

	for index := range manifestBytes {
		manifestBytes[index] = 'x'
	}
	if !reflect.DeepEqual(activation.Inventory(), wantInventory) {
		t.Fatalf("activation aliases manifest bytes: %#v", activation.Inventory())
	}
	mutableCopy := activation.Inventory()
	mutableCopy[0].Path = "changed"
	if !reflect.DeepEqual(activation.Inventory(), wantInventory) {
		t.Fatalf("activation returned mutable inventory: %#v", activation.Inventory())
	}
	freshDescriptor, freshManifest := activationFixture(t, "/kubernetes/")
	fresh, err := Admit(freshDescriptor, freshManifest)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Inventory()[0].Path != "details/core.json" {
		t.Fatalf("fresh activation aliases prior result: %#v", fresh.Inventory())
	}
}

func TestAdmitProjectionInventoryFailsClosed(t *testing.T) {
	baseDescriptor, baseManifest := activationFixture(t, "/kubernetes/")
	tests := []struct {
		name   string
		mutate func(*DescriptorV1, *[]byte)
	}{
		{name: "wrong origin", mutate: func(descriptor *DescriptorV1, _ *[]byte) {
			descriptor.ProjectionManifestURL = "https://attacker.example/manifest.json"
		}},
		{name: "protocol relative origin", mutate: func(descriptor *DescriptorV1, _ *[]byte) {
			descriptor.ProjectionManifestURL = "//attacker.example/manifest.json"
		}},
		{name: "manifest traversal", mutate: func(descriptor *DescriptorV1, _ *[]byte) {
			descriptor.ProjectionManifestURL = "/kubernetes/snapshots/../manifest.json"
		}},
		{name: "manifest query", mutate: func(descriptor *DescriptorV1, _ *[]byte) { descriptor.ProjectionManifestURL += "?changed=1" }},
		{name: "encoded traversal base", mutate: mutateDescriptorBase("/kubernetes/%2e%2e/")},
		{name: "query-like base", mutate: mutateDescriptorBase("/kubernetes?attacker=/")},
		{name: "wrong snapshot", mutate: func(descriptor *DescriptorV1, _ *[]byte) {
			descriptor.SnapshotID = "snapshot-sha256-" + strings.Repeat("f", 64)
		}},
		{name: "wrong revision", mutate: func(descriptor *DescriptorV1, _ *[]byte) { descriptor.RevisionID = "git-fedcba9876543210" }},
		{name: "wrong format", mutate: func(descriptor *DescriptorV1, _ *[]byte) { descriptor.ProjectionFormat = "projection-v1" }},
		{name: "wrong digest", mutate: func(descriptor *DescriptorV1, _ *[]byte) { descriptor.ProjectionDigest = strings.Repeat("f", 64) }},
		{name: "wrong data base", mutate: func(descriptor *DescriptorV1, _ *[]byte) {
			descriptor.ProjectionDataBase = "/other/snapshots/" + descriptor.SnapshotID + "/projection-data/"
		}},
		{name: "wrong catalog URL", mutate: func(descriptor *DescriptorV1, _ *[]byte) {
			descriptor.CatalogURL = "/other/snapshots/" + descriptor.SnapshotID + "/catalog.json"
		}},
		{name: "wrong search data base", mutate: func(descriptor *DescriptorV1, _ *[]byte) {
			descriptor.SearchDataBase = "/other/snapshots/" + descriptor.SnapshotID + "/search-data/"
		}},
		{name: "wrong descriptor version", mutate: func(descriptor *DescriptorV1, _ *[]byte) { descriptor.SchemaVersion = 2 }},
		{name: "corrupt JSON", mutate: func(_ *DescriptorV1, manifest *[]byte) { (*manifest)[0] = '[' }},
		{name: "trailing JSON", mutate: func(_ *DescriptorV1, manifest *[]byte) { *manifest = append(*manifest, []byte(`{}`)...) }},
		{name: "unknown JSON field", mutate: func(_ *DescriptorV1, manifest *[]byte) {
			*manifest = append((*manifest)[:len(*manifest)-1], []byte(`,"unknown":true}`)...)
		}},
		{name: "duplicate top-level JSON key", mutate: func(_ *DescriptorV1, manifest *[]byte) {
			*manifest = append([]byte(`{"schemaVersion":1,`), (*manifest)[1:]...)
		}},
		{name: "missing inventory entry", mutate: mutateManifestEnvelope(func(manifest *catalog.ManifestV1) {
			manifest.Children = append(manifest.Children[:1], manifest.Children[2:]...)
		})},
		{name: "extra inventory entry", mutate: mutateManifestEnvelope(func(manifest *catalog.ManifestV1) {
			manifest.Children = append(manifest.Children, catalog.ChildIdentityV1{Path: "zz-extra.json", Kind: "source", Length: 1, SHA256: strings.Repeat("d", 64)})
		})},
		{name: "changed inventory path", mutate: mutateManifestChild(func(child *catalog.ChildIdentityV1) { child.Path = "details/other.json" })},
		{name: "changed inventory kind", mutate: mutateManifestChild(func(child *catalog.ChildIdentityV1) { child.Kind = "source" })},
		{name: "changed inventory size", mutate: mutateManifestChild(func(child *catalog.ChildIdentityV1) { child.Length++ })},
		{name: "changed inventory hash", mutate: mutateManifestChild(func(child *catalog.ChildIdentityV1) { child.SHA256 = strings.Repeat("c", 64) })},
		{name: "coordinated invalid projection path", mutate: mutateManifestAndDescriptor(func(child *catalog.ChildIdentityV1) { child.Path = "detailz/core.json" })},
		{name: "coordinated invalid projection kind", mutate: mutateManifestAndDescriptor(func(child *catalog.ChildIdentityV1) { child.Kind = "source" })},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := baseDescriptor
			manifest := append([]byte(nil), baseManifest...)
			test.mutate(&descriptor, &manifest)
			activation, err := Admit(descriptor, manifest)
			if err == nil {
				t.Fatalf("mutation admitted: %#v", activation)
			}
			if !reflect.DeepEqual(activation, Activation{}) {
				t.Fatalf("failed admission returned state: %#v", activation)
			}
		})
	}
}

func TestAdmitProjectionInventoryAcceptsRootMount(t *testing.T) {
	descriptor, manifestBytes := activationFixture(t, "/")
	activation, err := Admit(descriptor, manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	if activation.PublicationKey() != "public-kubernetes" || len(activation.Inventory()) != 2 {
		t.Fatalf("root activation = %#v", activation)
	}
}

func activationFixture(t *testing.T, publicationBase string) (DescriptorV1, []byte) {
	t.Helper()
	children := []catalog.ChildIdentityV1{
		{Path: "catalog.json", Kind: "catalog", Length: 89, SHA256: strings.Repeat("1", 64)},
		{Path: "details/core.json", Kind: "detail", Length: 137, SHA256: strings.Repeat("a", 64)},
		{Path: "schema-nodes/core-000000.json", Kind: "schema-node", Length: 251, SHA256: strings.Repeat("b", 64)},
		{Path: "search/directory.json", Kind: "search-directory", Length: 73, SHA256: strings.Repeat("2", 64)},
	}
	identity := catalog.SnapshotIdentityV1{
		SchemaVersion: 1,
		CatalogID:     "kubernetes",
		RevisionID:    "git-0123456789abcdef",
		Versions:      catalog.CompilerVersions{ProjectionFormat: "projection-v2"},
		Children:      append([]catalog.ChildIdentityV1(nil), children...),
	}
	identityBytes, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(identityBytes)
	digestHex := hex.EncodeToString(digest[:])
	snapshotID := catalog.SnapshotID("snapshot-sha256-" + digestHex)
	manifestBytes, err := json.Marshal(catalog.ManifestV1{
		SchemaVersion: 1,
		SnapshotID:    snapshotID,
		Identity:      identity,
		Children:      append([]catalog.ChildIdentityV1(nil), children...),
	})
	if err != nil {
		t.Fatal(err)
	}
	manifestURL := publicationBase + "snapshots/" + string(snapshotID) + "/manifest.json"
	catalogURL := publicationBase + "snapshots/" + string(snapshotID) + "/catalog.json"
	searchDataBase := publicationBase + "snapshots/" + string(snapshotID) + "/search-data/"
	dataBase := publicationBase + "snapshots/" + string(snapshotID) + "/projection-data/"
	return DescriptorV1{
		SchemaVersion:         1,
		CatalogID:             "kubernetes",
		PublicationKey:        "public-kubernetes",
		PublicationBase:       publicationBase,
		SnapshotID:            string(snapshotID),
		RevisionID:            identity.RevisionID,
		ProjectionFormat:      "projection-v2",
		ProjectionDigest:      digestHex,
		ProjectionManifestURL: manifestURL,
		CatalogURL:            catalogURL,
		SearchDataBase:        searchDataBase,
		ProjectionDataBase:    dataBase,
	}, manifestBytes
}

func mutateManifestChild(mutate func(*catalog.ChildIdentityV1)) func(*DescriptorV1, *[]byte) {
	return func(_ *DescriptorV1, manifestBytes *[]byte) {
		var manifest catalog.ManifestV1
		if err := json.Unmarshal(*manifestBytes, &manifest); err != nil {
			panic(err)
		}
		mutate(&manifest.Children[1])
		mutate(&manifest.Identity.Children[1])
		encoded, err := json.Marshal(manifest)
		if err != nil {
			panic(err)
		}
		*manifestBytes = encoded
	}
}

func mutateDescriptorBase(publicationBase string) func(*DescriptorV1, *[]byte) {
	return func(descriptor *DescriptorV1, _ *[]byte) {
		descriptor.PublicationBase = publicationBase
		descriptor.ProjectionManifestURL = publicationBase + "snapshots/" + descriptor.SnapshotID + "/manifest.json"
		descriptor.ProjectionDataBase = publicationBase + "snapshots/" + descriptor.SnapshotID + "/projection-data/"
	}
}

func mutateManifestAndDescriptor(mutate func(*catalog.ChildIdentityV1)) func(*DescriptorV1, *[]byte) {
	return func(descriptor *DescriptorV1, manifestBytes *[]byte) {
		var manifest catalog.ManifestV1
		if err := json.Unmarshal(*manifestBytes, &manifest); err != nil {
			panic(err)
		}
		mutate(&manifest.Children[1])
		mutate(&manifest.Identity.Children[1])
		identityBytes, err := json.Marshal(manifest.Identity)
		if err != nil {
			panic(err)
		}
		digest := sha256.Sum256(identityBytes)
		descriptor.ProjectionDigest = hex.EncodeToString(digest[:])
		descriptor.SnapshotID = "snapshot-sha256-" + descriptor.ProjectionDigest
		descriptor.ProjectionManifestURL = descriptor.PublicationBase + "snapshots/" + descriptor.SnapshotID + "/manifest.json"
		descriptor.ProjectionDataBase = descriptor.PublicationBase + "snapshots/" + descriptor.SnapshotID + "/projection-data/"
		manifest.SnapshotID = catalog.SnapshotID(descriptor.SnapshotID)
		encoded, err := json.Marshal(manifest)
		if err != nil {
			panic(err)
		}
		*manifestBytes = encoded
	}
}

func mutateManifestEnvelope(mutate func(*catalog.ManifestV1)) func(*DescriptorV1, *[]byte) {
	return func(_ *DescriptorV1, manifestBytes *[]byte) {
		var manifest catalog.ManifestV1
		if err := json.Unmarshal(*manifestBytes, &manifest); err != nil {
			panic(err)
		}
		mutate(&manifest)
		encoded, err := json.Marshal(manifest)
		if err != nil {
			panic(err)
		}
		*manifestBytes = encoded
	}
}
