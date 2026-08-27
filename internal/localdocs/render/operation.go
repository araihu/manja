package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

var errInvalidOperationHeaderFragment = errors.New("local docs operation-header fragment is invalid")

type OperationHeaderFragment struct {
	data  operationHeaderData
	valid bool
}

type operationHeaderData struct {
	Anchor      string
	Title       string
	Method      string
	Path        string
	Description string
	Deprecated  bool
}

func PrepareOperationHeader(detail catalog.DetailRecordV1, operation domain.Operation, documentHref string) (OperationHeaderFragment, error) {
	if !validDocumentHref(documentHref) {
		return OperationHeaderFragment{}, invalidOperationField("document href")
	}
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) {
		return OperationHeaderFragment{}, invalidOperationField("operation detail")
	}
	projected := detail.Operation
	id := string(detail.ID)
	if projected.ID != id || projected.Anchor != id || projected.HeadingID != id || projected.HeadingLevel == 0 || strings.TrimSpace(projected.Heading) == "" || !validOperationMethod(projected.Method) || !validOperationPath(projected.Path) {
		return OperationHeaderFragment{}, invalidOperationField("operation detail identity")
	}
	documentIndex := strings.LastIndex(documentHref, "/documents/")
	if documentIndex < 0 {
		return OperationHeaderFragment{}, invalidOperationField("document href route")
	}
	wantHref := documentHref[documentIndex+1:] + "?selected=" + id + "#" + id
	if projected.Href != wantHref {
		return OperationHeaderFragment{}, invalidOperationField("operation detail href")
	}
	if operation.Anchor != projected.Anchor || operation.Title != projected.Heading || operation.Method != projected.Method || operation.Path != projected.Path || operation.Summary != projected.Summary || operation.Description != projected.Description || operation.Deprecated != projected.Deprecated {
		return OperationHeaderFragment{}, invalidOperationField("prepared operation")
	}
	return OperationHeaderFragment{data: operationHeaderData{
		Anchor: operation.Anchor, Title: operationTitle(operation), Method: operation.Method,
		Path: operation.Path, Description: operation.Description, Deprecated: operation.Deprecated,
	}, valid: true}, nil
}

func validOperationMethod(value string) bool {
	if domain.ValidateCanonicalIdentity("operation method", value, false) != nil {
		return false
	}
	for _, character := range strings.ToUpper(value) {
		if character < 'A' || character > 'Z' {
			return false
		}
	}
	return true
}

func validOperationPath(value string) bool {
	if domain.ValidateCanonicalIdentity("operation path", value, false) != nil {
		return false
	}
	cleanInput := value
	if value != "/" && strings.HasSuffix(value, "/") {
		cleanInput = strings.TrimSuffix(value, "/")
	}
	return strings.HasPrefix(value, "/") && path.Clean(cleanInput) == cleanInput && !strings.ContainsAny(value, " ?#")
}

func OperationHeader(fragment OperationHeaderFragment, actions, provenance templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		if !fragment.valid {
			return errInvalidOperationHeaderFragment
		}
		var output boundedBuffer
		if err := operationHeader(fragment.data, actions, provenance).Render(ctx, &output); err != nil {
			return err
		}
		_, err := writer.Write(output.Bytes())
		return err
	})
}

func (fragment OperationHeaderFragment) Bytes(ctx context.Context, actions, provenance templ.Component) ([]byte, error) {
	var output bytes.Buffer
	if err := OperationHeader(fragment, actions, provenance).Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func operationTitle(operation domain.Operation) string {
	for _, value := range []string{operation.Title, operation.Summary, operation.ID} {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return strings.TrimSpace(operation.Method + " " + operation.Path)
}

func invalidOperationField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationHeaderFragment, name)
}
