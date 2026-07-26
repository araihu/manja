package architecture_test

import (
	"encoding/json"
	"go/version"
	"path/filepath"
	"testing"
)

const (
	goshtosoModulePath = "github.com/araihu/goshtoso"
	goshtosoVersion    = "v0.0.12"
	minimumGoVersion   = "go1.26.5"
)

type moduleDependency struct {
	Path    string
	Version string
	Replace *moduleDependency
}

type moduleFile struct {
	Go string
}

func TestGoshtosoConsumerModulesUseV0012(t *testing.T) {
	root := repositoryRoot(t)
	modules := map[string]string{
		"root": root,
		"site": filepath.Join(root, "site"),
	}

	for name, dir := range modules {
		t.Run(name, func(t *testing.T) {
			dependency := listedModule(t, dir, goshtosoModulePath)
			if dependency.Path != goshtosoModulePath {
				t.Fatalf("module path = %q, want %q", dependency.Path, goshtosoModulePath)
			}
			if dependency.Version != goshtosoVersion {
				t.Errorf("%s version = %q, want %q", goshtosoModulePath, dependency.Version, goshtosoVersion)
			}
			if dependency.Replace != nil {
				t.Errorf("%s must not be replaced: %#v", goshtosoModulePath, dependency.Replace)
			}

			module := editedModule(t, dir)
			if version.Compare("go"+module.Go, minimumGoVersion) < 0 {
				t.Errorf("go directive = %q, want at least %q", module.Go, minimumGoVersion[2:])
			}
		})
	}
}

func listedModule(t *testing.T, dir, modulePath string) moduleDependency {
	t.Helper()
	out, err := command(dir, "go", "list", "-m", "-json", modulePath).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -m %s: %v\n%s", modulePath, err, out)
	}
	var dependency moduleDependency
	if err := json.Unmarshal(out, &dependency); err != nil {
		t.Fatalf("decode go list -m %s: %v\n%s", modulePath, err, out)
	}
	return dependency
}

func editedModule(t *testing.T, dir string) moduleFile {
	t.Helper()
	out, err := command(dir, "go", "mod", "edit", "-json").CombinedOutput()
	if err != nil {
		t.Fatalf("go mod edit -json: %v\n%s", err, out)
	}
	var module moduleFile
	if err := json.Unmarshal(out, &module); err != nil {
		t.Fatalf("decode go mod edit -json: %v\n%s", err, out)
	}
	return module
}
