package abi

import (
	"strings"
	"testing"
)

func TestAdmitCopiesPublicDescriptorManifestAndProjectionAllowlist(t *testing.T) {
	descriptor, manifest := validActivationFixture()
	activation, err := Admit(descriptor, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if activation.CatalogID() != descriptor.CatalogID || activation.SnapshotID() != descriptor.SnapshotID || activation.RevisionID() != descriptor.RevisionID {
		t.Fatalf("activation identity = %#v", activation)
	}
	if activation.PublicationKey() != descriptor.PublicationKey || activation.ProjectionDigest() != descriptor.ProjectionDigest {
		t.Fatalf("activation cache identity = publication %q digest %q, want publication %q digest %q", activation.PublicationKey(), activation.ProjectionDigest(), descriptor.PublicationKey, descriptor.ProjectionDigest)
	}
	artifact, ok := activation.Artifact("details/core.json")
	if !ok || artifact.Kind != "detail" || artifact.Length != 7 || artifact.SHA256 != strings.Repeat("a", 64) {
		t.Fatalf("detail artifact = %#v, %t", artifact, ok)
	}
	if _, ok := activation.Artifact("details/unknown.json"); ok {
		t.Fatal("unknown projection path was allowlisted")
	}
	manifest.Children[0].Path = "details/mutated.json"
	if _, ok := activation.Artifact("details/core.json"); !ok {
		t.Fatal("activation aliases manifest children")
	}
}

func TestAdmitFailsClosedForDescriptorManifestAndChildMutations(t *testing.T) {
	descriptor, manifest := validActivationFixture()
	tests := []struct {
		name   string
		mutate func(*Descriptor, *Manifest)
	}{
		{name: "wrong snapshot", mutate: func(d *Descriptor, _ *Manifest) { d.SnapshotID = "snapshot-sha256-" + strings.Repeat("f", 64) }},
		{name: "wrong manifest identity", mutate: func(_ *Descriptor, m *Manifest) { m.Identity.RevisionID = "revision-other" }},
		{name: "wrong manifest digest", mutate: func(_ *Descriptor, m *Manifest) { m.Identity.ProjectionFormat = "projection-v1" }},
		{name: "missing identity digest", mutate: func(_ *Descriptor, m *Manifest) { m.IdentityDigest = "" }},
		{name: "too many children", mutate: func(_ *Descriptor, m *Manifest) { m.Children = make([]Artifact, maxProjectionChildren+1) }},
		{name: "backslash publication base", mutate: func(d *Descriptor, _ *Manifest) { d.PublicationBase = `/docs\/` }},
		{name: "duplicate child", mutate: func(_ *Descriptor, m *Manifest) { m.Children = append(m.Children, m.Children[0]) }},
		{name: "wrong child prefix", mutate: func(_ *Descriptor, m *Manifest) { m.Children[0].Path = "../details/core.json" }},
		{name: "backslash child path", mutate: func(_ *Descriptor, m *Manifest) { m.Children[0].Path = `details\core.json` }},
		{name: "wrong child kind", mutate: func(_ *Descriptor, m *Manifest) { m.Children[0].Kind = "catalog" }},
		{name: "invalid child digest", mutate: func(_ *Descriptor, m *Manifest) { m.Children[0].SHA256 = strings.Repeat("A", 64) }},
		{name: "oversized child", mutate: func(_ *Descriptor, m *Manifest) { m.Children[0].Length = maxProjectionChildBytes + 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d, m := cloneFixture(descriptor, manifest)
			test.mutate(&d, &m)
			activation, err := Admit(d, m)
			if err == nil {
				t.Fatalf("Admit succeeded with mutation: %#v", activation)
			}
			if activation.CatalogID() != "" || activation.PublicationKey() != "" || activation.SnapshotID() != "" || activation.RevisionID() != "" || activation.ProjectionDigest() != "" || len(activation.inventory) != 0 {
				t.Fatalf("failed admission returned state: %#v", activation)
			}
		})
	}
}

func TestActivationRouteAllowlistIsGetOnlyAndPublicationScoped(t *testing.T) {
	descriptor, manifest := validActivationFixture()
	activation, err := Admit(descriptor, manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		path string
		want bool
	}{
		{name: "declared detail", path: "/docs/snapshots/snapshot-sha256-" + strings.Repeat("b", 64) + "/projection-data/details/core.json", want: true},
		{name: "declared schema node", path: "/docs/snapshots/snapshot-sha256-" + strings.Repeat("b", 64) + "/projection-data/schema-nodes/core.json", want: true},
		{name: "other publication", path: "/other/snapshots/snapshot-sha256-" + strings.Repeat("b", 64) + "/projection-data/details/core.json", want: false},
		{name: "query", path: "/docs/snapshots/snapshot-sha256-" + strings.Repeat("b", 64) + "/projection-data/details/core.json?x=1", want: false},
		{name: "traversal", path: "/docs/snapshots/snapshot-sha256-" + strings.Repeat("b", 64) + "/projection-data/details/../schema-nodes/core.json", want: false},
		{name: "unknown child", path: "/docs/snapshots/snapshot-sha256-" + strings.Repeat("b", 64) + "/projection-data/details/unknown.json", want: false},
		{name: "backslash", path: `/docs/snapshots/snapshot-sha256-` + strings.Repeat("b", 64) + `/projection-data/details\core.json`, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := activation.Allows("GET", test.path); got != test.want {
				t.Fatalf("Allows(GET, %q) = %t, want %t", test.path, got, test.want)
			}
			if test.want && activation.Allows("POST", test.path) {
				t.Fatal("non-GET projection route was allowlisted")
			}
		})
	}
}

