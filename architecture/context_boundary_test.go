package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicOperationsAreContextFirst(t *testing.T) {
	root := repositoryRoot(t)
	checkPortInterfaces(t, filepath.Join(root, "application", "port"))
	checkApplicationMethods(t, filepath.Join(root, "application"))
}

func checkPortInterfaces(t *testing.T, dir string) {
	t.Helper()
	files := parseDirectory(t, dir)
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			typeSpec, ok := node.(*ast.TypeSpec)
			if !ok || !typeSpec.Name.IsExported() {
				return true
			}
			interfaceType, ok := typeSpec.Type.(*ast.InterfaceType)
			if !ok {
				return true
			}
			for _, field := range interfaceType.Methods.List {
				if len(field.Names) == 0 || !field.Names[0].IsExported() {
					continue
				}
				operation, ok := field.Type.(*ast.FuncType)
				if !ok || !contextFirst(operation) {
					t.Errorf("port operation %s.%s must accept context.Context first", typeSpec.Name.Name, field.Names[0].Name)
				}
			}
			return false
		})
	}
}

func checkApplicationMethods(t *testing.T, dir string) {
	t.Helper()
	if !hasProductionGoFiles(t, dir) {
		return
	}
	files := parseDirectory(t, dir)
	for _, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || !function.Name.IsExported() {
				continue
			}
			if !contextFirst(function.Type) {
				t.Errorf("application operation %s must accept context.Context first", function.Name.Name)
			}
		}
	}
}

func parseDirectory(t *testing.T, dir string) map[string]*ast.File {
	t.Helper()
	packages, err := parser.ParseDir(token.NewFileSet(), dir, func(info fs.FileInfo) bool {
		return filepath.Ext(info.Name()) == ".go" && !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	if len(packages) != 1 {
		t.Fatalf("parse %s: got %d packages, want 1", dir, len(packages))
	}
	for _, pkg := range packages {
		return pkg.Files
	}
	return nil
}

func contextFirst(function *ast.FuncType) bool {
	if function.Params == nil || len(function.Params.List) == 0 {
		return false
	}
	selector, ok := function.Params.List[0].Type.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Context" {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && identifier.Name == "context"
}
