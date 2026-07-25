package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/araihu/manja/internal/core"
)

const validConfig = `
version: 1
contracts:
  payments:
    spec: docs/openapi.yaml
    defaultPolicy: stable
    policies:
      stable:
        requireReleaseBaseline: true
        rules:
          operation.removed: fail
          schema.removed: warn
        exceptions:
          - finding: 4f6c9c3f
            reason: Existing v1 client migration
            author: api-platform
            expires: 2026-08-31T23:59:59Z
`

func TestLoadBuildsRepositoryPolicyLayer(t *testing.T) {
	file := writeConfig(t, validConfig)

	loaded, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := loaded.Contract("payments")
	if err != nil {
		t.Fatal(err)
	}
	layer, err := contract.PolicyLayer("")
	if err != nil {
		t.Fatal(err)
	}

	if contract.Spec != "docs/openapi.yaml" {
		t.Fatalf("spec = %q", contract.Spec)
	}
	if layer.Name != "stable" || layer.Source != core.PolicySourceRepository || !layer.RequireReleaseBaseline {
		t.Fatalf("layer = %#v", layer)
	}
	if layer.Rules["operation.removed"] != core.RuleLevelFail || layer.Rules["schema.removed"] != core.RuleLevelWarn {
		t.Fatalf("rules = %#v", layer.Rules)
	}
	if len(layer.Exceptions) != 1 {
		t.Fatalf("exceptions = %#v", layer.Exceptions)
	}
	exception := layer.Exceptions[0]
	wantExpiry := time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC)
	if exception.FindingID != "4f6c9c3f" || exception.RuleID != "" || exception.ExpiresAt != wantExpiry || exception.ExpiresAt.Location() != time.UTC {
		t.Fatalf("exception = %#v", exception)
	}
}

func TestLoadRejectsUnknownFieldsAndMultipleDocuments(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "unknown field", data: validConfig + "unknown: rejected\n"},
		{name: "multiple documents", data: validConfig + "---\nversion: 1\ncontracts: {}\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, tt.data)); err == nil {
				t.Fatal("Load succeeded")
			}
		})
	}
}

func TestLoadRejectsMissingRequiredConfiguration(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "version", data: strings.Replace(validConfig, "version: 1\n", "", 1)},
		{name: "contract id", data: strings.Replace(validConfig, "  payments:\n", "  \"\":\n", 1)},
		{name: "spec", data: strings.Replace(validConfig, "spec: docs/openapi.yaml", "spec: \"\"", 1)},
		{name: "profile", data: `
version: 1
contracts:
  payments:
    spec: docs/openapi.yaml
    defaultPolicy: stable
    policies: {}
`},
		{name: "reason", data: strings.Replace(validConfig, "            reason: Existing v1 client migration\n", "", 1)},
		{name: "author", data: strings.Replace(validConfig, "            author: api-platform\n", "", 1)},
		{name: "expiry", data: strings.Replace(validConfig, "            expires: 2026-08-31T23:59:59Z\n", "", 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, tt.data)); err == nil {
				t.Fatal("Load succeeded")
			}
		})
	}
}

func TestLoadRejectsInvalidExceptionAndRuleLevel(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "finding and rule",
			data: strings.Replace(validConfig, "          - finding: 4f6c9c3f\n", "          - finding: 4f6c9c3f\n            rule: operation.removed\n", 1),
		},
		{
			name: "invalid level",
			data: strings.Replace(validConfig, "operation.removed: fail", "operation.removed: block", 1),
		},
		{
			name: "non RFC3339 expiry",
			data: strings.Replace(validConfig, "2026-08-31T23:59:59Z", "2026-08-31", 1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, tt.data)); err == nil {
				t.Fatal("Load succeeded")
			}
		})
	}
}

func TestContractAndPolicySelectionRejectUnknownValues(t *testing.T) {
	loaded, err := Load(writeConfig(t, validConfig))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loaded.Contract("unknown"); err == nil {
		t.Fatal("Contract succeeded for unknown id")
	}
	contract, err := loaded.Contract("payments")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := contract.PolicyLayer("unknown"); err == nil {
		t.Fatal("PolicyLayer succeeded for unknown profile")
	}

	missingDefault := strings.Replace(validConfig, "defaultPolicy: stable", "defaultPolicy: unknown", 1)
	if _, err := Load(writeConfig(t, missingDefault)); err == nil {
		t.Fatal("Load succeeded for missing default profile")
	}
}

func TestCommittedConfigUsesReviewCandidateFixture(t *testing.T) {
	loaded, err := Load("testdata/manja.yaml")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := loaded.Contract("payments")
	if err != nil {
		t.Fatal(err)
	}
	if contract.Spec != "internal/adapters/openapi/testdata/review/candidate.yaml" {
		t.Fatalf("spec = %q", contract.Spec)
	}
}

func writeConfig(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manja.yaml")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
