//go:build !manja_runtime

package selfhosted

import (
	"encoding/json"
	"testing"

	"github.com/araihu/manja/application/catalog"
	artifact "github.com/araihu/manja/application/htmlartifact"
)

func BenchmarkExportDetailIdentity(b *testing.B) {
	document := catalog.DocumentDirectoryV1{Key: "doc", Operations: make([]catalog.OperationDirectoryV1, 2000)}
	for i := range document.Operations {
		document.Operations[i] = catalog.OperationDirectoryV1{Method: "GET", Path: "/pets", Title: "List pets", Tags: []string{"Pets"}}
	}
	child := catalog.ChildIdentityV1{Path: "details/doc.json", Kind: "detail", SHA256: "child"}
	dependencies, err := prepareExportDetailDependencies(document, "/pets/", "compiler")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload, err := dependencies.payload(child)
		if err != nil {
			b.Fatal(err)
		}
		_ = artifact.PayloadSHA256(payload)
	}
}

func TestExportDetailDependenciesInvalidateCompleteDocument(t *testing.T) {
	document := catalog.DocumentDirectoryV1{Key: "doc", Title: "Pets", Operations: []catalog.OperationDirectoryV1{{Title: "first", Tags: []string{"Pets"}}, {Title: "second"}}, Schemas: []catalog.SchemaDirectoryV1{{Name: "Pet"}}, SchemaNodeShards: []catalog.ShardReferenceV1{{Path: "nodes", SHA256: "nodes-hash"}}}
	child := catalog.ChildIdentityV1{Path: "details", Kind: "detail", SHA256: "detail-hash", Length: 12}
	key := func(document catalog.DocumentDirectoryV1, child catalog.ChildIdentityV1, base, compiler, normalizer, binary string) artifact.BuildKey {
		t.Helper()
		dependencies, err := prepareExportDetailDependencies(document, base, compiler)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := dependencies.payload(child)
		if err != nil {
			t.Fatal(err)
		}
		result, err := artifact.NewBuildKey(artifact.BuildKeyInput{Fragment: artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentOperation, Resource: "detail"}, CanonicalPayloadSHA256: artifact.PayloadSHA256(payload), CompilerIdentity: dependencies.compilerIdentity, NormalizerIdentity: normalizer, ManjaVersion: binary, RendererFingerprint: binary, UIFingerprint: binary})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	original := key(document, child, "/pets/", "compiler", "normalizer", "binary")
	for name, mutate := range map[string]func(*catalog.DocumentDirectoryV1){
		"title":           func(d *catalog.DocumentDirectoryV1) { d.Title = "changed" },
		"route":           func(d *catalog.DocumentDirectoryV1) { d.Key = "changed" },
		"version":         func(d *catalog.DocumentDirectoryV1) { d.APIVersion = "changed" },
		"source":          func(d *catalog.DocumentDirectoryV1) { d.SourceChild = "changed" },
		"operation title": func(d *catalog.DocumentDirectoryV1) { d.Operations[0].Title = "changed" },
		"operation tags":  func(d *catalog.DocumentDirectoryV1) { d.Operations[0].Tags[0] = "changed" },
		"operation href":  func(d *catalog.DocumentDirectoryV1) { d.Operations[0].Href = "changed" },
		"navigation order": func(d *catalog.DocumentDirectoryV1) {
			d.Operations[0], d.Operations[1] = d.Operations[1], d.Operations[0]
		},
		"schema inventory": func(d *catalog.DocumentDirectoryV1) { d.Schemas[0].Name = "changed" },
		"schema reference": func(d *catalog.DocumentDirectoryV1) { d.SchemaNodeShards[0].SHA256 = "changed" },
		"security": func(d *catalog.DocumentDirectoryV1) {
			d.SecuritySchemes = []catalog.SecuritySchemeDirectoryV1{{Name: "changed"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			data, _ := json.Marshal(document)
			var changed catalog.DocumentDirectoryV1
			if err := json.Unmarshal(data, &changed); err != nil {
				t.Fatal(err)
			}
			mutate(&changed)
			if key(changed, child, "/pets/", "compiler", "normalizer", "binary") == original {
				t.Fatal("dependency change reused build key")
			}
		})
	}
	for _, values := range [][4]string{{"/changed/", "compiler", "normalizer", "binary"}, {"/pets/", "changed", "normalizer", "binary"}, {"/pets/", "compiler", "changed", "binary"}, {"/pets/", "compiler", "normalizer", "changed"}} {
		if key(document, child, values[0], values[1], values[2], values[3]) == original {
			t.Fatal("toolchain/base change reused build key")
		}
	}
	changedChild := child
	changedChild.SHA256 = "changed"
	if key(document, changedChild, "/pets/", "compiler", "normalizer", "binary") == original {
		t.Fatal("child change reused build key")
	}
	if key(document, child, "/pets/", "compiler", "normalizer", "binary") != original {
		t.Fatal("nondeterministic build key")
	}
	dependencies, err := prepareExportDetailDependencies(document, "/pets/", "compiler")
	if err != nil {
		t.Fatal(err)
	}
	newPayload, err := dependencies.payload(child)
	if err != nil {
		t.Fatal(err)
	}
	oldPayload, err := json.Marshal(struct {
		Document        catalog.DocumentDirectoryV1 `json:"document"`
		Child           catalog.ChildIdentityV1     `json:"child"`
		Schemas         []catalog.ShardReferenceV1  `json:"schemas"`
		PublicationBase string                      `json:"publicationBase"`
	}{document, child, document.SchemaNodeShards, "/pets/"})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.PayloadSHA256(oldPayload) == artifact.PayloadSHA256(newPayload) {
		t.Fatal("old identity scheme reused")
	}
}
