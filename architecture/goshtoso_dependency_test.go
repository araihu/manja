package architecture_test

import (
	"encoding/json"
	"go/version"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

func TestCITestsSiteWithNormalizedScratchModfile(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range []string{
		`modfile=.manja-ci-site.mod`,
		`trap 'rm -f "$modfile" "$sumfile"' EXIT`,
		`go mod tidy -modfile="$modfile"`,
		`go test -modfile="$modfile" ./...`,
	} {
		if !strings.Contains(string(workflow), contract) {
			t.Errorf("CI site gate missing %q", contract)
		}
	}
}

func TestNormalizedConsumerModfileLeavesCommittedMetadataUntouched(t *testing.T) {
	root := t.TempDir()
	dependencyDir := filepath.Join(root, "dependency")
	consumerDir := filepath.Join(root, "consumer")
	for _, dir := range []string{dependencyDir, consumerDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(dependencyDir, "go.mod"):        "module example.com/dependency\n\ngo 1.26.5\n",
		filepath.Join(dependencyDir, "dependency.go"): "package dependency\n",
		filepath.Join(consumerDir, "go.mod"):          "module example.com/consumer\n\ngo 1.26.5\n\nrequire example.com/dependency v0.0.0\n\nreplace example.com/dependency => ../dependency\n",
		filepath.Join(consumerDir, "consumer.go"):     "package consumer\n\nimport _ \"example.com/dependency\"\n",
		filepath.Join(consumerDir, "go.sum"):          "github.com/a-h/templ v0.3.1020/go.mod h1:A2DlK61v+K+NRoGnhmYbNYVmtYHcFO5/AisMvBdDxTM=\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	originalMod := []byte(files[filepath.Join(consumerDir, "go.mod")])
	originalSum := []byte(files[filepath.Join(consumerDir, "go.sum")])

	modFile := normalizedConsumerModfile(t, consumerDir)
	runConsumerModuleCommand(t, consumerDir, "test", "test", "-modfile="+modFile, "./...", "-count=1")

	for path, want := range map[string][]byte{
		filepath.Join(consumerDir, "go.mod"): originalMod,
		filepath.Join(consumerDir, "go.sum"): originalSum,
	} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s changed:\n%s", filepath.Base(path), got)
		}
	}
	normalizedSum, err := os.ReadFile(strings.TrimSuffix(modFile, ".mod") + ".sum")
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if strings.Contains(string(normalizedSum), "github.com/a-h/templ") {
		t.Fatalf("normalized sum retained stale dependency:\n%s", normalizedSum)
	}
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
				// Consumer modules inherit Manja's transitive graph through replace.
				// Normalize throwaway metadata so dependency PRs test compatibility
				// without rewriting committed consumer module files.
				modFile := normalizedConsumerModfile(t, module.dir)
				runConsumerModuleCommand(t, module.dir, "test", "test", "-modfile="+modFile, "./...", "-count=1")
				runConsumerModuleCommand(t, module.dir, "vet", "vet", "-modfile="+modFile, "./...")
			}
		})
	}
}

func normalizedConsumerModfile(t *testing.T, dir string) string {
	t.Helper()
	modBytes, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read consumer go.mod: %v", err)
	}
	mod, err := os.CreateTemp(dir, ".manja-consumer-*.mod")
	if err != nil {
		t.Fatalf("create consumer modfile: %v", err)
	}
	modFile := mod.Name()
	sumFile := strings.TrimSuffix(modFile, ".mod") + ".sum"
	t.Cleanup(func() {
		for _, path := range []string{modFile, sumFile} {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				t.Errorf("remove temporary consumer metadata %s: %v", filepath.Base(path), err)
			}
		}
	})
	if _, err := mod.Write(modBytes); err != nil {
		_ = mod.Close()
		t.Fatalf("write consumer modfile: %v", err)
	}
	if err := mod.Close(); err != nil {
		t.Fatalf("close consumer modfile: %v", err)
	}
	if sumBytes, err := os.ReadFile(filepath.Join(dir, "go.sum")); err == nil {
		if err := os.WriteFile(sumFile, sumBytes, 0o600); err != nil {
			t.Fatalf("write consumer sumfile: %v", err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("read consumer go.sum: %v", err)
	}
	runConsumerModuleCommand(t, dir, "tidy", "mod", "tidy", "-modfile="+modFile)
	return modFile
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
