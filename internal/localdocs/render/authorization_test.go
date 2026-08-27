package render

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/application/projection"
	"github.com/araihu/manja/domain"
)

func TestPreparedOperationAuthorizationRendersCanonicalEscapedHTML(t *testing.T) {
	detail, operation := operationAuthorizationFixture()
	fragment, err := PrepareOperationAuthorization(detail, operation)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`data-manja-authorization`,
		`aria-label="Authorization requirements"`,
		`Authorization`,
		`Read-only`,
		`Credentials are never accepted or stored on this documentation page.`,
		`data-manja-security-requirement="oauth-primary"`,
		`oauth2`,
		`required`,
		`Use &lt;token&gt;.`,
		`Bearer`,
		`JWT`,
		`https://auth.example.test/.well-known/openid-configuration`,
		`pods:read`,
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("authorization fragment missing %q:\n%s", want, body)
		}
	}
	if bytes.Contains(body, []byte("Use <token>.")) {
		t.Fatalf("authorization fragment contains unescaped projection content: %s", body)
	}
}

func TestPrepareOperationAuthorizationFailsClosedOnInconsistentInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*catalog.DetailRecordV1, *domain.Operation)
	}{
		{name: "schema detail", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Kind, detail.Operation, detail.Schema = "schema", nil, &projection.SchemaDetail{}
		}},
		{name: "detail identity", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.ID = "changed"
		}},
		{name: "operation anchor", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation) {
			operation.Anchor = "changed"
		}},
		{name: "security inventory", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation) {
			operation.Security = nil
		}},
		{name: "security ordinal", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.Security[0].Ordinal = 1
		}},
		{name: "security id", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.Security[0].ID = "changed"
		}},
		{name: "duplicate security identity", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation) {
			duplicate := detail.Operation.Security[0]
			duplicate.Ordinal = 1
			detail.Operation.Security = append(detail.Operation.Security, duplicate)
			operation.Security = append(operation.Security, operation.Security[0])
		}},
		{name: "scope ordinal", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.Security[0].Scopes[0].Ordinal = 1
		}},
		{name: "scope id", mutate: func(detail *catalog.DetailRecordV1, _ *domain.Operation) {
			detail.Operation.Security[0].Scopes[0].ID = "changed"
		}},
		{name: "duplicate scope", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation) {
			duplicate := detail.Operation.Security[0].Scopes[0]
			duplicate.Ordinal = 1
			detail.Operation.Security[0].Scopes = append(detail.Operation.Security[0].Scopes, duplicate)
			operation.Security[0].Scopes = append(operation.Security[0].Scopes, operation.Security[0].Scopes[0])
		}},
		{name: "prepared name", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation) {
			operation.Security[0].Name = "changed"
		}},
		{name: "prepared scope", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation) {
			operation.Security[0].Scopes[0] = "changed"
		}},
		{name: "prepared definition", mutate: func(_ *catalog.DetailRecordV1, operation *domain.Operation) {
			operation.Security[0].Definition.Scheme = "Basic"
		}},
		{name: "invalid UTF-8 description", mutate: func(detail *catalog.DetailRecordV1, operation *domain.Operation) {
			detail.Operation.Security[0].Definition.Description = string([]byte{0xff})
			operation.Security[0].Definition.Description = detail.Operation.Security[0].Definition.Description
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			detail, operation := operationAuthorizationFixture()
			test.mutate(&detail, &operation)
			fragment, err := PrepareOperationAuthorization(detail, operation)
			if err == nil || !reflect.DeepEqual(fragment, OperationAuthorizationFragment{}) {
				t.Fatalf("inconsistent authorization prepared: fragment=%#v err=%v", fragment, err)
			}
		})
	}
}

