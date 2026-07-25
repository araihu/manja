package architecture_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const modulePath = "github.com/araihu/manja"

type listedPackage struct {
	ImportPath string
	Standard   bool
}

func TestModulePathRemainsCanonical(t *testing.T) {
	root := repositoryRoot(t)
	out, err := command(root, "go", "list", "-m", "-f", "{{.Path}}").CombinedOutput()
	if err != nil {
		t.Fatalf("go list module: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != modulePath {
		t.Fatalf("module path = %q, want %q", got, modulePath)
	}
}

func TestDomainImportsOnlyStandardLibrary(t *testing.T) {
	dependencies := packageDependencies(t, modulePath+"/domain")
	for _, dependency := range dependencies {
		if dependency.ImportPath == modulePath+"/domain" || dependency.Standard {
			continue
		}
		t.Errorf("domain depends on non-standard package %q", dependency.ImportPath)
	}
}

func TestApplicationDependencyDirection(t *testing.T) {
	if !hasProductionGoFiles(t, filepath.Join(repositoryRoot(t), "application")) {
		return
	}
	allowed := map[string]bool{
		modulePath + "/application":      true,
		modulePath + "/application/port": true,
		modulePath + "/domain":           true,
	}
	for _, dependency := range packageDependencies(t, modulePath+"/application") {
		if dependency.Standard || allowed[dependency.ImportPath] {
			continue
		}
		t.Errorf("application depends on forbidden package %q", dependency.ImportPath)
	}
}

func TestPortAndContracttestDependencyDirection(t *testing.T) {
	for _, target := range []struct {
		path    string
		allowed map[string]bool
	}{
		{
			path: modulePath + "/application/port",
			allowed: map[string]bool{
				modulePath + "/application/port": true,
				modulePath + "/domain":           true,
			},
		},
		{
			path: modulePath + "/contracttest",
			allowed: map[string]bool{
				modulePath + "/application/port": true,
				modulePath + "/contracttest":     true,
				modulePath + "/domain":           true,
			},
		},
	} {
		relative := strings.TrimPrefix(target.path, modulePath+"/")
		if !hasProductionGoFiles(t, filepath.Join(repositoryRoot(t), filepath.FromSlash(relative))) {
			continue
		}
		for _, dependency := range packageDependencies(t, target.path) {
			if dependency.Standard || target.allowed[dependency.ImportPath] {
				continue
			}
			t.Errorf("%s depends on forbidden package %q", target.path, dependency.ImportPath)
		}
	}
}

func hasProductionGoFiles(t *testing.T, dir string) bool {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("list Go files in %s: %v", dir, err)
	}
	for _, match := range matches {
		if !strings.HasSuffix(match, "_test.go") {
			return true
		}
	}
	return false
}

func packageDependencies(t *testing.T, packagePath string) []listedPackage {
	t.Helper()
	cmd := command(repositoryRoot(t), "go", "list", "-deps", "-json", packagePath)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("go list %s: %v\n%s", packagePath, err, exitErr.Stderr)
		}
		t.Fatalf("go list %s: %v", packagePath, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	var dependencies []listedPackage
	for decoder.More() {
		var dependency listedPackage
		if err := decoder.Decode(&dependency); err != nil {
			t.Fatalf("decode go list output for %s: %v", packagePath, err)
		}
		dependencies = append(dependencies, dependency)
	}
	sort.Slice(dependencies, func(i, j int) bool {
		return dependencies[i].ImportPath < dependencies[j].ImportPath
	})
	return dependencies
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	return filepath.Dir(filepath.Dir(file))
}

func command(dir, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	return cmd
}
