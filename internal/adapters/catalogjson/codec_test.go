package catalogjson

import (
	"bytes"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestCodecCanonicalRoundTrips(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		encode func() ([]byte, error)
		decode func([]byte) error
	}{
		"catalog": {
			encode: func() ([]byte, error) { return EncodeCatalog(codecCatalog()) },
			decode: func(data []byte) error { _, err := DecodeCatalog(data); return err },
		},
		"detail": {
			encode: func() ([]byte, error) { return EncodeDetailShard(codecDetailShard()) },
			decode: func(data []byte) error { _, err := DecodeDetailShard(data); return err },
		},
		"schema node": {
			encode: func() ([]byte, error) { return EncodeSchemaNodeShard(codecSchemaNodeShard()) },
			decode: func(data []byte) error { _, err := DecodeSchemaNodeShard(data); return err },
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, err := test.encode()
			if err != nil {
				t.Fatal(err)
			}
			if bytes.ContainsAny(data, "\n\r\t") {
				t.Fatalf("canonical bytes contain whitespace: %q", data)
			}
			if err := test.decode(data); err != nil {
				t.Fatal(err)
			}
			for mutation, changed := range map[string][]byte{
				"leading whitespace": append([]byte(" "), data...),
				"trailing value":     append(append([]byte(nil), data...), []byte("{}")...),
				"unknown field":      append(append([]byte(nil), data[:len(data)-1]...), []byte(`,"unknown":true}`)...),
				"invalid utf8":       append(append([]byte(nil), data...), 0xff),
			} {
				if err := test.decode(changed); err == nil {
					t.Fatalf("%s mutation was accepted", mutation)
				}
			}
		})
	}
}

func TestCodecRejectsDuplicateKeysAndOversizedIntegers(t *testing.T) {
	t.Parallel()

	for name, data := range map[string][]byte{
		"duplicate": []byte(`{"schemaVersion":1,"schemaVersion":1,"catalogId":"catalog","title":"Catalog","branding":{"displayName":"","logoSrc":"","logoAlt":"","logoHomeHref":"","faviconHref":""},"defaultDocumentKey":"doc","profileId":"strict-v1","documents":[]}`),
		"integer":   []byte(`{"schemaVersion":18446744073709551616,"schemaVersion":1}`),
	} {
		if _, err := DecodeCatalog(data); err == nil {
			t.Fatalf("%s JSON was accepted", name)
		}
	}
}

func TestCodecRejectsSemanticDanglingAndMalformedRecords(t *testing.T) {
	t.Parallel()

	catalogValue := codecCatalog()
	manifest := catalog.ManifestV1{Children: []catalog.ChildIdentityV1{{Path: "catalog.json", Kind: "catalog", Length: 1, SHA256: strings.Repeat("a", 64)}}}
	if err := ValidateCatalogManifest(catalogValue, manifest); err == nil {
		t.Fatal("dangling catalog child references were accepted")
	}

	detail := codecDetailShard()
	detail.Records[0].Schema = &projection.SchemaDetail{}
	if _, err := EncodeDetailShard(detail); err == nil {
		t.Fatal("detail record with two payloads was accepted")
	}

	nodes := codecSchemaNodeShard()
	nodes.Nodes[0].Ordinal = 2
	if _, err := EncodeSchemaNodeShard(nodes); err == nil {
		t.Fatal("noncontiguous schema-node ordinal was accepted")
	}
}

func codecCatalog() catalog.CatalogArtifactV1 {
	id := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	return catalog.CatalogArtifactV1{
		SchemaVersion: 1, CatalogID: "catalog", Title: "Catalog", DefaultDocumentKey: "doc", ProfileID: domain.CompatibilityProfileStrict,
		Documents: []catalog.DocumentDirectoryV1{{
			Key: "doc", SourcePath: "openapi.json", Title: "Doc", APIVersion: "v1", SourceChild: "sources/doc.json",
			SchemaNodeShards: []catalog.ShardReferenceV1{{Path: "schema-nodes/doc/" + strings.Repeat("b", 64) + ".json", FirstOrdinal: 0, LastOrdinal: 0, Records: 1, Length: 1, SHA256: strings.Repeat("b", 64)}},
			Operations:       []catalog.OperationDirectoryV1{{DetailID: id, Method: "GET", Path: "/pets", Title: "Pets", Href: "documents/doc/?selected=" + string(id) + "#" + string(id), DetailChild: "details/doc/" + strings.Repeat("c", 64) + ".json", Tags: []string{}, Facets: []catalog.FacetV1{}}},
			Schemas:          []catalog.SchemaDirectoryV1{},
		}},
	}
}

func codecDetailShard() catalog.DetailShardV1 {
	id := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	return catalog.DetailShardV1{SchemaVersion: 1, DocumentKey: "doc", Records: []catalog.DetailRecordV1{{
		ID: id, Kind: "operation", Operation: &projection.OperationDetail{ID: string(id), Anchor: string(id), HeadingID: string(id), Method: "GET", Path: "/pets"},
	}}}
}

func codecSchemaNodeShard() catalog.SchemaNodeShardV1 {
	return catalog.SchemaNodeShardV1{SchemaVersion: 1, DocumentKey: "doc", FirstOrdinal: 0, Nodes: []projection.SchemaNode{{Ordinal: 0, ID: "node", Name: "Pet"}}}
}
