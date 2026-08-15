package render

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

const maximumOperationAuthorizationRecords = 4096

var errInvalidOperationAuthorizationFragment = errors.New("local docs operation authorization fragment is invalid")

// OperationAuthorizationFragment owns admitted authorization display data.
// It renders documentation only; credentials and request activation remain outside this fragment.
type OperationAuthorizationFragment struct {
	requirements []operationAuthorizationData
	binding      operationPreparationBinding
	valid        bool
}

type operationAuthorizationData struct {
	Name             string
	TypeLabel        string
	Instruction      string
	Description      string
	ParameterLabel   string
	ParameterName    string
	Scheme           string
	BearerFormat     string
	OpenIDConnectURL string
	Scopes           []string
}

func PrepareOperationAuthorization(detail catalog.DetailRecordV1, operation domain.Operation) (OperationAuthorizationFragment, error) {
	if detail.Kind != "operation" || detail.Operation == nil || detail.Schema != nil || !validDetailID(detail.ID) {
		return OperationAuthorizationFragment{}, invalidOperationAuthorizationField("operation")
	}
	projected := detail.Operation
	if projected.ID != string(detail.ID) || projected.Anchor != string(detail.ID) || operation.Anchor != projected.Anchor {
		return OperationAuthorizationFragment{}, invalidOperationAuthorizationField("operation identity")
	}
	if len(projected.Security) != len(operation.Security) || len(projected.Security) > maximumOperationAuthorizationRecords {
		return OperationAuthorizationFragment{}, invalidOperationAuthorizationField("security inventory")
	}

	fragment := OperationAuthorizationFragment{requirements: make([]operationAuthorizationData, 0, len(projected.Security)), valid: true}
	seen := make(map[string]struct{}, len(projected.Security))
	records := len(projected.Security)
	preparedBytes := 0
	for requirementIndex, requirement := range projected.Security {
		prepared := operation.Security[requirementIndex]
		if requirement.Ordinal != uint32(requirementIndex) || requirement.ID != requirement.Name {
			return OperationAuthorizationFragment{}, invalidOperationAuthorizationField("security identity")
		}
		if domain.ValidateCanonicalIdentity("security name", requirement.Name, false) != nil {
			return OperationAuthorizationFragment{}, invalidOperationAuthorizationField("security name")
		}
		if _, duplicate := seen[requirement.ID]; duplicate {
			return OperationAuthorizationFragment{}, invalidOperationAuthorizationField("duplicate security identity")
		}
		seen[requirement.ID] = struct{}{}
		if records > maximumOperationAuthorizationRecords-len(requirement.Scopes) {
			return OperationAuthorizationFragment{}, invalidOperationAuthorizationField("security record limit")
		}
		records += len(requirement.Scopes)

		scopes := make([]string, 0, len(requirement.Scopes))
		scopeIDs := make(map[string]struct{}, len(requirement.Scopes))
		for scopeIndex, scope := range requirement.Scopes {
			if scope.Ordinal != uint32(scopeIndex) || scope.ID != operationAuthorizationRecordID("scope", scope.Value) || domain.ValidateCanonicalIdentity("security scope", scope.Value, false) != nil {
				return OperationAuthorizationFragment{}, invalidOperationAuthorizationField("security scope")
			}
			if _, duplicate := scopeIDs[scope.ID]; duplicate {
				return OperationAuthorizationFragment{}, invalidOperationAuthorizationField("duplicate security scope")
			}
			scopeIDs[scope.ID] = struct{}{}
			scopes = append(scopes, scope.Value)
		}

		converted := domain.OperationSecurity{
			Name: requirement.Name, Scopes: scopes,
			Definition: domain.SecurityScheme{
				Name: requirement.Definition.Name, Type: requirement.Definition.Type,
				Description: requirement.Definition.Description, ParameterName: requirement.Definition.ParameterName,
				In: requirement.Definition.In, Scheme: requirement.Definition.Scheme,
				BearerFormat: requirement.Definition.BearerFormat, OpenIDConnectURL: requirement.Definition.OpenIDConnectURL,
			},
		}
		if !operationAuthorizationEqual(converted, prepared) || !validOperationAuthorizationStrings(converted) {
			return OperationAuthorizationFragment{}, invalidOperationAuthorizationField("prepared security")
		}
		parameterName := operationAuthorizationParameterName(prepared)
		data := operationAuthorizationData{
			Name: prepared.Name, TypeLabel: operationAuthorizationTypeLabel(prepared),
			Instruction: operationAuthorizationInstruction(prepared), Description: prepared.Definition.Description,
			ParameterName: parameterName,
			Scopes:        append([]string(nil), prepared.Scopes...),
		}
		if parameterName != "" {
			data.ParameterLabel = operationAuthorizationParameterLocation(prepared)
		}
		if strings.TrimSpace(prepared.Definition.Scheme) != "" {
			data.Scheme = prepared.Definition.Scheme
		}
		if strings.TrimSpace(prepared.Definition.BearerFormat) != "" {
			data.BearerFormat = prepared.Definition.BearerFormat
		}
		if strings.TrimSpace(prepared.Definition.OpenIDConnectURL) != "" {
			data.OpenIDConnectURL = prepared.Definition.OpenIDConnectURL
		}
		if !admitOperationAuthorizationBytes(&preparedBytes, data) {
			return OperationAuthorizationFragment{}, invalidOperationAuthorizationField("prepared byte limit")
		}
		fragment.requirements = append(fragment.requirements, data)
	}
	parentBinding, err := bindOperationPreparationParent(detail, operation)
	if err != nil {
		return OperationAuthorizationFragment{}, invalidOperationAuthorizationField("preparation context")
	}
	fragment.binding = operationPreparationBinding{parent: parentBinding}
	return fragment, nil
}

