package architecture_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestProjectionBuilderDependencyDirection(t *testing.T) {
	assertProjectionDependencies(t, modulePath+"/application/projection", map[string]bool{
		modulePath + "/application/projection": true,
		modulePath + "/domain":                 true,
	})
	assertProjectionDirectImports(t, "application/projection", map[string]bool{
		modulePath + "/domain": true,
	})
}

func TestProjectionCodecDependencyDirection(t *testing.T) {
	assertProjectionDependencies(t, modulePath+"/internal/adapters/projectionjson", map[string]bool{
		modulePath + "/application/projection":           true,
		modulePath + "/domain":                           true,
		modulePath + "/internal/adapters/projectionjson": true,
	})
	assertProjectionDirectImports(t, "internal/adapters/projectionjson", map[string]bool{
		modulePath + "/application/projection": true,
	})
}

func TestProjectionPublicAPIShape(t *testing.T) {
	dir := filepath.Join(repositoryRoot(t), "application", "projection")
	files := parseProjectionPackage(t, dir)
	want := projectionPublicAPI()
	seen := make(map[string]bool)

	for filename, file := range files {
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				for _, rawSpec := range declaration.Specs {
					typeSpec, ok := rawSpec.(*ast.TypeSpec)
					if !ok {
						if exportedValueSpec(rawSpec) {
							t.Errorf("%s exports a variable or constant; projection API permits exported DTO types only", filename)
						}
						continue
					}
					if !typeSpec.Name.IsExported() {
						continue
					}
					fields, ok := want[typeSpec.Name.Name]
					if !ok {
						t.Errorf("%s exports undocumented type %s", filename, typeSpec.Name.Name)
						continue
					}
					seen[typeSpec.Name.Name] = true
					assertProjectionStruct(t, filename, typeSpec, fields)
				}
			case *ast.FuncDecl:
				if !declaration.Name.IsExported() {
					continue
				}
				if declaration.Recv == nil {
					t.Errorf("%s exports undocumented function %s", filename, declaration.Name.Name)
					continue
				}
				assertProjectionBuildMethod(t, filename, declaration)
			}
		}
	}

	for name := range want {
		if !seen[name] {
			t.Errorf("projection API missing exported type %s", name)
		}
	}
}

type projectionField struct {
	typeName string
	jsonName string
}

