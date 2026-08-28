package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	searchVersion               = 1
	maxSearchSegments           = 16
	maxSearchTokenSegments      = 8
	maxSearchTrigramSegments    = 4
	maxSearchDecodedBytes       = 2 << 20
	maxSearchPostings           = 10_000
	maxSearchPostingsPerSegment = 2_000
	maxSearchResults            = 20
	maxSearchSnippetScalars     = 64
	maxSearchTrigramsPerToken   = 3
)

type searchBuildRecord struct {
	record SearchRecordV1
	tokens []string
	exact  []searchExactKey
}

type searchExactKey struct {
	value    string
	priority uint8
}

type exactSearchEntry struct {
	entry  SearchExactEntryV1
	digest string
}

func BuildSearchArtifacts(directory CatalogArtifactV1, bounds Bounds) (SearchArtifacts, error) {
	return buildSearchArtifacts(directory, bounds, true)
}

func BuildSearchArtifactsWithoutResourceLimits(directory CatalogArtifactV1, bounds Bounds) (SearchArtifacts, error) {
	return buildSearchArtifacts(directory, bounds, false)
}

func buildSearchArtifacts(directory CatalogArtifactV1, bounds Bounds, resourceLimits bool) (SearchArtifacts, error) {
	builders := make([]searchBuildRecord, 0)
	type schemaSearchOccurrence struct {
		documentKey string
		schema      SchemaDirectoryV1
	}
	schemaGroups := make(map[string][]schemaSearchOccurrence)
	for _, document := range directory.Documents {
		for _, operation := range document.Operations {
			values := []string{operation.Title, operation.OperationID, operation.Method, operation.Path, document.Key, searchSnippet(operation.Description)}
			values = append(values, operation.Tags...)
			for _, facet := range operation.Facets {
				values = append(values, facet.Name, facet.Value)
			}
			builders = append(builders, searchBuildRecord{
				record: SearchRecordV1{
					DetailID: operation.DetailID, DocumentKey: document.Key, Kind: "operation",
					Title: operation.Title, Description: searchSnippet(operation.Description), Href: operation.Href,
					OperationID: operation.OperationID, Method: operation.Method, Path: operation.Path,
					Occurrences: 1, Documents: []string{document.Key},
				},
				tokens: searchTokenSet(values...),
				exact: []searchExactKey{
					{value: string(operation.DetailID), priority: 1},
					{value: operation.OperationID, priority: 2},
					{value: operation.Path, priority: 2},
					{value: operation.Method + " " + operation.Path, priority: 2},
				},
			})
		}
		for _, schema := range document.Schemas {
			groupKey := schema.Name + "\x00" + schema.CanonicalSHA256 + "\x00" + schema.ProjectionSHA256 + "\x00" + string(directory.ProfileID)
			if schema.CanonicalSHA256 == "" || schema.ProjectionSHA256 == "" {
				groupKey += "\x00" + string(schema.DetailID)
			}
			schemaGroups[groupKey] = append(schemaGroups[groupKey], schemaSearchOccurrence{documentKey: document.Key, schema: schema})
		}
	}
	schemaGroupKeys := make([]string, 0, len(schemaGroups))
	for key := range schemaGroups {
		schemaGroupKeys = append(schemaGroupKeys, key)
	}
	sort.Strings(schemaGroupKeys)
	for _, groupKey := range schemaGroupKeys {
		occurrences := schemaGroups[groupKey]
		sort.Slice(occurrences, func(i, j int) bool {
			if occurrences[i].documentKey == occurrences[j].documentKey {
				return occurrences[i].schema.DetailID < occurrences[j].schema.DetailID
			}
			return occurrences[i].documentKey < occurrences[j].documentKey
		})
		canonical := occurrences[0]
		documents := make([]string, 0, len(occurrences))
		exact := []searchExactKey{
			{value: canonical.schema.Name, priority: 2},
			{value: shortSchemaName(canonical.schema.Name), priority: 2},
		}
		tokenValues := []string{canonical.schema.Name, shortSchemaName(canonical.schema.Name), searchSnippet(canonical.schema.Description)}
		for _, occurrence := range occurrences {
			documents = append(documents, occurrence.documentKey)
			tokenValues = append(tokenValues, occurrence.documentKey)
			exact = append(exact, searchExactKey{value: string(occurrence.schema.DetailID), priority: 1})
		}
		builders = append(builders, searchBuildRecord{
			record: SearchRecordV1{
				DetailID: canonical.schema.DetailID, DocumentKey: canonical.documentKey, Kind: "schema", Title: canonical.schema.Name,
				Description: searchSnippet(canonical.schema.Description), Href: canonical.schema.Href, SchemaName: canonical.schema.Name,
				Occurrences: uint32(len(occurrences)), Documents: documents,
			},
			tokens: searchTokenSet(tokenValues...),
			exact:  exact,
		})
	}
	sort.Slice(builders, func(i, j int) bool { return builders[i].record.DetailID < builders[j].record.DetailID })

	records := make([]SearchRecordV1, len(builders))
	ranks := make([]SearchRankRecordV1, len(builders))
	exactMatches := make(map[string]map[uint32]uint8)
	tokenPostings := make(map[string][]uint32)
	trigramPostings := make(map[string][]uint32)
	for index, builder := range builders {
		recordID := uint32(index)
		records[index] = builder.record
		ranks[index] = SearchRankRecordV1{Title: builder.record.Title, Kind: builder.record.Kind}
		for _, key := range builder.exact {
			normalized, err := normalizeSearchExact(key.value)
			if err != nil {
				if key.value == string(builder.record.DetailID) {
					return SearchArtifacts{}, err
				}
				continue
			}
			if exactMatches[normalized] == nil {
				exactMatches[normalized] = make(map[uint32]uint8)
			}
			priority, exists := exactMatches[normalized][recordID]
			if !exists || key.priority < priority {
				exactMatches[normalized][recordID] = key.priority
			}
		}
		for _, token := range builder.tokens {
			tokenPostings[token] = append(tokenPostings[token], recordID)
			for _, trigram := range searchTrigrams(token) {
				trigramPostings[trigram] = appendUniqueRecord(trigramPostings[trigram], recordID)
			}
		}
	}

	children := make([]ChildArtifact, 0)
	usage := BudgetUsage{}
	exactBuckets, exactChildren, exactUsage, err := buildExactSearchSegments(exactMatches, bounds.PostingSegmentBytes)
	if err != nil {
		return SearchArtifacts{}, err
	}
	children = append(children, exactChildren...)
	mergeSearchUsage(&usage, exactUsage)

	tokenRoutes, postingReferences, postingChildren, postingUsage, err := buildPostingSearchSegments("postings", "search-posting", tokenPostings, bounds.PostingSegmentBytes)
	if err != nil {
		return SearchArtifacts{}, err
	}
	children = append(children, postingChildren...)
	mergeSearchUsage(&usage, postingUsage)

	trigramRoutes, trigramReferences, trigramChildren, trigramUsage, err := buildPostingSearchSegments("trigrams", "search-trigram", trigramPostings, bounds.PostingSegmentBytes)
	if err != nil {
		return SearchArtifacts{}, err
	}
	children = append(children, trigramChildren...)
	mergeSearchUsage(&usage, trigramUsage)

	recordReferences, recordChildren, recordUsage, err := buildSearchRecordSegments(records, bounds.PostingSegmentBytes)
	if err != nil {
		return SearchArtifacts{}, err
	}
	children = append(children, recordChildren...)
	mergeSearchUsage(&usage, recordUsage)

	directoryValue := SearchDirectoryV1{
		SchemaVersion: 1, SearchVersion: searchVersion,
		ExactBuckets: exactBuckets, TokenRoutes: tokenRoutes, TrigramRoutes: trigramRoutes,
		PostingSegments: postingReferences, TrigramSegments: trigramReferences, RecordSegments: recordReferences,
		Ranks: ranks,
	}
	directoryBytes, err := json.Marshal(directoryValue)
	if err != nil {
		return SearchArtifacts{}, fmt.Errorf("encode search directory: %w", err)
	}
	directoryChild, err := newChild("search/directory.json", "search-directory", directoryBytes)
	if err != nil {
		return SearchArtifacts{}, err
	}
	children = append(children, directoryChild)
	usage.Children++
	usage.SearchBytes += directoryChild.Length
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	if resourceLimits && usage.SearchBytes > bounds.SearchBytes {
		return SearchArtifacts{}, fmt.Errorf("search usage %d exceeds %d", usage.SearchBytes, bounds.SearchBytes)
	}
	return SearchArtifacts{Directory: directoryValue, Children: children, Usage: usage}, nil
}