func operationAuthorizationEqual(left, right domain.OperationSecurity) bool {
	return left.Name == right.Name && left.Definition == right.Definition && slices.Equal(left.Scopes, right.Scopes)
}

func validOperationAuthorizationStrings(security domain.OperationSecurity) bool {
	values := []string{
		security.Name, security.Definition.Name, security.Definition.Type, security.Definition.Description,
		security.Definition.ParameterName, security.Definition.In, security.Definition.Scheme,
		security.Definition.BearerFormat, security.Definition.OpenIDConnectURL,
	}
	values = append(values, security.Scopes...)
	for _, value := range values {
		if !utf8.ValidString(value) {
			return false
		}
	}
	return true
}

func admitOperationAuthorizationBytes(total *int, data operationAuthorizationData) bool {
	values := []string{
		data.Name, data.TypeLabel, data.Instruction, data.Description, data.ParameterLabel,
		data.ParameterName, data.Scheme, data.BearerFormat, data.OpenIDConnectURL,
	}
	values = append(values, data.Scopes...)
	for _, value := range values {
		if len(value) > maximumHTMLFragmentBytes-*total {
			return false
		}
		*total += len(value)
	}
	return true
}

func operationAuthorizationRecordID(kind string, parts ...string) string {
	hash := sha256.New()
	hash.Write([]byte(kind))
	hash.Write([]byte{0})
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len([]byte(part))))
		hash.Write(length[:])
		hash.Write([]byte(part))
	}
	return kind + "-" + hex.EncodeToString(hash.Sum(nil))
}

func operationAuthorizationTypeLabel(security domain.OperationSecurity) string {
	if strings.TrimSpace(security.Definition.Type) != "" {
		return security.Definition.Type
	}
	return "security scheme"
}

func operationAuthorizationParameterName(security domain.OperationSecurity) string {
	if strings.TrimSpace(security.Definition.ParameterName) != "" {
		return security.Definition.ParameterName
	}
	typeName := strings.ToLower(strings.TrimSpace(security.Definition.Type))
	if typeName == "http" || typeName == "oauth2" || typeName == "openidconnect" {
		return "Authorization"
	}
	return ""
}

func operationAuthorizationParameterLocation(security domain.OperationSecurity) string {
	switch strings.ToLower(strings.TrimSpace(security.Definition.In)) {
	case "header":
		return "Request header"
	case "query":
		return "Query parameter"
	case "cookie":
		return "Cookie"
	}
	if operationAuthorizationParameterName(security) != "" {
		return "Request header"
	}
	return "Parameter"
}

func operationAuthorizationInstruction(security domain.OperationSecurity) string {
	typeName := strings.ToLower(strings.TrimSpace(security.Definition.Type))
	scheme := strings.ToLower(strings.TrimSpace(security.Definition.Scheme))
	parameter := operationAuthorizationParameterName(security)
	switch {
	case typeName == "http" && scheme == "bearer":
		return "Send the token in the Authorization request header using the Bearer scheme."
	case typeName == "http" && scheme == "basic":
		return "Send credentials in the Authorization request header using the Basic scheme."
	case typeName == "oauth2" || typeName == "openidconnect":
		return "Send the access token in the Authorization request header using the Bearer scheme."
	case typeName == "apikey" && strings.EqualFold(security.Definition.In, "header"):
		return "Send the API key in the " + parameter + " request header."
	case typeName == "apikey" && parameter != "":
		return "Send the API key through the documented " + strings.ToLower(operationAuthorizationParameterLocation(security)) + "."
	}
	return ""
}

func OperationAuthorization(fragment OperationAuthorizationFragment) templ.Component { return fragment }

func (fragment OperationAuthorizationFragment) Render(ctx context.Context, writer io.Writer) error {
	if !fragment.valid {
		return errInvalidOperationAuthorizationFragment
	}
	var output boundedBuffer
	if err := operationAuthorization(fragment.requirements).Render(ctx, &output); err != nil {
		return err
	}
	_, err := writer.Write(output.Bytes())
	return err
}

func (fragment OperationAuthorizationFragment) Bytes(ctx context.Context) ([]byte, error) {
	var output bytes.Buffer
	if err := fragment.Render(ctx, &output); err != nil {
		return nil, err
	}
	return append([]byte(nil), output.Bytes()...), nil
}

func invalidOperationAuthorizationField(name string) error {
	return fmt.Errorf("%w: %s", errInvalidOperationAuthorizationFragment, name)
}

var _ templ.Component = OperationAuthorizationFragment{}