func TestActivationResolvesDeclaredProjectionRouteWithImmutableMetadata(t *testing.T) {
	descriptor, manifest := validActivationFixture()
	activation, err := Admit(descriptor, manifest)
	if err != nil {
		t.Fatal(err)
	}
	base := descriptor.ProjectionDataBase
	for _, test := range []struct {
		name    string
		method  string
		path    string
		want    Artifact
		allowed bool
	}{
		{name: "detail", method: "GET", path: base + "details/core.json", want: manifest.Children[0], allowed: true},
		{name: "schema node", method: "GET", path: base + "schema-nodes/core.json", want: manifest.Children[1], allowed: true},
		{name: "catalog is not projection", method: "GET", path: base + "catalog.json"},
		{name: "wrong method", method: "POST", path: base + "details/core.json"},
		{name: "unknown child", method: "GET", path: base + "details/unknown.json"},
		{name: "query", method: "GET", path: base + "details/core.json?x=1"},
		{name: "traversal", method: "GET", path: base + "details/../schema-nodes/core.json"},
		{name: "backslash", method: "GET", path: base + `details\core.json`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, allowed := activation.Resolve(test.method, test.path)
			if allowed != test.allowed {
				t.Fatalf("Resolve(%q, %q) allowed = %t, want %t", test.method, test.path, allowed, test.allowed)
			}
			if !allowed {
				if got != (Artifact{}) {
					t.Fatalf("Resolve(%q, %q) returned failed state %#v", test.method, test.path, got)
				}
				return
			}
			if got != test.want {
				t.Fatalf("Resolve(%q, %q) = %#v, want %#v", test.method, test.path, got, test.want)
			}
		})
	}
}

func validActivationFixture() (Descriptor, Manifest) {
	digest := strings.Repeat("a", 64)
	snapshot := "snapshot-sha256-" + strings.Repeat("b", 64)
	descriptor := Descriptor{
		SchemaVersion: 1, CatalogID: "core", PublicationKey: "public-core", PublicationBase: "/docs/",
		SnapshotID: snapshot, RevisionID: "revision-immutable-1", ProjectionFormat: "projection-v2", ProjectionDigest: strings.Repeat("b", 64),
		ProjectionManifestURL: "/docs/snapshots/" + snapshot + "/manifest.json", CatalogURL: "/docs/snapshots/" + snapshot + "/catalog.json",
		SearchDataBase: "/docs/snapshots/" + snapshot + "/search-data/", ProjectionDataBase: "/docs/snapshots/" + snapshot + "/projection-data/",
	}
	manifest := Manifest{
		SchemaVersion: 1, SnapshotID: snapshot,
		Identity:       Identity{SchemaVersion: 1, CatalogID: "core", RevisionID: "revision-immutable-1", ProjectionFormat: "projection-v2"},
		IdentityDigest: strings.Repeat("b", 64),
		Children: []Artifact{
			{Path: "details/core.json", Kind: "detail", Length: 7, SHA256: digest},
			{Path: "schema-nodes/core.json", Kind: "schema-node", Length: 8, SHA256: strings.Repeat("c", 64)},
			{Path: "catalog.json", Kind: "catalog", Length: 9, SHA256: strings.Repeat("d", 64)},
		},
	}
	return descriptor, manifest
}

func cloneFixture(descriptor Descriptor, manifest Manifest) (Descriptor, Manifest) {
	manifest.Children = append([]Artifact(nil), manifest.Children...)
	return descriptor, manifest
}