func projectionPublicAPI() map[string]map[string]projectionField {
	f := func(typeName, jsonName string) projectionField {
		return projectionField{typeName: typeName, jsonName: jsonName}
	}
	return map[string]map[string]projectionField{
		"Builder": {},
		"Document": {
			"FormatVersion": f("uint32", "formatVersion"), "ProjectID": f("string", "projectId"),
			"RevisionID": f("string", "revisionId"), "Title": f("string", "title"),
			"APIVersion": f("string", "apiVersion"), "Branding": f("Branding", "branding"),
			"Overview": f("Overview", "overview"), "MainLandmark": f("Landmark", "mainLandmark"),
			"OperationGroupHeading": f("Heading", "operationGroupHeading"),
			"SchemaGroupHeading":    f("Heading", "schemaGroupHeading"),
			"SidebarSections":       f("[]SidebarSection", "sidebarSections"),
			"Operations":            f("[]OperationDirectory", "operations"),
			"OperationDetails":      f("[]OperationDetail", "operationDetails"),
			"Schemas":               f("[]SchemaDirectory", "schemas"), "SchemaDetails": f("[]SchemaDetail", "schemaDetails"),
			"Search": f("[]SearchRecord", "search"), "PublicRoutes": f("[]PublicRoute", "publicRoutes"),
		},
		"Branding": {
			"DisplayName": f("string", "displayName"), "LogoSrc": f("string", "logoSrc"),
			"LogoAlt": f("string", "logoAlt"), "LogoHomeHref": f("string", "logoHomeHref"),
			"FaviconHref": f("string", "faviconHref"),
		},
		"Overview": {
			"Anchor": f("string", "anchor"), "Href": f("string", "href"), "HeadingID": f("string", "headingId"),
			"Heading": f("string", "heading"), "HeadingLevel": f("uint32", "headingLevel"),
			"Description": f("string", "description"), "TermsOfService": f("string", "termsOfService"),
			"SpecDownloadFilename": f("string", "specDownloadFilename"), "Contact": f("Contact", "contact"),
			"License": f("License", "license"), "Servers": f("[]Server", "servers"),
		},
		"Contact":  {"Name": f("string", "name"), "URL": f("string", "url"), "Email": f("string", "email")},
		"License":  {"Name": f("string", "name"), "URL": f("string", "url"), "Identifier": f("string", "identifier")},
		"Landmark": {"ID": f("string", "id"), "Role": f("string", "role")},
		"Heading":  {"ID": f("string", "id"), "Text": f("string", "text"), "Level": f("uint32", "level")},
		"Server": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "URL": f("string", "url"),
			"Description": f("string", "description"), "Variables": f("[]ServerVariable", "variables"),
		},
		"ServerVariable": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Name": f("string", "name"),
			"Default": f("string", "default"), "Description": f("string", "description"), "Enum": f("[]TextRecord", "enum"),
		},
		"TextRecord": {"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Value": f("string", "value")},
		"SidebarSection": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Kind": f("string", "kind"),
			"Title": f("string", "title"), "Href": f("string", "href"), "Items": f("[]SidebarItem", "items"),
		},
		"SidebarItem": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Anchor": f("string", "anchor"),
			"Href": f("string", "href"), "Label": f("string", "label"), "Method": f("string", "method"),
		},
		"OperationDirectory": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Anchor": f("string", "anchor"),
			"Href": f("string", "href"), "Method": f("string", "method"), "Path": f("string", "path"),
			"Title": f("string", "title"), "Deprecated": f("bool", "deprecated"), "Sections": f("[]TextRecord", "sections"),
		},
		"OperationDetail": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Anchor": f("string", "anchor"),
			"Href": f("string", "href"), "HeadingID": f("string", "headingId"), "Heading": f("string", "heading"),
			"HeadingLevel": f("uint32", "headingLevel"), "Method": f("string", "method"), "Path": f("string", "path"),
			"Summary": f("string", "summary"), "Description": f("string", "description"), "Deprecated": f("bool", "deprecated"),
			"Tags": f("[]TextRecord", "tags"), "Parameters": f("[]Parameter", "parameters"),
			"HasRequestBody": f("bool", "hasRequestBody"), "RequestBody": f("RequestBody", "requestBody"),
			"Responses": f("[]Response", "responses"), "Security": f("[]SecurityRequirement", "security"),
			"CodeSamples": f("[]CodeSample", "codeSamples"),
		},
		"Parameter": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Name": f("string", "name"),
			"In": f("string", "in"), "Required": f("bool", "required"), "Description": f("string", "description"),
			"Schema": f("WireSchema", "schema"), "Examples": f("[]Example", "examples"),
		},
		"RequestBody": {"Description": f("string", "description"), "Required": f("bool", "required"), "MediaTypes": f("[]MediaType", "mediaTypes")},
		"Response": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Status": f("string", "status"),
			"Description": f("string", "description"), "MediaTypes": f("[]MediaType", "mediaTypes"),
		},
		"MediaType": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "ContentType": f("string", "contentType"),
			"Schema": f("WireSchema", "schema"), "Examples": f("[]Example", "examples"),
		},
		"SecurityRequirement": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Name": f("string", "name"),
			"Scopes": f("[]TextRecord", "scopes"),
		},
		"CodeSample": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Label": f("string", "label"),
			"Language": f("string", "language"), "Code": f("string", "code"),
		},
		"WireSchema": {
			"Name": f("string", "name"), "Type": f("string", "type"), "Format": f("string", "format"),
			"Description": f("string", "description"), "DefaultValue": f("string", "defaultValue"),
			"ExampleText": f("string", "exampleText"), "JSON": f("string", "json"),
			"Properties": f("[]SchemaProperty", "properties"), "Items": f("[]SchemaItem", "items"),
		},
		"SchemaProperty": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Name": f("string", "name"),
			"Required": f("bool", "required"), "Description": f("string", "description"), "Schema": f("WireSchema", "schema"),
		},
		"SchemaItem": {"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Schema": f("WireSchema", "schema")},
		"Example": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Text": f("string", "text"), "Provided": f("bool", "provided"),
		},
		"SchemaDirectory": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Anchor": f("string", "anchor"),
			"Href": f("string", "href"), "Name": f("string", "name"), "Title": f("string", "title"),
			"Description": f("string", "description"),
		},
		"SchemaDetail": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "Anchor": f("string", "anchor"),
			"Href": f("string", "href"), "HeadingID": f("string", "headingId"), "Heading": f("string", "heading"),
			"HeadingLevel": f("uint32", "headingLevel"), "Description": f("string", "description"),
			"Schema": f("WireSchema", "schema"), "ExampleSchemaJSON": f("string", "exampleSchemaJSON"),
			"Examples": f("[]Example", "examples"),
		},
		"SearchRecord": {
			"Ordinal": f("uint32", "ordinal"), "ID": f("string", "id"), "ResultID": f("string", "resultId"),
			"Title": f("string", "title"), "Description": f("string", "description"), "Href": f("string", "href"),
			"Kind": f("string", "kind"), "Method": f("string", "method"), "Path": f("string", "path"),
			"Section": f("string", "section"), "Keywords": f("[]TextRecord", "keywords"),
		},
		"PublicRoute": {
			"Ordinal": f("uint32", "ordinal"), "Path": f("string", "path"), "Title": f("string", "title"),
			"Description": f("string", "description"),
		},
	}
}

