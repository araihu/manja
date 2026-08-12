package architecture_test

import (
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
