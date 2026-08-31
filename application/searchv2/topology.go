package searchv2

import (
	"fmt"
	"sort"

	"github.com/araihu/manja/domain"
)

const (
	DefaultCatalogKey   = "default"
	DefaultCatalogTitle = "Default"
)

// Spec describes one deployment-visible spec before catalog assignment.
type Spec struct {
	Key   string
	Title string
}

// DeclaredCatalog describes explicit user configuration. An empty SpecKeys
// slice is valid and the resulting catalog remains visible.
type DeclaredCatalog struct {
	Key      string
	Title    string
	SpecKeys []string
}

// EffectiveCatalog is a deterministic catalog assignment consumed by both
// page and search builds.
type EffectiveCatalog struct {
	Key      string
	Title    string
	Implicit bool
	Specs    []Spec
}

// Topology is the canonical deployment catalog topology.
type Topology struct {
	Catalogs []EffectiveCatalog
}

// ResolveTopology retains every declared catalog, including empty ones, and
// creates the implicit default catalog only when at least one spec is not
// referenced by any declared catalog. Both catalogs and their specs are sorted
// by stable key, independent of source enumeration order.
func ResolveTopology(declared []DeclaredCatalog, specs []Spec) (Topology, error) {
	bySpec := make(map[string]Spec, len(specs))
	for index, spec := range specs {
		if err := domain.ValidateCatalogDocumentKey(spec.Key); err != nil {
			return Topology{}, fmt.Errorf("spec %d: %w", index, err)
		}
		if err := domain.ValidateCanonicalIdentity("spec title", spec.Title, false); err != nil {
			return Topology{}, fmt.Errorf("spec %q: %w", spec.Key, err)
		}
		if _, exists := bySpec[spec.Key]; exists {
			return Topology{}, fmt.Errorf("spec key %q is duplicated", spec.Key)
		}
		bySpec[spec.Key] = spec
	}

	seenCatalogs := make(map[string]struct{}, len(declared))
	referenced := make(map[string]struct{}, len(specs))
	result := make([]EffectiveCatalog, 0, len(declared)+1)
	for index, catalog := range declared {
		if err := domain.ValidateCatalogID(catalog.Key); err != nil {
			return Topology{}, fmt.Errorf("catalog %d: %w", index, err)
		}
		if catalog.Key == DefaultCatalogKey {
			return Topology{}, fmt.Errorf("catalog key %q is reserved for implicit assignment", DefaultCatalogKey)
		}
		if err := domain.ValidateCanonicalIdentity("catalog title", catalog.Title, false); err != nil {
			return Topology{}, fmt.Errorf("catalog %q: %w", catalog.Key, err)
		}
		if _, exists := seenCatalogs[catalog.Key]; exists {
			return Topology{}, fmt.Errorf("catalog key %q is duplicated", catalog.Key)
		}
		seenCatalogs[catalog.Key] = struct{}{}

		seenMembers := make(map[string]struct{}, len(catalog.SpecKeys))
		members := make([]Spec, 0, len(catalog.SpecKeys))
		for _, key := range catalog.SpecKeys {
			if _, duplicate := seenMembers[key]; duplicate {
				return Topology{}, fmt.Errorf("catalog %q spec key %q is duplicated", catalog.Key, key)
			}
			spec, exists := bySpec[key]
			if !exists {
				return Topology{}, fmt.Errorf("catalog %q references unknown spec %q", catalog.Key, key)
			}
			seenMembers[key] = struct{}{}
			referenced[key] = struct{}{}
			members = append(members, spec)
		}
		sortSpecs(members)
		result = append(result, EffectiveCatalog{Key: catalog.Key, Title: catalog.Title, Specs: members})
	}

	unreferenced := make([]Spec, 0, len(specs)-len(referenced))
	for key, spec := range bySpec {
		if _, exists := referenced[key]; !exists {
			unreferenced = append(unreferenced, spec)
		}
	}
	if len(unreferenced) > 0 {
		sortSpecs(unreferenced)
		result = append(result, EffectiveCatalog{
			Key: DefaultCatalogKey, Title: DefaultCatalogTitle,
			Implicit: true, Specs: unreferenced,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return Topology{Catalogs: result}, nil
}

func sortSpecs(specs []Spec) {
	sort.Slice(specs, func(i, j int) bool { return specs[i].Key < specs[j].Key })
}
