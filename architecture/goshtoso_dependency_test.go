package architecture_test

import (
	"encoding/json"
	"go/version"
	"io/fs"
	"path/filepath"
	"sort"
	"testing"
)

const (
	goshtosoModulePath = "github.com/araihu/goshtoso"
	goshtosoVersion    = "v0.1.8"
	minimumGoVersion   = "go1.26.5"
)

type consumerModule struct {
	name string
	dir  string
}

type moduleDependency struct {
	Path    string
	Version string
	Replace *moduleDependency
}

type moduleFile struct {
	Go string
}

func TestGoshtosoConsumerModulesUseV018(t *testing.T) {
	root := repositoryRoot(t)
	for _, module := range discoverConsumerModules(t, root) {
		t.Run(module.name, func(t *testing.T) {
			dependency := listedModule(t, module.dir, goshtosoModulePath)
			if dependency.Path != goshtosoModulePath {
				t.Fatalf("module path = %q, want %q", dependency.Path, goshtosoModulePath)
			}
			if dependency.Version != goshtosoVersion {
				t.Errorf("%s version = %q, want %q", goshtosoModulePath, dependency.Version, goshtosoVersion)
			}
			if dependency.Replace != nil {
				t.Errorf("%s must not be replaced: %#v", goshtosoModulePath, dependency.Replace)
			}

			moduleFile := editedModule(t, module.dir)
			if version.Compare("go"+moduleFile.Go, minimumGoVersion) < 0 {
				t.Errorf("go directive = %q, want at least %q", moduleFile.Go, minimumGoVersion[2:])
			}

			if module.dir != root {
				runConsumerModuleCommand(t, module.dir, "test", "test", "./...", "-count=1")
				runConsumerModuleCommand(t, module.dir, "vet", "vet", "./...")
				runConsumerModuleCommand(t, module.dir, "tidy-diff", "mod", "tidy", "-diff")
			}
		})
	}
}

func discoverConsumerModules(t *testing.T, root string) []consumerModule {
	t.Helper()
	skippedDirectories := map[string]bool{
		".git":         true,
		".manja":       true,
		"node_modules": true,
		"tmp":          true,
		"vendor":       true,
	}
	var modules []consumerModule
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && path != root && skippedDirectories[entry.Name()] {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != "go.mod" {
			return nil
		}

		dir := filepath.Dir(path)
		relative, err := filepath.Rel(root, dir)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if name == "." {
			name = "root"
		}
		modules = append(modules, consumerModule{name: name, dir: dir})
		return nil
	})
	if err != nil {
		t.Fatalf("discover consumer modules: %v", err)
	}
	if len(modules) == 0 {
		t.Fatal("discover consumer modules: no go.mod files found")
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].name < modules[j].name })
	return modules
}

func runConsumerModuleCommand(t *testing.T, dir, gate string, args ...string) {
	t.Helper()
	out, err := command(dir, "go", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: go %v: %v\n%s", gate, args, err, out)
	}
	t.Logf("%s: PASS", gate)
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