func buildExactSearchSegments(matches map[string]map[uint32]uint8, segmentLimit uint64) ([]SearchExactBucketReferenceV1, []ChildArtifact, BudgetUsage, error) {
	// Keep the complete digest alongside each entry so that an oversized
	// first-level bucket can be split without re-hashing or materializing one
	// monolithic segment. Prefixes are deliberately variable length: ordinary
	// buckets retain the compact one-nibble route, while only hot buckets are
	// recursively sharded by subsequent digest nibbles.
	buckets := make(map[string][]exactSearchEntry)
	for key, byRecord := range matches {
		digest := sha256.Sum256([]byte(key))
		digestHex := hex.EncodeToString(digest[:])
		prefix := digestHex[:1]
		entry := SearchExactEntryV1{Key: key, Matches: make([]SearchExactMatchV1, 0, len(byRecord))}
		for record, priority := range byRecord {
			entry.Matches = append(entry.Matches, SearchExactMatchV1{Record: record, Priority: priority})
		}
		sort.Slice(entry.Matches, func(i, j int) bool {
			if entry.Matches[i].Priority == entry.Matches[j].Priority {
				return entry.Matches[i].Record < entry.Matches[j].Record
			}
			return entry.Matches[i].Priority < entry.Matches[j].Priority
		})
		buckets[prefix] = append(buckets[prefix], exactSearchEntry{entry: entry, digest: digestHex})
	}
	type exactSearchArtifact struct {
		reference SearchExactBucketReferenceV1
		child     ChildArtifact
	}
	artifacts := make([]exactSearchArtifact, 0, len(buckets))
	usage := BudgetUsage{}
	// Emit recursively by digest nibble, then sort the complete reference/child
	// pairs as a final canonicalization step. The explicit sort keeps the wire
	// directory valid even if the partition walk changes in the future, while
	// retaining the child alignment used by callers that consume this helper.
	var emit func(string, []exactSearchEntry) error
	emit = func(prefix string, entries []exactSearchEntry) error {
		sort.Slice(entries, func(i, j int) bool { return entries[i].entry.Key < entries[j].entry.Key })
		estimated, err := exactSearchSegmentSize(entries)
		if err != nil {
			return err
		}
		if estimated <= segmentLimit {
			plainEntries := make([]SearchExactEntryV1, len(entries))
			for index, candidate := range entries {
				plainEntries[index] = candidate.entry
			}
			value := SearchExactSegmentV1{SchemaVersion: 1, SearchVersion: searchVersion, Entries: plainEntries}
			encoded, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("encode exact search segment: %w", err)
			}
			// The size preflight is exact for encoding/json's canonical object
			// shape, but retain this guard so future model/tag changes fail closed.
			if uint64(len(encoded)) > segmentLimit {
				return fmt.Errorf("exact search bucket %q exceeds segment bytes", prefix)
			}
			child, err := contentAddressedSearchChild("exact", "search-exact", encoded)
			if err != nil {
				return err
			}
			postings := uint32(0)
			for _, entry := range plainEntries {
				postings += uint32(len(entry.Matches))
			}
			artifacts = append(artifacts, exactSearchArtifact{
				reference: SearchExactBucketReferenceV1{Prefix: prefix, SearchSegmentReferenceV1: searchSegmentReference(child, uint32(len(plainEntries)), postings)},
				child:     child,
			})
			addSearchChildUsage(&usage, child)
			return nil
		}

		if len(prefix) >= sha256.Size*2 {
			if len(entries) == 1 {
				return fmt.Errorf("exact search entry %q exceeds segment bytes", entries[0].entry.Key)
			}
			return fmt.Errorf("exact search bucket %q exceeds segment bytes", prefix)
		}
		childrenByNibble := make([][]exactSearchEntry, 16)
		for _, candidate := range entries {
			nibble := strings.IndexByte("0123456789abcdef", candidate.digest[len(prefix)])
			if nibble < 0 {
				return fmt.Errorf("exact search digest for %q is invalid", candidate.entry.Key)
			}
			childrenByNibble[nibble] = append(childrenByNibble[nibble], candidate)
		}
		for nibble, childEntries := range childrenByNibble {
			if len(childEntries) == 0 {
				continue
			}
			if err := emit(prefix+string("0123456789abcdef"[nibble]), childEntries); err != nil {
				return err
			}
		}
		return nil
	}
	prefixes := make([]string, 0, len(buckets))
	for prefix := range buckets {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		if err := emit(prefix, buckets[prefix]); err != nil {
			return nil, nil, BudgetUsage{}, err
		}
	}
	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].reference.Prefix == artifacts[j].reference.Prefix {
			return artifacts[i].child.Path < artifacts[j].child.Path
		}
		return artifacts[i].reference.Prefix < artifacts[j].reference.Prefix
	})
	references := make([]SearchExactBucketReferenceV1, len(artifacts))
	children := make([]ChildArtifact, len(artifacts))
	for index, artifact := range artifacts {
		references[index] = artifact.reference
		children[index] = artifact.child
	}
	return references, children, usage, nil
}

