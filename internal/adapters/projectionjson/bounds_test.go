package projectionjson

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestMarshalEnforcesInternedRecordNodeAndDocumentBounds(t *testing.T) {
	t.Run("operation", func(t *testing.T) {
		document := mustBuild(t, operationFixture())
		assertRecordBoundary(t, &document, 256*1024, func(filler string) {
			document.OperationDetails[0].Description = filler
		}, func() any { return document.OperationDetails[0] })
	})
	t.Run("schema", func(t *testing.T) {
		document := mustBuild(t, fullFixture())
		assertRecordBoundary(t, &document, 512*1024, func(filler string) {
			document.SchemaDetails[0].Description = filler
		}, func() any { return document.SchemaDetails[0] })
	})
	t.Run("schema node", func(t *testing.T) {
		for _, size := range []int{maxSchemaNodeBytes - 1, maxSchemaNodeBytes, maxSchemaNodeBytes + 1} {
			document, recordBytes := documentWithSchemaNodeSize(t, size)
			encoded, err := Marshal(document)
			if size <= maxSchemaNodeBytes && (err != nil || encoded == nil || recordBytes != size) {
				t.Fatalf("schema node size %d rejected or changed: record=%d err=%v", size, recordBytes, err)
			}
			if size > maxSchemaNodeBytes && (err == nil || encoded != nil) {
				t.Fatalf("schema node size %d accepted", size)
			}
		}
	})
	t.Run("document", func(t *testing.T) {
		for _, size := range []int{maxProjectionBytes - 1, maxProjectionBytes, maxProjectionBytes + 1} {
			document := documentWithCanonicalSize(t, size)
			encoded, err := Marshal(document)
			if size <= maxProjectionBytes && (err != nil || len(encoded) != size) {
				t.Fatalf("document size %d rejected or changed: len=%d err=%v", size, len(encoded), err)
			}
			if size > maxProjectionBytes && (err == nil || encoded != nil) {
				t.Fatalf("document size %d accepted", size)
			}
		}
	})
}

func documentWithSchemaNodeSize(t *testing.T, target int) (projection.Document, int) {
	t.Helper()
	low, high := 0, target
	for low <= high {
		middle := low + (high-low)/2
		input := emptyFixture()
		input.Schemas = []domain.Schema{{
			Name: "Bounded", Summary: domain.SchemaSummary{Type: "string", Description: strings.Repeat("x", middle)},
		}}
		document := mustBuild(t, input)
		record, err := json.Marshal(document.SchemaNodes[document.SchemaDetails[0].SchemaRef])
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case len(record) < target:
			low = middle + 1
		case len(record) > target:
			high = middle - 1
		default:
			return document, len(record)
		}
	}
	t.Fatalf("cannot construct schema node of %d bytes", target)
	return projection.Document{}, 0
}

func TestUnmarshalPreflightUsesActualInputLength(t *testing.T) {
	input := make([]byte, maxProjectionBytes+1)
	for index := range input {
		input[index] = 'x'
	}
	if document, err := Unmarshal(input); err == nil || document.FormatVersion != 0 {
		t.Fatalf("oversized input accepted: %#v", document)
	}
}

func assertRecordBoundary(t *testing.T, document *projection.Document, limit int, set func(string), record func() any) {
	t.Helper()
	find := func(target int) string {
		low, high := 0, target
		for low <= high {
			middle := low + (high-low)/2
			set(strings.Repeat("x", middle))
			bytes, err := json.Marshal(record())
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case len(bytes) < target:
				low = middle + 1
			case len(bytes) > target:
				high = middle - 1
			default:
				return strings.Repeat("x", middle)
			}
		}
		t.Fatalf("cannot construct record of %d bytes", target)
		return ""
	}
	for _, size := range []int{limit - 1, limit, limit + 1} {
		set(find(size))
		bytes, err := Marshal(*document)
		if size <= limit && (err != nil || bytes == nil) {
			t.Fatalf("record size %d rejected: %v", size, err)
		}
		if size > limit && (err == nil || bytes != nil) {
			t.Fatalf("record size %d accepted", size)
		}
	}
}

func documentWithCanonicalSize(t *testing.T, target int) projection.Document {
	t.Helper()
	document := mustBuild(t, emptyFixture())
	low, high := 0, target
	for low <= high {
		middle := low + (high-low)/2
		document.Title = strings.Repeat("x", middle)
		encoded, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case len(encoded) < target:
			low = middle + 1
		case len(encoded) > target:
			high = middle - 1
		default:
			return document
		}
	}
	t.Fatalf("cannot construct document of %d bytes", target)
	return projection.Document{}
}