func TestPrepareOperationAuthorizationRejectsRecordCountAboveLimit(t *testing.T) {
	detail, operation := operationAuthorizationFixture()
	detail.Operation.Security[0].Scopes = make([]projection.TextRecord, maximumOperationAuthorizationRecords)
	operation.Security[0].Scopes = make([]string, maximumOperationAuthorizationRecords)
	fragment, err := PrepareOperationAuthorization(detail, operation)
	if err == nil || !reflect.DeepEqual(fragment, OperationAuthorizationFragment{}) {
		t.Fatalf("oversized record inventory prepared: fragment=%#v err=%v", fragment, err)
	}
}

func TestPreparedOperationAuthorizationCopiesInputsAndRendersDeterministically(t *testing.T) {
	detail, operation := operationAuthorizationFixture()
	fragment, err := PrepareOperationAuthorization(detail, operation)
	if err != nil {
		t.Fatal(err)
	}
	detail.Operation.Security[0].Definition.Description = "changed"
	detail.Operation.Security[0].Scopes[0].Value = "changed"
	operation.Security[0].Definition.Description = "changed"
	operation.Security[0].Scopes[0] = "changed"

	first, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || bytes.Contains(first, []byte("changed")) {
		t.Fatalf("authorization aliases inputs or is nondeterministic:\nfirst=%s\nsecond=%s", first, second)
	}
	first[0] = 'x'
	third, err := fragment.Bytes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(second, third) {
		t.Fatal("rendered bytes alias fragment state")
	}
}

func TestPreparedOperationAuthorizationRejectsOversizedEscapedOutputWithoutPartialBytes(t *testing.T) {
	detail, operation := operationAuthorizationFixture()
	detail.Operation.Security[0].Definition.Description = strings.Repeat("<", maximumHTMLFragmentBytes/2)
	operation.Security[0].Definition.Description = detail.Operation.Security[0].Definition.Description
	fragment, err := PrepareOperationAuthorization(detail, operation)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fragment.Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("oversized authorization = bytes=%d err=%v", len(body), err)
	}
}

func TestZeroOperationAuthorizationFragmentFailsWithoutBytes(t *testing.T) {
	body, err := (OperationAuthorizationFragment{}).Bytes(context.Background())
	if err == nil || body != nil {
		t.Fatalf("zero authorization = bytes=%d err=%v", len(body), err)
	}
}

func operationAuthorizationFixture() (catalog.DetailRecordV1, domain.Operation) {
	detailID := domain.DetailID("detail-sha256-" + strings.Repeat("a", 64))
	projected := projection.SecurityRequirement{
		Ordinal: 0, ID: "oauth-primary", Name: "oauth-primary",
		Scopes: []projection.TextRecord{{Ordinal: 0, ID: operationAuthorizationRecordID("scope", "pods:read"), Value: "pods:read"}},
		Definition: projection.SecurityScheme{
			Name: "oauth-primary", Type: "oauth2", Description: "Use <token>.",
			Scheme: "Bearer", BearerFormat: "JWT",
			OpenIDConnectURL: "https://auth.example.test/.well-known/openid-configuration",
		},
	}
	detail := catalog.DetailRecordV1{ID: detailID, Kind: "operation", Operation: &projection.OperationDetail{
		ID: string(detailID), Anchor: string(detailID), HeadingID: string(detailID), Heading: "List Pods", HeadingLevel: 2,
		Method: "GET", Path: "/api/v1/pods", Security: []projection.SecurityRequirement{projected},
	}}
	operation := domain.Operation{Anchor: string(detailID), Title: "List Pods", Method: "GET", Path: "/api/v1/pods", Security: []domain.OperationSecurity{{
		Name: projected.Name, Scopes: []string{"pods:read"}, Definition: domain.SecurityScheme{
			Name: projected.Definition.Name, Type: projected.Definition.Type, Description: projected.Definition.Description,
			Scheme: projected.Definition.Scheme, BearerFormat: projected.Definition.BearerFormat,
			OpenIDConnectURL: projected.Definition.OpenIDConnectURL,
		},
	}}}
	return detail, operation
}
