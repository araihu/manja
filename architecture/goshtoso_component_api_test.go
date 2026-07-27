package architecture_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestWebTemplatesUseGoshtosoV0013ComponentAPI(t *testing.T) {
	root := repositoryRoot(t)
	templates := []string{
		"internal/web/templates/management.templ",
		"internal/web/templates/public.templ",
	}
	forbidden := []struct {
		name    string
		pattern *regexp.Regexp
	}{
		{"generic Variant config fields", regexp.MustCompile(`\bVariant\s*:`)},
		{"generic Style config fields", regexp.MustCompile(`\bStyle\s*:`)},
		{"button.Config constructors", regexp.MustCompile(`\bbutton\.Config\b`)},
		{"badge.Variant types", regexp.MustCompile(`\bbadge\.Variant\b`)},
		{"removed SoftVariantClasses", regexp.MustCompile(`\.SoftVariantClasses\s*\(`)},
		{"removed BadgeCellClasses", regexp.MustCompile(`\btable\.BadgeCellClasses\s*\(`)},
		{"sidebar BadgeClass escape hatch", regexp.MustCompile(`\bBadgeClass\s*:`)},
	}

	for _, relative := range templates {
		t.Run(filepath.Base(relative), func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
			if err != nil {
				t.Fatalf("read %s: %v", relative, err)
			}
			for _, forbiddenAPI := range forbidden {
				if forbiddenAPI.pattern.Match(source) {
					t.Errorf("%s still contains %s (%s)", relative, forbiddenAPI.name, forbiddenAPI.pattern)
				}
			}
		})
	}
}
