package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDomainVocabularyExcludesHostedSaaSConcepts(t *testing.T) {
	root := repositoryRoot(t)
	domainDir := filepath.Join(root, "domain")
	packages, err := parser.ParseDir(token.NewFileSet(), domainDir, func(info fs.FileInfo) bool {
		return filepath.Ext(info.Name()) == ".go" && !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse domain: %v", err)
	}
	forbidden := []string{"tenant", "organization", "billing", "subscription", "entitlement"}
	for _, pkg := range packages {
		for filename, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.Ident:
					checkForbiddenVocabulary(t, filename, "identifier", value.Name, forbidden)
				case *ast.Field:
					if value.Tag == nil {
						return true
					}
					tag := reflect.StructTag(strings.Trim(value.Tag.Value, "`"))
					for _, key := range []string{"json", "yaml"} {
						serialized := strings.Split(tag.Get(key), ",")[0]
						checkForbiddenVocabulary(t, filename, key+" tag", serialized, forbidden)
					}
				}
				return true
			})
		}
	}
}

func checkForbiddenVocabulary(t *testing.T, filename, kind, value string, forbidden []string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, term := range forbidden {
		if strings.Contains(lower, term) {
			t.Errorf("%s: %s %q contains forbidden public-domain term %q", filename, kind, value, term)
		}
	}
}
