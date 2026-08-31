package htmlartifact

import (
	"strings"
	"testing"
)

func TestBuildKeyIsDeterministicAndResolvesDefaultProfile(t *testing.T) {
	input := buildKeyFixture()
	input.Fragment.Kind = FragmentSidebar
	input.Fragment.Resource = "catalog/default/spec/pets/sidebar/operations/000000"
	first, err := NewBuildKey(input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := NewBuildKey(input)
	if err != nil {
		t.Fatal(err)
	}
	if first != replay || !first.Valid() {
		t.Fatalf("build keys = %q and %q", first, replay)
	}

	explicit := input
	explicit.Profile.SidebarChunkSize = DefaultSidebarChunkSize
	explicitKey, err := NewBuildKey(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if explicitKey != first {
		t.Fatalf("default profile key %q differs from explicit default %q", first, explicitKey)
	}
}

func TestBuildKeyInvalidatesForEveryOutputDependency(t *testing.T) {
	baseline := buildKeyFixture()
	want, err := NewBuildKey(baseline)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*BuildKeyInput)
	}{
		{"fragment format", func(value *BuildKeyInput) { value.Fragment.Format = "manja-fragment-v2" }},
		{"resource kind", func(value *BuildKeyInput) { value.Fragment.Kind = FragmentSchema }},
		{"resource identity", func(value *BuildKeyInput) { value.Fragment.Resource = "catalog/default/spec/pets/operation/createPet" }},
		{"payload", func(value *BuildKeyInput) { value.CanonicalPayloadSHA256 = strings.Repeat("b", 64) }},
		{"Manja version", func(value *BuildKeyInput) { value.ManjaVersion = "v2.1.0" }},
		{"renderer", func(value *BuildKeyInput) { value.RendererFingerprint = "templates-sha256:b" }},
		{"UI", func(value *BuildKeyInput) { value.UIFingerprint = "assets-sha256:b" }},
		{"compiler", func(value *BuildKeyInput) { value.CompilerIdentity = "goshtoso@v0.2.7" }},
		{"normalizer", func(value *BuildKeyInput) { value.NormalizerIdentity = "openapi-normalizer-v4" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := baseline
			test.mutate(&changed)
			got, err := NewBuildKey(changed)
			if err != nil {
				t.Fatal(err)
			}
			if got == want {
				t.Fatalf("changed dependency retained build key %q", got)
			}
		})
	}

	t.Run("sidebar chunk size invalidates sidebar only", func(t *testing.T) {
		sidebar := baseline
		sidebar.Fragment.Kind = FragmentSidebar
		sidebar.Fragment.Resource = "catalog/default/spec/pets/sidebar/operations/000000"
		first, err := NewBuildKey(sidebar)
		if err != nil {
			t.Fatal(err)
		}
		sidebar.Profile.SidebarChunkSize = 24
		changed, err := NewBuildKey(sidebar)
		if err != nil {
			t.Fatal(err)
		}
		if changed == first {
			t.Fatal("sidebar chunk-size change retained the sidebar build key")
		}

		operation := baseline
		operation.Profile.SidebarChunkSize = 24
		unchanged, err := NewBuildKey(operation)
		if err != nil {
			t.Fatal(err)
		}
		if unchanged != want {
			t.Fatal("sidebar chunk-size change invalidated an operation fragment")
		}
	})
}

func TestBuildKeyRejectsIncompleteInputs(t *testing.T) {
	input := buildKeyFixture()
	input.UIFingerprint = ""
	if _, err := NewBuildKey(input); err == nil {
		t.Fatal("missing UI fingerprint was accepted")
	}
	input = buildKeyFixture()
	input.CanonicalPayloadSHA256 = strings.Repeat("A", 64)
	if _, err := NewBuildKey(input); err == nil {
		t.Fatal("non-canonical payload digest was accepted")
	}
}

func buildKeyFixture() BuildKeyInput {
	return BuildKeyInput{
		Fragment: FragmentIdentity{
			Format:   "manja-fragment-v1",
			Kind:     FragmentOperation,
			Resource: "catalog/default/spec/pets/operation/getPet",
		},
		CanonicalPayloadSHA256: strings.Repeat("a", 64),
		ManjaVersion:           "v2.0.0",
		RendererFingerprint:    "templates-sha256:a",
		UIFingerprint:          "assets-sha256:a",
		CompilerIdentity:       "goshtoso@v0.2.6",
		NormalizerIdentity:     "openapi-normalizer-v3",
	}
}
