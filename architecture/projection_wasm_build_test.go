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
