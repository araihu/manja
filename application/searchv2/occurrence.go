package searchv2

import "fmt"

// RecordLocation supplies the catalog-specific navigation metadata deliberately
// excluded from reusable semantic runs.
type RecordLocation struct {
	PageHref     string
	FragmentHref string
}

// NewCatalogRecord creates the navigation record for any effective catalog,
// including an explicitly declared empty catalog. An implicit default catalog
// can only reach this function when topology resolution gave it members.
func NewCatalogRecord(catalog EffectiveCatalog, pageHref string) (Record, error) {
	if err := validateEffectiveCatalog(catalog); err != nil {
		return Record{}, err
	}
	return NewRecord(RecordInput{
		Kind: KindCatalog, CatalogKey: catalog.Key, CatalogTitle: catalog.Title,
		LogicalKey: catalog.Key, Title: catalog.Title, PageHref: pageHref,
	})
}

// ExpandSemanticRun creates catalog-specific occurrences without changing or
// re-analyzing the reusable semantic run. Each semantic record must have one
// explicit location; callers invoke this once for every catalog membership.
func ExpandSemanticRun(catalog EffectiveCatalog, run SemanticRun, locations map[SemanticRecordID]RecordLocation) ([]Record, error) {
	if err := validateEffectiveCatalog(catalog); err != nil {
		return nil, err
	}
	if err := run.validate(); err != nil {
		return nil, err
	}
	if !catalogContainsSpec(catalog, run.SpecKey) {
		return nil, fmt.Errorf("catalog %q does not contain spec %q", catalog.Key, run.SpecKey)
	}
	if len(locations) != len(run.Records) {
		return nil, fmt.Errorf("spec %q has %d semantic records but %d locations", run.SpecKey, len(run.Records), len(locations))
	}

	records := make([]Record, 0, len(run.Records))
	for _, semantic := range run.Records {
		location, ok := locations[semantic.ID]
		if !ok {
			return nil, fmt.Errorf("semantic search record %q has no deployment location", semantic.ID)
		}
		record, err := NewRecord(RecordInput{
			Kind: semantic.Kind, CatalogKey: catalog.Key, CatalogTitle: catalog.Title,
			SpecKey: semantic.SpecKey, SpecTitle: semantic.SpecTitle,
			LogicalKey: semantic.LogicalKey, Title: semantic.Title, Description: semantic.Description,
			OperationID: semantic.OperationID, Method: semantic.Method, Path: semantic.Path,
			PageHref: location.PageHref, FragmentHref: location.FragmentHref,
		})
		if err != nil {
			return nil, fmt.Errorf("expand semantic search record %q: %w", semantic.ID, err)
		}
		records = append(records, record)
	}
	sortRecords(records)
	return records, nil
}

func validateEffectiveCatalog(catalog EffectiveCatalog) error {
	if catalog.Key == DefaultCatalogKey && !catalog.Implicit {
		return fmt.Errorf("catalog key %q is reserved for implicit assignment", DefaultCatalogKey)
	}
	if catalog.Implicit && catalog.Key != DefaultCatalogKey {
		return fmt.Errorf("implicit catalog must use key %q", DefaultCatalogKey)
	}
	if catalog.Implicit && len(catalog.Specs) == 0 {
		return fmt.Errorf("implicit default catalog must not be empty")
	}
	topology, err := ResolveTopology(
		[]DeclaredCatalog{{Key: catalog.Key, Title: catalog.Title, SpecKeys: specKeys(catalog.Specs)}},
		catalog.Specs,
	)
	if catalog.Implicit {
		// Resolve as an undeclared topology because "default" is reserved.
		topology, err = ResolveTopology(nil, catalog.Specs)
	}
	if err != nil {
		return err
	}
	if len(topology.Catalogs) != 1 || topology.Catalogs[0].Key != catalog.Key {
		return fmt.Errorf("effective catalog %q is not canonical", catalog.Key)
	}
	canonical := topology.Catalogs[0]
	if canonical.Title != catalog.Title || canonical.Implicit != catalog.Implicit || len(canonical.Specs) != len(catalog.Specs) {
		return fmt.Errorf("effective catalog %q is not canonical", catalog.Key)
	}
	for index := range canonical.Specs {
		if canonical.Specs[index] != catalog.Specs[index] {
			return fmt.Errorf("effective catalog %q members are not canonically ordered", catalog.Key)
		}
	}
	return nil
}

func catalogContainsSpec(catalog EffectiveCatalog, specKey string) bool {
	for _, spec := range catalog.Specs {
		if spec.Key == specKey {
			return true
		}
	}
	return false
}

func specKeys(specs []Spec) []string {
	keys := make([]string, len(specs))
	for index := range specs {
		keys[index] = specs[index].Key
	}
	return keys
}