func assertProjectionDependencies(t *testing.T, packagePath string, allowed map[string]bool) {
	t.Helper()
	for _, dependency := range packageDependencies(t, packagePath) {
		if dependency.Standard || allowed[dependency.ImportPath] {
			continue
		}
		t.Errorf("%s depends on forbidden package %q", packagePath, dependency.ImportPath)
	}
}

func assertProjectionDirectImports(t *testing.T, relative string, allowed map[string]bool) {
	t.Helper()
	for filename, file := range parseProjectionPackage(t, filepath.Join(repositoryRoot(t), filepath.FromSlash(relative))) {
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("%s import path: %v", filename, err)
			}
			first := strings.Split(path, "/")[0]
			if !strings.Contains(first, ".") || allowed[path] {
				continue
			}
			t.Errorf("%s directly imports forbidden package %q", relative, path)
		}
	}
}

func parseProjectionPackage(t *testing.T, dir string) map[string]*ast.File {
	t.Helper()
	packages, err := parser.ParseDir(token.NewFileSet(), dir, func(info fs.FileInfo) bool {
		return strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse projection package %s: %v", dir, err)
	}
	if len(packages) != 1 {
		t.Fatalf("parse projection package %s: got %d packages, want 1", dir, len(packages))
	}
	for _, pkg := range packages {
		return pkg.Files
	}
	return nil
}

func exportedValueSpec(spec ast.Spec) bool {
	valueSpec, ok := spec.(*ast.ValueSpec)
	if !ok {
		return false
	}
	for _, name := range valueSpec.Names {
		if name.IsExported() {
			return true
		}
	}
	return false
}

func assertProjectionStruct(t *testing.T, filename string, typeSpec *ast.TypeSpec, want map[string]projectionField) {
	t.Helper()
	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		t.Errorf("%s: exported type %s must be a concrete struct", filename, typeSpec.Name.Name)
		return
	}
	got := make(map[string]projectionField)
	for _, field := range structType.Fields.List {
		for _, name := range field.Names {
			if !name.IsExported() {
				t.Errorf("%s: DTO %s contains unexported field %s", filename, typeSpec.Name.Name, name.Name)
				continue
			}
			jsonName := ""
			if field.Tag != nil {
				raw, err := strconv.Unquote(field.Tag.Value)
				if err != nil {
					t.Fatalf("%s: parse tag for %s.%s: %v", filename, typeSpec.Name.Name, name.Name, err)
				}
				jsonName = reflect.StructTag(raw).Get("json")
			}
			got[name.Name] = projectionField{typeName: projectionNodeString(t, field.Type), jsonName: jsonName}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: exported fields for %s = %#v, want %#v", filename, typeSpec.Name.Name, got, want)
	}
}

func assertProjectionBuildMethod(t *testing.T, filename string, declaration *ast.FuncDecl) {
	t.Helper()
	if declaration.Name.Name != "Build" || receiverName(declaration.Recv) != "Builder" {
		t.Errorf("%s exports undocumented method %s.%s", filename, receiverName(declaration.Recv), declaration.Name.Name)
		return
	}
	if _, pointer := declaration.Recv.List[0].Type.(*ast.StarExpr); pointer {
		t.Errorf("%s: Builder.Build must use a value receiver", filename)
	}
	wantParams := []string{"context.Context", "domain.SpecIndex"}
	wantResults := []string{"Document", "error"}
	if got := projectionFieldTypes(t, declaration.Type.Params); !reflect.DeepEqual(got, wantParams) {
		t.Errorf("%s: Builder.Build parameters = %v, want %v", filename, got, wantParams)
	}
	if got := projectionFieldTypes(t, declaration.Type.Results); !reflect.DeepEqual(got, wantResults) {
		t.Errorf("%s: Builder.Build results = %v, want %v", filename, got, wantResults)
	}
}

func projectionFieldTypes(t *testing.T, fields *ast.FieldList) []string {
	t.Helper()
	if fields == nil {
		return nil
	}
	var types []string
	for _, field := range fields.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			types = append(types, projectionNodeString(t, field.Type))
		}
	}
	return types
}

func projectionNodeString(t *testing.T, node ast.Node) string {
	t.Helper()
	var buffer bytes.Buffer
	if err := printer.Fprint(&buffer, token.NewFileSet(), node); err != nil {
		t.Fatalf("print AST node: %v", err)
	}
	return buffer.String()
}
