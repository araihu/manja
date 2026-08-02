package webassets

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type Report struct {
	Bundles []BundleReport
}

type BundleReport struct {
	Name     string
	Output   string
	Inputs   []string
	Packages []string
}

func normalizeInputs(inputs []string) ([]string, []string, error) {
	normalized := make([]string, 0, len(inputs))
	packageSet := make(map[string]struct{})
	for _, input := range inputs {
		input = filepath.ToSlash(input)
		if strings.HasPrefix(input, "(disabled):") || pathBase(input) == "request-composer-vendor.js" {
			continue
		}
		if marker := strings.LastIndex(input, "/node_modules/"); marker >= 0 {
			input = input[marker+len("/node_modules/"):]
		} else {
			input = strings.TrimPrefix(input, "node_modules/")
		}
		if input == "" || filepath.IsAbs(input) || strings.HasPrefix(input, "../") {
			return nil, nil, fmt.Errorf("unsafe esbuild input %q", input)
		}
		normalized = append(normalized, input)
		if pkg := packageNameFromInput(input); pkg != "" {
			packageSet[pkg] = struct{}{}
		}
	}
	sort.Strings(normalized)
	normalized = compactStrings(normalized)
	packages := make([]string, 0, len(packageSet))
	for pkg := range packageSet {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)
	return normalized, packages, nil
}

func pathBase(value string) string {
	if index := strings.LastIndexByte(value, '/'); index >= 0 {
		return value[index+1:]
	}
	return value
}

func packageNameFromInput(input string) string {
	parts := strings.Split(input, "/")
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	if strings.HasPrefix(parts[0], "@") {
		if len(parts) < 3 {
			return ""
		}
		return parts[0] + "/" + parts[1]
	}
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}
