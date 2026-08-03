package catalog

import (
	"context"
	"testing"

	"github.com/araihu/manja/domain"
)

func TestCompilerIsDeterministicAcrossInputOrder(t *testing.T) {
	t.Parallel()

	candidate, index := compilerFixture()
	compiler, err := NewCompiler(DefaultCompilerOptions())
	if err != nil {
		t.Fatal(err)
	}
	first, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Documents[0], candidate.Documents[1] = candidate.Documents[1], candidate.Documents[0]
	index.Documents[0], index.Documents[1] = index.Documents[1], index.Documents[0]
	second, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshotsEqual(t, first, second)
}

func TestCompilerIgnoresLocaleAndTimezone(t *testing.T) {
	candidate, index := compilerFixture()
	compiler, err := NewCompiler(DefaultCompilerOptions())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TZ", "UTC")
	t.Setenv("LC_ALL", "C")
	first, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TZ", "Pacific/Chatham")
	t.Setenv("LC_ALL", "pt_BR.UTF-8")
	second, err := compiler.Compile(context.Background(), candidate, index)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshotsEqual(t, first, second)
}

func TestSnapshotIdentityChangesForCanonicalInputs(t *testing.T) {
	t.Parallel()

	baseCandidate, baseIndex := compilerFixture()
	baseOptions := DefaultCompilerOptions()
	compile := func(candidateMutation func(*CompilerOptions, *domainFixture)) SnapshotID {
		options := baseOptions
		candidate, index := compilerFixture()
		fixture := domainFixture{candidate: &candidate, index: &index}
		candidateMutation(&options, &fixture)
		compiler, err := NewCompiler(options)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := compiler.Compile(context.Background(), candidate, index)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot.ID
	}
	compiler, err := NewCompiler(baseOptions)
	if err != nil {
		t.Fatal(err)
	}
	base, err := compiler.Compile(context.Background(), baseCandidate, baseIndex)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*CompilerOptions, *domainFixture){
		"source byte": func(_ *CompilerOptions, fixture *domainFixture) { fixture.candidate.Documents[0].Bytes[1] = 'X' },
		"document key": func(_ *CompilerOptions, fixture *domainFixture) {
			fixture.candidate.Documents[0].Key = "beta-v2"
			fixture.index.Documents[0].Key = "beta-v2"
		},
		"profile": func(options *CompilerOptions, fixture *domainFixture) {
			fixture.candidate.ProfileID = domain.CompatibilityProfileKubernetes
			fixture.index.ProfileID = domain.CompatibilityProfileKubernetes
			options.ProfileAllowlist = []byte(`{"schemaVersion":1,"diagnostics":[]}`)
		},
		"kin checksum": func(options *CompilerOptions, _ *domainFixture) {
			options.Versions.KinOpenAPIChecksum = "different"
		},
		"compiler format": func(options *CompilerOptions, _ *domainFixture) { options.Versions.CompilerFormat = "compiler-v2" },
		"projection format": func(options *CompilerOptions, _ *domainFixture) {
			options.Versions.ProjectionFormat = "projection-v3"
		},
		"search format": func(options *CompilerOptions, _ *domainFixture) { options.Versions.SearchFormat = "search-v2" },
		"partition policy": func(options *CompilerOptions, _ *domainFixture) {
			options.Versions.PartitionPolicy = "partition-v2"
		},
		"budget": func(options *CompilerOptions, _ *domainFixture) { options.Bounds.Children-- },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			if got := compile(mutate); got == base.ID {
				t.Fatalf("mutation %q did not change snapshot ID", name)
			}
		})
	}
}

func TestSnapshotIdentityChangesWithKubernetesAllowlist(t *testing.T) {
	t.Parallel()

	candidate, index := compilerFixture()
	candidate.ProfileID = domain.CompatibilityProfileKubernetes
	index.ProfileID = domain.CompatibilityProfileKubernetes
	compile := func(allowlist string) SnapshotID {
		options := DefaultCompilerOptions()
		options.ProfileAllowlist = []byte(allowlist)
		compiler, err := NewCompiler(options)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := compiler.Compile(context.Background(), candidate, index)
		if err != nil {
			t.Fatal(err)
		}
		return snapshot.ID
	}
	if compile(`{"schemaVersion":1,"diagnostics":[]}`) == compile(`{"schemaVersion":1,"diagnostics":[{"changed":true}]}`) {
		t.Fatal("Kubernetes allowlist mutation did not change snapshot ID")
	}
}

type domainFixture struct {
	candidate *domain.CatalogCandidate
	index     *domain.CatalogIndex
}
