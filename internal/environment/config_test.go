package environment

import "testing"

func TestParseDefaultsResourceLimitsOff(t *testing.T) {
	config, err := Parse(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if config.ResourceLimits {
		t.Fatal("resource limits are enabled by default")
	}
	if config.LocalDocsDisabled() {
		t.Fatal("local docs are disabled by default")
	}
}

func TestParseResourceLimitsOn(t *testing.T) {
	config, err := Parse(map[string]string{"MANJA_RESOURCE_LIMITS": "true", "MANJA_LOCAL_DOCS": "off"})
	if err != nil {
		t.Fatal(err)
	}
	if !config.ResourceLimits || !config.LocalDocsDisabled() {
		t.Fatalf("config = %#v", config)
	}
}

func TestParseRejectsInvalidValues(t *testing.T) {
	for name, values := range map[string]map[string]string{
		"resource limits":  {"MANJA_RESOURCE_LIMITS": "on"},
		"local docs":       {"MANJA_LOCAL_DOCS": "yes"},
		"empty local docs": {"MANJA_LOCAL_DOCS": ""},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(values); err == nil {
				t.Fatal("invalid environment was accepted")
			}
		})
	}
}
