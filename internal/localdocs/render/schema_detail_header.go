package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

var errInvalidSchemaDetailHeaderFragment = errors.New("local docs schema-detail header fragment is invalid")

// SchemaDetailHeaderFragment owns the immutable identity and summary fields
// shown above a prepared schema-node body. Actions and provenance stay at the
// catalog composition boundary and are rendered through caller components.
type SchemaDetailHeaderFragment struct {
	data  schemaDetailHeaderData
	valid bool
}

type schemaDetailHeaderData struct {
	Version     string
	TypeLabel   string
	Title       string
	Description string
}

// PrepareSchemaDetailHeader binds the schema header to one verified detail,
// prepared schema, document identity, and optional prepared schema node. It
// copies every value consumed by the shared templ component.
func PrepareSchemaDetailHeader(
	detail catalog.DetailRecordV1,
	schema domain.Schema,
	document catalog.DocumentDirectoryV1,
	node *SchemaNodeFragment,
) (SchemaDetailHeaderFragment, error) {
	if detail.Kind != "schema" || detail.Schema == nil || detail.Operation != nil || !validDetailID(detail.ID) {
		return SchemaDetailHeaderFragment{}, invalidSchemaDetailHeaderField("schema detail")
	}
	projected := detail.Schema
	id := string(detail.ID)
	if projected.ID != id || projected.Anchor != id || projected.HeadingID != id || projected.HeadingLevel == 0 || strings.TrimSpace(projected.Heading) == "" {
		return SchemaDetailHeaderFragment{}, invalidSchemaDetailHeaderField("schema detail identity")
	}
	if domain.ValidateCatalogDocumentKey(document.Key) != nil {
		return SchemaDetailHeaderFragment{}, invalidSchemaDetailHeaderField("document key")
	}
	wantHref := "documents/" + document.Key + "/?selected=" + id + "#" + id
	if projected.Href != wantHref {
		return SchemaDetailHeaderFragment{}, invalidSchemaDetailHeaderField("schema detail href")
	}
	if schema.Name != projected.Heading || schema.Description != projected.Description {
		return SchemaDetailHeaderFragment{}, invalidSchemaDetailHeaderField("prepared schema")
	}
	for name, value := range map[string]string{
		"schema heading":     projected.Heading,
		"schema description": projected.Description,
		"document version":   document.APIVersion,
	} {
		if !utf8.ValidString(value) {
			return SchemaDetailHeaderFragment{}, invalidSchemaDetailHeaderField(name)
		}
	}
	version := strings.TrimSpace(document.APIVersion)
	if strings.EqualFold(version, "unversioned") {
		version = ""
	}
	typeLabel := ""
	if node != nil {
		if !node.valid {
			return SchemaDetailHeaderFragment{}, invalidSchemaDetailHeaderField("schema node")
		}
		for name, value := range map[string]string{
			"schema node type":   node.Type(),
			"schema node format": node.Format(),
		} {
			if !utf8.ValidString(value) || domain.ValidateCanonicalIdentity(name, value, true) != nil {
				return SchemaDetailHeaderFragment{}, invalidSchemaDetailHeaderField(name)
			}
		}
		typeLabel = strings.TrimSpace(node.Type() + " " + node.Format())
	}
	data := schemaDetailHeaderData{
		Version: version, TypeLabel: typeLabel,
		Title: projected.Heading, Description: projected.Description,
	}
	return SchemaDetailHeaderFragment{data: data, valid: true}, nil
}

// SchemaDetailHeader renders the admitted schema header with composition-owned
// action and provenance slots.
func SchemaDetailHeader(fragment SchemaDetailHeaderFragment, actions, provenance templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidSchemaDetailHeaderFragment
		}
		var output boundedBuffer
		if err := schemaDetailHeader(fragment.data, actions, provenance).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment SchemaDetailHeaderFragment) Bytes(ctx context.Context, actions, provenance templ.Component) ([]byte, error) {
	var output bytes.Buffer
	if err := SchemaDetailHeader(fragment, actions, provenance).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidSchemaDetailHeaderField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidSchemaDetailHeaderFragment, strings.TrimSpace(name))
}