// exactSearchSegmentSize computes the encoded size without first allocating a
// byte slice for the whole bucket. This matters for large catalogs: an
// overloaded bucket is split by digest prefix before a potentially multi-MiB
// JSON value is ever materialized.
func exactSearchSegmentSize(entries []exactSearchEntry) (uint64, error) {
	empty, err := json.Marshal(SearchExactSegmentV1{SchemaVersion: 1, SearchVersion: searchVersion, Entries: []SearchExactEntryV1{}})
	if err != nil {
		return 0, fmt.Errorf("encode exact search segment envelope: %w", err)
	}
	if len(empty) < 2 {
		return 0, fmt.Errorf("exact search segment envelope is invalid")
	}
	total := uint64(len(empty) - 2) // replace the empty [] with joined entries.
	for index, candidate := range entries {
		encoded, err := json.Marshal(candidate.entry)
		if err != nil {
			return 0, fmt.Errorf("encode exact search entry: %w", err)
		}
		if index > 0 {
			total++ // comma between entries.
		}
		if uint64(len(encoded)) > ^uint64(0)-total {
			return 0, fmt.Errorf("exact search segment size overflows")
		}
		total += uint64(len(encoded))
	}
	return total, nil
}

func buildPostingSearchSegments(directoryName, kind string, postings map[string][]uint32, segmentLimit uint64) ([]SearchPostingRouteV1, []SearchSegmentReferenceV1, []ChildArtifact, BudgetUsage, error) {
	entries := make([]SearchPostingEntryV1, 0, len(postings))
	for key, records := range postings {
		entries = append(entries, SearchPostingEntryV1{Key: key, Records: records})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	routes := make([]SearchPostingRouteV1, 0, len(entries))
	references := make([]SearchSegmentReferenceV1, 0)
	children := make([]ChildArtifact, 0)
	usage := BudgetUsage{}
	for start := 0; start < len(entries); {
		end := start
		var encoded []byte
		postingCount := 0
		for end < len(entries) {
			candidatePostings := postingCount + len(entries[end].Records)
			if end > start && candidatePostings > maxSearchPostingsPerSegment {
				break
			}
			candidate, err := json.Marshal(SearchPostingSegmentV1{SchemaVersion: 1, SearchVersion: searchVersion, Entries: entries[start : end+1]})
			if err != nil {
				return nil, nil, nil, BudgetUsage{}, fmt.Errorf("encode %s search segment: %w", directoryName, err)
			}
			if uint64(len(candidate)) > segmentLimit {
				if end == start {
					return nil, nil, nil, BudgetUsage{}, fmt.Errorf("search posting %q exceeds segment bytes", entries[start].Key)
				}
				break
			}
			encoded = candidate
			postingCount = candidatePostings
			end++
		}
		if end == start || len(encoded) == 0 {
			return nil, nil, nil, BudgetUsage{}, fmt.Errorf("search posting partition made no progress")
		}
		if len(references) >= int(^uint16(0)) {
			return nil, nil, nil, BudgetUsage{}, fmt.Errorf("search posting segment ordinal overflow")
		}
		child, err := contentAddressedSearchChild(directoryName, kind, encoded)
		if err != nil {
			return nil, nil, nil, BudgetUsage{}, err
		}
		ordinal := uint16(len(references))
		encodedPostings := uint32(0)
		for _, entry := range entries[start:end] {
			routes = append(routes, SearchPostingRouteV1{Key: entry.Key, Segment: ordinal})
			encodedPostings += uint32(len(entry.Records))
		}
		references = append(references, searchSegmentReference(child, uint32(end-start), encodedPostings))
		children = append(children, child)
		addSearchChildUsage(&usage, child)
		start = end
	}
	return routes, references, children, usage, nil
}

func buildSearchRecordSegments(records []SearchRecordV1, segmentLimit uint64) ([]SearchRecordSegmentReferenceV1, []ChildArtifact, BudgetUsage, error) {
	references := make([]SearchRecordSegmentReferenceV1, 0)
	children := make([]ChildArtifact, 0)
	usage := BudgetUsage{}
	for start := 0; start < len(records); {
		end := start
		var encoded []byte
		for end < len(records) {
			candidate, err := json.Marshal(SearchRecordSegmentV1{SchemaVersion: 1, SearchVersion: searchVersion, FirstRecord: uint32(start), Records: records[start : end+1]})
			if err != nil {
				return nil, nil, BudgetUsage{}, fmt.Errorf("encode search record segment: %w", err)
			}
			if uint64(len(candidate)) > segmentLimit {
				if end == start {
					return nil, nil, BudgetUsage{}, fmt.Errorf("search record %q exceeds segment bytes", records[start].DetailID)
				}
				break
			}
			encoded = candidate
			end++
		}
		if end == start || len(encoded) == 0 {
			return nil, nil, BudgetUsage{}, fmt.Errorf("search record partition made no progress")
		}
		child, err := contentAddressedSearchChild("records", "search-record", encoded)
		if err != nil {
			return nil, nil, BudgetUsage{}, err
		}
		references = append(references, SearchRecordSegmentReferenceV1{
			Path: child.Path, FirstRecord: uint32(start), Records: uint32(end - start), Length: child.Length, SHA256: child.SHA256,
		})
		children = append(children, child)
		addSearchChildUsage(&usage, child)
		start = end
	}
	return references, children, usage, nil
}

func contentAddressedSearchChild(directoryName, kind string, encoded []byte) (ChildArtifact, error) {
	digest := sha256.Sum256(encoded)
	return newChild("search/"+directoryName+"/"+hex.EncodeToString(digest[:])+".json", kind, encoded)
}

func searchSegmentReference(child ChildArtifact, entries, postings uint32) SearchSegmentReferenceV1 {
	return SearchSegmentReferenceV1{Path: child.Path, Entries: entries, Postings: postings, Length: child.Length, SHA256: child.SHA256}
}

func addSearchChildUsage(usage *BudgetUsage, child ChildArtifact) {
	usage.Children++
	usage.SearchBytes += child.Length
	if child.Length > usage.PostingSegmentBytes {
		usage.PostingSegmentBytes = child.Length
	}
}

func mergeSearchUsage(total *BudgetUsage, next BudgetUsage) {
	total.Children += next.Children
	total.SearchBytes += next.SearchBytes
	if next.PostingSegmentBytes > total.PostingSegmentBytes {
		total.PostingSegmentBytes = next.PostingSegmentBytes
	}
}

func appendUniqueRecord(records []uint32, value uint32) []uint32 {
	if len(records) == 0 || records[len(records)-1] != value {
		return append(records, value)
	}
	return records
}

func searchTokenSet(values ...string) []string {
	seen := make(map[string]struct{})
	for _, value := range values {
		value = sanitizeSearchText(value)
		value = norm.NFKC.String(value)
		for _, piece := range splitSearchIndexWords(value) {
			piece = cases.Fold().String(piece)
			if piece != "" {
				seen[piece] = struct{}{}
			}
		}
		value = cases.Fold().String(value)
		for _, token := range tokenizeSearchText(value) {
			if token != "" {
				seen[token] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for token := range seen {
		result = append(result, token)
	}
	sort.Strings(result)
	return result
}

func splitSearchIndexWords(value string) []string {
	runes := []rune(value)
	words := make([]string, 0)
	start := -1
	flush := func(end int) {
		if start >= 0 && end > start {
			words = append(words, string(runes[start:end]))
		}
		start = -1
	}
	for index, character := range runes {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) {
			flush(index)
			continue
		}
		if start < 0 {
			start = index
			continue
		}
		previous := runes[index-1]
		boundary := unicode.IsLower(previous) && unicode.IsUpper(character) || unicode.IsDigit(previous) != unicode.IsDigit(character)
		if !boundary && unicode.IsUpper(previous) && unicode.IsUpper(character) && index+1 < len(runes) && unicode.IsLower(runes[index+1]) {
			boundary = true
		}
		if boundary {
			flush(index)
			start = index
		}
	}
	flush(len(runes))
	return words
}

func searchTrigrams(value string) []string {
	runes := []rune(value)
	if len(runes) < 3 {
		return nil
	}
	count := len(runes) - 2
	positions := []int{0}
	if count > 1 {
		positions = append(positions, count/2)
	}
	if count > 2 {
		positions = append(positions, count-1)
	}
	seen := make(map[string]struct{}, maxSearchTrigramsPerToken)
	result := make([]string, 0, maxSearchTrigramsPerToken)
	for _, position := range positions {
		if position < 0 || position+3 > len(runes) {
			continue
		}
		trigram := string(runes[position : position+3])
		if _, exists := seen[trigram]; exists {
			continue
		}
		seen[trigram] = struct{}{}
		result = append(result, trigram)
	}
	sort.Strings(result)
	return result
}

func sanitizeSearchText(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if unicode.IsControl(character) {
			builder.WriteByte(' ')
		} else {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func searchSnippet(value string) string {
	value = strings.Join(strings.Fields(sanitizeSearchText(value)), " ")
	if utf8.RuneCountInString(value) <= maxSearchSnippetScalars {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxSearchSnippetScalars])
}

func shortSchemaName(value string) string {
	if index := strings.LastIndexAny(value, ".:/"); index >= 0 && index+1 < len(value) {
		return value[index+1:]
	}
	return value
}
