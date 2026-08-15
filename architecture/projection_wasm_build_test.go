package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectionWasmBuild(t *testing.T) {
	cmd := command(repositoryRoot(t), "go", "build", "-trimpath", "./application/projection")
	cmd.Env = append(cmd.Env, "GOOS=js", "GOARCH=wasm", "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 4096 {
			output = output[:4096]
		}
		t.Fatalf("compile projection for js/wasm: %v\n%s", err, output)
	}
}

func TestLocalDocsSchemaNodeRendererWasmBuildAndBoundary(t *testing.T) {
	root := repositoryRoot(t)
	list := command(root, "go", "list", "-deps", "./internal/localdocs/render")
	list.Env = append(list.Env, "GOOS=js", "GOARCH=wasm", "GOWORK=off")
	output, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("list local docs renderer dependencies for js/wasm: %v\n%s", err, output)
	}
	allowed := map[string]bool{
		modulePath + "/domain":                    true,
		modulePath + "/application/projection":    true,
		modulePath + "/application/catalog":       true,
		modulePath + "/internal/localdocs/render": true,
	}
	for _, dependency := range strings.Fields(string(output)) {
		if strings.HasPrefix(dependency, modulePath+"/") && !allowed[dependency] {
			t.Errorf("local docs renderer depends on forbidden package %q", dependency)
		}
	}

	imports := command(root, "go", "list", "-f", `{{join .Imports "\n"}}`, "./internal/localdocs/render")
	imports.Env = append(imports.Env, "GOOS=js", "GOARCH=wasm", "GOWORK=off")
	output, err = imports.CombinedOutput()
	if err != nil {
		t.Fatalf("list local docs renderer imports for js/wasm: %v\n%s", err, output)
	}
	for _, forbidden := range []string{"net/http", "os", modulePath + "/internal/web", modulePath + "/internal/adapters"} {
		for _, imported := range strings.Fields(string(output)) {
			if imported == forbidden || strings.HasPrefix(imported, forbidden+"/") {
				t.Errorf("local docs renderer directly imports forbidden package %q", imported)
			}
		}
	}

	build := command(root, "go", "build", "-trimpath", "./internal/localdocs/render")
	build.Env = append(build.Env, "GOOS=js", "GOARCH=wasm", "GOWORK=off")
	output, err = build.CombinedOutput()
	if err != nil {
		if len(output) > 4096 {
			output = output[:4096]
		}
		t.Fatalf("compile local docs renderer for js/wasm: %v\n%s", err, output)
	}
}

func TestSchemaNodeFragmentUsesOneCanonicalComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "catalog.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, "templ catalogSchemaNode(") {
		t.Fatal("catalog template retains a second schema-node renderer")
	}
	if !strings.Contains(text, "@localrender.SchemaNode(*data.SchemaNode)") {
		t.Fatal("catalog template does not delegate to canonical local renderer")
	}
}

func TestSchemaDetailHeaderFragmentUsesOneCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "catalog.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, `<header class="grid min-w-0 gap-4">`) {
		t.Fatal("catalog template retains a second schema-detail header renderer")
	}
	if !strings.Contains(text, "@localrender.SchemaDetailHeader(*data.SchemaDetailHeader") {
		t.Fatal("catalog template does not delegate to canonical schema-detail header renderer")
	}
}

func TestSchemaDetailExampleFragmentUsesOneCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "catalog.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, `@codeExample("Root JSON Schema", "json", schema.ExampleSchemaJSON)`) {
		t.Fatal("catalog template retains a second schema-detail example renderer")
	}
	if !strings.Contains(text, "@localrender.SchemaDetailExample(*data.SchemaDetailExample)") {
		t.Fatal("catalog template does not delegate to canonical schema-detail example renderer")
	}
}

func TestSchemaDetailBodyFragmentUsesOneCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "catalog.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "@localrender.SchemaDetailBody(*data.SchemaDetailBody)") {
		t.Fatal("catalog template does not delegate to canonical schema-detail body renderer")
	}
}

func TestSchemaDetailFragmentUsesOneCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "catalog.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "@localrender.SchemaDetail(*data.SchemaDetail") {
		t.Fatal("catalog template does not delegate to canonical schema-detail renderer")
	}
}

func TestCatalogDocumentHeaderFragmentUsesOneCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "catalog.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, `<header data-catalog-document-header`) {
		t.Fatal("catalog template retains a second document-header renderer")
	}
	if !strings.Contains(text, "@localrender.CatalogDocumentHeader(*data.DocumentHeader") {
		t.Fatal("catalog template does not delegate to canonical document-header renderer")
	}
}

func TestCatalogDocumentInfoFragmentUsesOneCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "catalog.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, `aria-label="OpenAPI information"`) {
		t.Fatal("catalog template retains a second document-info renderer")
	}
	if !strings.Contains(text, "@localrender.CatalogDocumentInfo(*data.DocumentInfo") {
		t.Fatal("catalog template does not delegate to canonical document-info renderer")
	}
}

