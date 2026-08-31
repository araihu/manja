package searchv2

import "testing"

func TestDeploymentRankingContextNeverBecomesAScopeFilter(t *testing.T) {
	policy := DefaultRankingPolicy()
	current := rankingRecord(t, "platform", "pets")
	other := rankingRecord(t, "partners", "billing")
	context, err := SpecQueryContext("platform", "pets")
	if err != nil {
		t.Fatal(err)
	}

	weak, err := policy.RankClass(MatchPrefix, context, current)
	if err != nil {
		t.Fatal(err)
	}
	exact, err := policy.RankClass(MatchExactIdentity, context, other)
	if err != nil {
		t.Fatal(err)
	}
	if exact <= weak {
		t.Fatalf("exact result outside context scored %d, contextual prefix scored %d", exact, weak)
	}
	if policy.Scope != DeploymentScope {
		t.Fatalf("scope = %q, want deployment", policy.Scope)
	}
}

func TestExactIdentityOutsideContextOutranksLocalExactToken(t *testing.T) {
	policy := DefaultRankingPolicy()
	current := rankingRecord(t, "platform", "pets")
	other := rankingRecord(t, "partners", "billing")
	context, err := SpecQueryContext("platform", "pets")
	if err != nil {
		t.Fatal(err)
	}

	localToken, err := policy.RankClass(MatchExactToken, context, current)
	if err != nil {
		t.Fatal(err)
	}
	remoteIdentity, err := policy.RankClass(MatchExactIdentity, context, other)
	if err != nil {
		t.Fatal(err)
	}
	if remoteIdentity <= localToken {
		t.Fatalf("remote exact identity scored %d, local exact token scored %d", remoteIdentity, localToken)
	}
}

func TestRankingContextsApplyNoHomeBoostThenCatalogAndSpecBoosts(t *testing.T) {
	policy := DefaultRankingPolicy()
	record := rankingRecord(t, "platform", "pets")
	home, _ := policy.RankClass(MatchFuzzy, HomeQueryContext(), record)
	catalogContext, _ := CatalogQueryContext("platform")
	catalog, _ := policy.RankClass(MatchFuzzy, catalogContext, record)
	specContext, _ := SpecQueryContext("platform", "pets")
	spec, _ := policy.RankClass(MatchFuzzy, specContext, record)
	if catalog-home != policy.CatalogBoost {
		t.Fatalf("catalog adjustment = %d, want %d", catalog-home, policy.CatalogBoost)
	}
	if spec-catalog != policy.SpecBoost {
		t.Fatalf("spec adjustment = %d, want %d", spec-catalog, policy.SpecBoost)
	}
}

func TestSpecBoostAppliesOnlyToCurrentCatalogOccurrence(t *testing.T) {
	policy := DefaultRankingPolicy()
	current := rankingRecord(t, "platform", "pets")
	sameSpecOtherCatalog := rankingRecord(t, "partners", "pets")
	context, err := SpecQueryContext("platform", "pets")
	if err != nil {
		t.Fatal(err)
	}
	currentScore, err := policy.RankClass(MatchFuzzy, context, current)
	if err != nil {
		t.Fatal(err)
	}
	otherScore, err := policy.RankClass(MatchFuzzy, context, sameSpecOtherCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if currentScore-otherScore != policy.CatalogBoost+policy.SpecBoost {
		t.Fatalf("current occurrence adjustment = %d, want %d", currentScore-otherScore, policy.CatalogBoost+policy.SpecBoost)
	}
}

func rankingRecord(t *testing.T, catalog, spec string) Record {
	t.Helper()
	return mustRecord(t, RecordInput{
		Kind: KindSchema, CatalogKey: catalog, CatalogTitle: catalog,
		SpecKey: spec, SpecTitle: spec, LogicalKey: "#/components/schemas/Pet", Title: "Pet",
		PageHref:     "/catalogs/" + catalog + "/specs/" + spec + "/schemas/pet",
		FragmentHref: "/catalogs/" + catalog + "/specs/" + spec + "/fragments/schemas/pet.html",
	})
}
