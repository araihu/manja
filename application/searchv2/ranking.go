package searchv2

import (
	"fmt"

	"github.com/araihu/manja/domain"
)

const DeploymentScope = "deployment"

// MatchQuality is an ordinal relevance class, from description-only through
// fuzzy, prefix, exact token/title, and exact identity/full-path matches. The
// rank stride is larger than every possible context adjustment, making context
// a tie-breaker rather than a substitute for relevance.
type MatchQuality uint8

const (
	MatchDescription   MatchQuality = 1
	MatchFuzzy         MatchQuality = 2
	MatchPrefix        MatchQuality = 3
	MatchExactToken    MatchQuality = 4
	MatchExactIdentity MatchQuality = 5
)

type ContextKind string

const (
	ContextHome    ContextKind = "home"
	ContextCatalog ContextKind = "catalog"
	ContextSpec    ContextKind = "spec"
)

// QueryContext controls deployment-wide contextual boosting. It never changes
// search scope and therefore cannot hide records outside the current catalog.
type QueryContext struct {
	Kind       ContextKind `json:"kind"`
	CatalogKey string      `json:"catalogKey,omitempty"`
	SpecKey    string      `json:"specKey,omitempty"`
}

// RankingPolicy is serialized metadata shared by future build and browser
// implementations.
type RankingPolicy struct {
	Scope         string `json:"scope"`
	QualityStride uint16 `json:"qualityStride"`
	CatalogBoost  uint16 `json:"catalogBoost"`
	SpecBoost     uint16 `json:"specBoost"`
}

// DefaultRankingPolicy encodes the accepted deployment search behavior.
func DefaultRankingPolicy() RankingPolicy {
	return RankingPolicy{
		Scope: DeploymentScope, QualityStride: 100,
		CatalogBoost: 8, SpecBoost: 24,
	}
}

func HomeQueryContext() QueryContext { return QueryContext{Kind: ContextHome} }

func CatalogQueryContext(catalogKey string) (QueryContext, error) {
	if err := domain.ValidateCatalogID(catalogKey); err != nil {
		return QueryContext{}, err
	}
	return QueryContext{Kind: ContextCatalog, CatalogKey: catalogKey}, nil
}

func SpecQueryContext(catalogKey, specKey string) (QueryContext, error) {
	if err := domain.ValidateCatalogID(catalogKey); err != nil {
		return QueryContext{}, err
	}
	if err := domain.ValidateCatalogDocumentKey(specKey); err != nil {
		return QueryContext{}, err
	}
	return QueryContext{Kind: ContextSpec, CatalogKey: catalogKey, SpecKey: specKey}, nil
}

// RankClass returns the deterministic quality/context component of a score.
// Text relevance within the same class remains the responsibility of the
// eventual search engine.
func (policy RankingPolicy) RankClass(quality MatchQuality, context QueryContext, record Record) (uint16, error) {
	if err := policy.validate(); err != nil {
		return 0, err
	}
	if quality < MatchDescription || quality > MatchExactIdentity {
		return 0, fmt.Errorf("search match quality %d is unsupported", quality)
	}
	if err := context.validate(); err != nil {
		return 0, err
	}
	if err := validateCanonicalRecord(record); err != nil {
		return 0, err
	}
	score := uint32(quality) * uint32(policy.QualityStride)
	catalogMatches := context.CatalogKey != "" && context.CatalogKey == record.CatalogKey
	if catalogMatches {
		score += uint32(policy.CatalogBoost)
	}
	if catalogMatches && context.SpecKey != "" && context.SpecKey == record.SpecKey {
		score += uint32(policy.SpecBoost)
	}
	if score > uint32(^uint16(0)) {
		return 0, fmt.Errorf("search rank class exceeds uint16")
	}
	return uint16(score), nil
}

func (policy RankingPolicy) validate() error {
	if policy.Scope != DeploymentScope {
		return fmt.Errorf("search scope %q is unsupported", policy.Scope)
	}
	if policy.CatalogBoost == 0 || policy.SpecBoost <= policy.CatalogBoost {
		return fmt.Errorf("search context boosts are invalid")
	}
	contextBoost := uint32(policy.CatalogBoost) + uint32(policy.SpecBoost)
	if uint32(policy.QualityStride) <= contextBoost {
		return fmt.Errorf("search quality stride must dominate context boosts")
	}
	if uint32(MatchExactIdentity)*uint32(policy.QualityStride)+contextBoost > uint32(^uint16(0)) {
		return fmt.Errorf("search ranking policy exceeds uint16")
	}
	return nil
}

func (context QueryContext) validate() error {
	switch context.Kind {
	case ContextHome:
		if context.CatalogKey != "" || context.SpecKey != "" {
			return fmt.Errorf("home search context must not contain catalog or spec keys")
		}
	case ContextCatalog:
		if err := domain.ValidateCatalogID(context.CatalogKey); err != nil {
			return err
		}
		if context.SpecKey != "" {
			return fmt.Errorf("catalog search context must not contain a spec key")
		}
	case ContextSpec:
		if err := domain.ValidateCatalogID(context.CatalogKey); err != nil {
			return err
		}
		if err := domain.ValidateCatalogDocumentKey(context.SpecKey); err != nil {
			return err
		}
	default:
		return fmt.Errorf("search context kind %q is unsupported", context.Kind)
	}
	return nil
}