func TestCatalogDocumentSecuritySchemesFragmentUsesOneCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "catalog.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, `aria-labelledby="document-security-schemes"`) {
		t.Fatal("catalog template retains a second document security-schemes renderer")
	}
	if strings.Contains(text, "data.Document.SecuritySchemes") {
		t.Fatal("catalog template reads document security schemes outside the canonical fragment")
	}
	if !strings.Contains(text, "@localrender.CatalogDocumentSecuritySchemes(*data.DocumentSecuritySchemes)") {
		t.Fatal("catalog template does not delegate to canonical document security-schemes renderer")
	}
}

func TestOperationHeaderFragmentUsesOneCanonicalComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "catalog.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, "templ catalogOperationHeader(") {
		t.Fatal("catalog template retains a second operation-header renderer")
	}
	if !strings.Contains(text, "@localrender.OperationHeader(*data.OperationHeader") {
		t.Fatal("catalog template does not delegate to canonical operation-header renderer")
	}
}

func TestOperationParametersFragmentUsesOneCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "public.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "if opts.OperationParameters != nil") {
		t.Fatal("shared operation-parameter renderer is not gated on prepared catalog data")
	}
	if !strings.Contains(text, "@localrender.OperationParameters(*opts.OperationParameters)") {
		t.Fatal("catalog operation body does not delegate to the canonical local renderer")
	}
}

func TestOperationExamplesFragmentUsesOneCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "public.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, want := range []string{
		"@localrender.OperationResponseExample(*opts.OperationExamples",
		"@localrender.OperationCodeSample(*opts.OperationExamples",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("catalog operation examples do not delegate to canonical local renderer %q", want)
		}
	}
}

func TestOperationSchemaTreesUseOneCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "public.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, want := range []string{
		"@localrender.OperationRequestBodySchemaTree(*opts.OperationSchemaTrees",
		"@localrender.OperationResponseSchemaTree(*opts.OperationSchemaTrees",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("catalog operation schema trees do not delegate to canonical local renderer %q", want)
		}
	}
}

func TestOperationRequestBodyFragmentUsesCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "public.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, want := range []string{
		"if op.RequestBody != nil && opts.OperationRequestBody != nil",
		"@localrender.OperationRequestBody(*opts.OperationRequestBody)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("catalog request body does not delegate to canonical local renderer %q", want)
		}
	}
}

func TestOperationRequestSectionFragmentUsesCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "public.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, want := range []string{
		"if opts.OperationRequestSection != nil",
		"@localrender.OperationRequestSection(*opts.OperationRequestSection)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("catalog request section does not delegate to canonical local renderer %q", want)
		}
	}
}

func TestOperationNavigationFragmentUsesCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "public.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, want := range []string{
		"if opts.OperationNavigation != nil",
		"@localrender.OperationNavigation(*opts.OperationNavigation)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("catalog operation navigation does not delegate to canonical local renderer %q", want)
		}
	}
}

func TestOperationResponsesFragmentUsesCanonicalCatalogComponent(t *testing.T) {
	root := repositoryRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "web", "templates", "public.templ"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, want := range []string{
		"if opts.OperationResponses != nil",
		"@localrender.OperationResponses(*opts.OperationResponses)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("catalog operation responses do not delegate to canonical local renderer %q", want)
		}
	}
}

func TestLocalDocsActivationWasmBuildAndBoundary(t *testing.T) {
	list := command(repositoryRoot(t), "go", "list", "-deps", "./internal/localdocs")
	list.Env = append(list.Env, "GOOS=js", "GOARCH=wasm", "GOWORK=off")
	output, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("list local docs activation dependencies for js/wasm: %v\n%s", err, output)
	}
	allowed := map[string]bool{
		modulePath + "/domain":                        true,
		modulePath + "/application/projection":        true,
		modulePath + "/application/catalog":           true,
		modulePath + "/internal/adapters/catalogjson": true,
		modulePath + "/internal/localdocs":            true,
	}
	for _, dependency := range strings.Fields(string(output)) {
		if strings.HasPrefix(dependency, modulePath+"/") && !allowed[dependency] {
			t.Errorf("local docs activation depends on forbidden package %q", dependency)
		}
	}

	build := command(repositoryRoot(t), "go", "build", "-trimpath", "./internal/localdocs")
	build.Env = append(build.Env, "GOOS=js", "GOARCH=wasm", "GOWORK=off")
	output, err = build.CombinedOutput()
	if err != nil {
		if len(output) > 4096 {
			output = output[:4096]
		}
		t.Fatalf("compile local docs activation for js/wasm: %v\n%s", err, output)
	}
}
