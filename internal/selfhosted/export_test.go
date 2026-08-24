package selfhosted

import "testing"

func TestExportBasePathValidation(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"/", "/group/project/", "/a-b_1/"} {
		if err := canonicalExportBasePath(value); err != nil {
			t.Errorf("canonicalExportBasePath(%q) = %v", value, err)
		}
	}
	for _, value := range []string{"", "project/", "/project", "//project/", "/group//project/", "/group/../project/", "/group/./project/", `/group\project/`, "/project%2f/", "/project/?x=1", "/project/#x", "/project name/", "/project\n/"} {
		if err := canonicalExportBasePath(value); err == nil {
			t.Errorf("canonicalExportBasePath(%q) succeeded", value)
		}
	}
}
