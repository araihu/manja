package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidQuery   = errors.New("invalid_query")
	ErrQueryTooBroad  = errors.New("query_too_broad")
	ErrSearchDeadline = errors.New("search_deadline")
)

type SearchResult struct {
	CatalogID       string
	SnapshotID      SnapshotID
	SearchVersion   uint32
	Query           string
	Results         []SearchRecordV1
	MatchQualities  []SearchMatchQuality
	PostingsScanned uint64
	SegmentsDecoded uint64
	BytesDecoded    uint64
	Duration        time.Duration
}

// SearchMatchQuality is the relevance class retained by catalog-local search
// and consumed by deployment-wide ranking. Context boosts may reorder results
// within a class, but must never let a weaker class outrank a stronger one.
type SearchMatchQuality uint8

const (
	SearchMatchDescription   SearchMatchQuality = 1
	SearchMatchFuzzy         SearchMatchQuality = 2
	SearchMatchPrefix        SearchMatchQuality = 3
	SearchMatchExactToken    SearchMatchQuality = 4
	SearchMatchExactIdentity SearchMatchQuality = 5
)

// CanonicalSearchQuery is a validated query normalized with the search
// service's canonical UTF-8, bounds, control-character, and NFKC rules.
type CanonicalSearchQuery struct {
	exact string
}

// CanonicalizeSearchQuery validates and normalizes one caller-supplied query.
func CanonicalizeSearchQuery(input string) (CanonicalSearchQuery, error) {
	exact, err := normalizeSearchExact(input)
	if err != nil {
		return CanonicalSearchQuery{}, err
	}
	return CanonicalSearchQuery{exact: exact}, nil
}

// String returns the validated canonical query value.
func (query CanonicalSearchQuery) String() string {
	return query.exact
}

type SearchService struct {
	catalogID      string
	snapshotID     SnapshotID
	directory      SearchDirectoryV1
	children       map[string]ChildArtifact
	recordSegments map[string]searchRecordSegmentCacheEntry
	child          func(context.Context, string, string, uint64, string) (ChildArtifact, error)
	deadline       time.Duration
}

type searchRecordSegmentCacheEntry struct {
	reference SearchRecordSegmentReferenceV1
	segment   SearchRecordSegmentV1
}

func NewSearchService(snapshot CompiledSnapshot) (*SearchService, error) {
	children := make(map[string]ChildArtifact, len(snapshot.Children))
	for _, child := range snapshot.Children {
		if _, duplicate := children[child.Path]; duplicate {
			return nil, fmt.Errorf("search child path %q is duplicated", child.Path)
		}
		children[child.Path] = child
	}
	directoryChild, exists := children[snapshot.Directory.SearchChild]
	if !exists || directoryChild.Kind != "search-directory" {
		return nil, fmt.Errorf("search directory child is missing")
	}
	if err := verifySearchChild(directoryChild, directoryChild.Length, directoryChild.SHA256); err != nil {
		return nil, err
	}
	var directory SearchDirectoryV1
	if err := decodeCanonicalSearchChild(directoryChild.Bytes, &directory); err != nil {
		return nil, fmt.Errorf("decode search directory: %w", err)
	}
	if directory.SchemaVersion != 1 || directory.SearchVersion != searchVersion {
		return nil, fmt.Errorf("search directory version is unsupported")
	}
	if err := validateSearchDirectory(directory); err != nil {
		return nil, err
	}
	service, err := newSearchService(snapshot.Directory.CatalogID, snapshot.ID, directory, nil)
	if err != nil {
		return nil, err
	}
	service.children = children
	service.recordSegments, err = prepareSearchRecordSegments(children, directory.RecordSegments)
	if err != nil {
		return nil, err
	}
	return service, nil
}

type RuntimeSearchChildLoader func(context.Context, string) ([]byte, ChildIdentityV1, error)

func NewRuntimeSearchService(snapshot RuntimeSnapshot, cache *ByteCache, loader RuntimeSearchChildLoader) (*SearchService, error) {
	if cache == nil || loader == nil {
		return nil, fmt.Errorf("runtime search cache and child loader are required")
	}
	return newSearchService(snapshot.Directory.CatalogID, snapshot.ID, snapshot.Search, func(ctx context.Context, pathValue, kind string, length uint64, digest string) (ChildArtifact, error) {
		digestBytes, err := hex.DecodeString(digest)
		if err != nil || len(digestBytes) != sha256.Size {
			return ChildArtifact{}, fmt.Errorf("search child %q digest is invalid", pathValue)
		}
		var digestKey [sha256.Size]byte
		copy(digestKey[:], digestBytes)
		value, err := cache.Load(ctx, CacheKey{SnapshotID: snapshot.ID, Digest: digestKey}, length, 128,
			func(loadContext context.Context) ([]byte, error) {
				data, identity, err := loader(loadContext, pathValue)
				if err != nil {
					return nil, err
				}
				if identity.Path != pathValue || identity.Kind != kind || identity.Length != length || identity.SHA256 != digest {
					return nil, fmt.Errorf("search child %q metadata differs", pathValue)
				}
				return data, nil
			},
			func(data []byte) (any, uint64, error) {
				return ChildArtifact{Path: pathValue, Kind: kind, Length: length, SHA256: digest, Bytes: data}, 128, nil
			},
		)
		if err != nil {
			return ChildArtifact{}, err
		}
		child, ok := value.(ChildArtifact)
		if !ok {
			return ChildArtifact{}, fmt.Errorf("search child %q cache type is invalid", pathValue)
		}
		return child, nil
	})
}

func newSearchService(catalogID string, snapshotID SnapshotID, directory SearchDirectoryV1, child func(context.Context, string, string, uint64, string) (ChildArtifact, error)) (*SearchService, error) {
	if directory.SchemaVersion != 1 || directory.SearchVersion != searchVersion {
		return nil, fmt.Errorf("search directory version is unsupported")
	}
	if err := validateSearchDirectory(directory); err != nil {
		return nil, err
	}
	return &SearchService{catalogID: catalogID, snapshotID: snapshotID, directory: directory, child: child, deadline: 100 * time.Millisecond}, nil
}

func (service *SearchService) Search(ctx context.Context, snapshot SnapshotID, query string) (SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	if snapshot != service.snapshotID {
		return SearchResult{}, fmt.Errorf("search snapshot %q is not active", snapshot)
	}
	canonical, err := CanonicalizeSearchQuery(query)
	if err != nil {
		return SearchResult{}, err
	}
	return service.SearchCanonical(ctx, snapshot, canonical)
}

// SearchCanonical searches with a query already produced by
// CanonicalizeSearchQuery, avoiding a second normalization pass.
func (service *SearchService) SearchCanonical(ctx context.Context, snapshot SnapshotID, query CanonicalSearchQuery) (SearchResult, error) {
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	if snapshot != service.snapshotID {
		return SearchResult{}, fmt.Errorf("search snapshot %q is not active", snapshot)
	}
	exact := query.String()
	if exact == "" {
		return SearchResult{}, fmt.Errorf("%w: canonical query is empty", ErrInvalidQuery)
	}
	searchContext, cancel := context.WithTimeout(ctx, service.deadline)
	defer cancel()

	receipt := searchLoadReceipt{loaded: make(map[string]struct{})}
	exactMatches, err := service.loadExactMatches(searchContext, exact, &receipt)
	if err != nil {
		return SearchResult{}, searchError(ctx, searchContext, err)
	}

	candidateIDs := make([]uint32, 0, len(exactMatches))
	exactPriority := make(map[uint32]uint8)
	matchQuality := make(map[uint32]SearchMatchQuality)
	if len(exactMatches) > 0 {
		for _, match := range exactMatches {
			candidateIDs = append(candidateIDs, match.Record)
			exactPriority[match.Record] = match.Priority
			matchQuality[match.Record] = SearchMatchExactIdentity
		}
		sort.Slice(candidateIDs, func(i, j int) bool { return candidateIDs[i] < candidateIDs[j] })
		candidateIDs = deduplicateRecordIDs(candidateIDs)
	}
	// A schema name can be an exact hit while an operation title is the more useful
	// navigational result. Merge token candidates instead of letting exact schemas
	// hide matching operations.
	normalized, normalizeErr := tokenizeNormalizedSearchExact(exact)
	if normalizeErr != nil {
		if len(exactMatches) == 0 {
			return SearchResult{}, normalizeErr
		}
	} else {
		tokenCandidates, anyPrefix, anyFuzzy, tokenErr := service.loadRankedCandidateIDs(searchContext, normalized.Tokens, &receipt)
		if tokenErr != nil {
			if len(exactMatches) == 0 || !errors.Is(tokenErr, ErrQueryTooBroad) {
				return SearchResult{}, searchError(ctx, searchContext, tokenErr)
			}
		} else {
			quality := SearchMatchExactToken
			if anyFuzzy {
				quality = SearchMatchFuzzy
			} else if anyPrefix {
				quality = SearchMatchPrefix
			}
			for _, recordID := range tokenCandidates {
				if matchQuality[recordID] < quality {
					matchQuality[recordID] = quality
				}
			}
			merged := unionRecordIDs(candidateIDs, tokenCandidates)
			if len(merged) > maxSearchPostings {
				if len(exactMatches) == 0 {
					return SearchResult{}, fmt.Errorf("%w: candidate records", ErrQueryTooBroad)
				}
			} else {
				candidateIDs = merged
			}
		}
	}
	if len(candidateIDs) > maxSearchPostings {
		return SearchResult{}, fmt.Errorf("%w: candidate records", ErrQueryTooBroad)
	}

	type rankedSearchID struct {
		recordID uint32
		kind     uint8
		priority uint8
		quality  SearchMatchQuality
		title    bool
	}
	ranked := make([]rankedSearchID, 0, len(candidateIDs))
	for _, recordID := range candidateIDs {
		if err := searchContext.Err(); err != nil {
			return SearchResult{}, searchError(ctx, searchContext, err)
		}
		if int(recordID) >= len(service.directory.Ranks) {
			return SearchResult{}, fmt.Errorf("search rank record ordinal %d is invalid", recordID)
		}
		title, _ := normalizeSearchExact(service.directory.Ranks[recordID].Title)
		ranked = append(ranked, rankedSearchID{
			recordID: recordID,
			kind:     searchKindPriority(service.directory.Ranks[recordID].Kind),
			priority: exactPriority[recordID],
			quality:  matchQuality[recordID],
			title:    title == exact,
		})
	}
	sort.Slice(ranked, func(i, j int) bool {
		left, right := ranked[i], ranked[j]
		if left.quality != right.quality {
			return left.quality > right.quality
		}
		if left.priority != right.priority {
			if left.priority == 0 {
				return false
			}
			if right.priority == 0 {
				return true
			}
			return left.priority < right.priority
		}
		if left.kind != right.kind {
			return left.kind < right.kind
		}
		if left.title != right.title {
			return left.title
		}
		leftTitle := service.directory.Ranks[left.recordID].Title
		rightTitle := service.directory.Ranks[right.recordID].Title
		if len(leftTitle) != len(rightTitle) {
			return len(leftTitle) < len(rightTitle)
		}
		return left.recordID < right.recordID
	})
	selectedIDs := make([]uint32, 0, min(len(ranked), maxSearchResults))
	for _, match := range ranked {
		selectedIDs = append(selectedIDs, match.recordID)
		if len(selectedIDs) == maxSearchResults {
			break
		}
	}
	recordLoadBase := cloneSearchLoadReceipt(receipt)
	results, err := service.loadSearchRecords(searchContext, selectedIDs, &receipt)
	if err != nil && len(exactMatches) > 0 && errors.Is(err, ErrQueryTooBroad) {
		selectedIDs = selectedIDs[:0]
		for _, match := range ranked {
			if match.quality != SearchMatchExactIdentity {
				continue
			}
			selectedIDs = append(selectedIDs, match.recordID)
			if len(selectedIDs) == maxSearchResults {
				break
			}
		}
		receipt = recordLoadBase
		results, err = service.loadSearchRecords(searchContext, selectedIDs, &receipt)
	}
	if err != nil {
		return SearchResult{}, searchError(ctx, searchContext, err)
	}
	qualities := make([]SearchMatchQuality, len(selectedIDs))
	for index, recordID := range selectedIDs {
		qualities[index] = matchQuality[recordID]
	}
	return SearchResult{
		CatalogID: service.catalogID, SnapshotID: service.snapshotID, SearchVersion: searchVersion,
		Query: exact, Results: results, MatchQualities: qualities, PostingsScanned: receipt.postings,
		SegmentsDecoded: receipt.segments, BytesDecoded: receipt.bytes, Duration: time.Since(started),
	}, nil
}

func searchKindPriority(kind string) uint8 {
	switch kind {
	case "operation":
		return 0
	case "schema":
		return 1
	default:
		return 2
	}
}

type searchLoadReceipt struct {
	postings       uint64
	segments       uint64
	recordSegments uint64
	bytes          uint64
	loaded         map[string]struct{}
}

func cloneSearchLoadReceipt(receipt searchLoadReceipt) searchLoadReceipt {
	cloned := receipt
	cloned.loaded = make(map[string]struct{}, len(receipt.loaded))
	for path := range receipt.loaded {
		cloned.loaded[path] = struct{}{}
	}
	return cloned
}

func (service *SearchService) loadExactMatches(ctx context.Context, key string, receipt *searchLoadReceipt) ([]SearchExactMatchV1, error) {
	digest := sha256.Sum256([]byte(key))
	digestHex := hex.EncodeToString(digest[:])
	reference, exists := exactSearchBucket(service.directory.ExactBuckets, digestHex)
	if !exists {
		return nil, nil
	}
	if err := reserveSearchReference(receipt, reference.SearchSegmentReferenceV1, maxSearchSegments); err != nil {
		return nil, err
	}
	child, err := service.searchChild(ctx, reference.Path, "search-exact", reference.Length, reference.SHA256)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var segment SearchExactSegmentV1
	if err := decodeCanonicalSearchChild(child.Bytes, &segment); err != nil {
		return nil, fmt.Errorf("decode exact search segment %q: %w", reference.Path, err)
	}
	if segment.SchemaVersion != 1 || segment.SearchVersion != searchVersion || len(segment.Entries) != int(reference.Entries) || countExactMatches(segment.Entries) != reference.Postings {
		return nil, fmt.Errorf("exact search segment %q is invalid", reference.Path)
	}
	entryIndex := sort.Search(len(segment.Entries), func(index int) bool { return segment.Entries[index].Key >= key })
	if entryIndex == len(segment.Entries) || segment.Entries[entryIndex].Key != key {
		return nil, nil
	}
	return append([]SearchExactMatchV1(nil), segment.Entries[entryIndex].Matches...), nil
}

// exactSearchBucket resolves the most-specific digest prefix. Older catalogs
// use one-nibble buckets; catalogs with a hot bucket may contain deeper
// prefixes (for example, "0a") only for the overloaded branch. Trying from
// longest to shortest keeps both directory shapes compatible and loads at most
// one exact segment for a query.
func exactSearchBucket(buckets []SearchExactBucketReferenceV1, digestHex string) (SearchExactBucketReferenceV1, bool) {
	for length := len(digestHex); length > 0; length-- {
		prefix := digestHex[:length]
		index := sort.Search(len(buckets), func(index int) bool {
			return buckets[index].Prefix >= prefix
		})
		if index < len(buckets) && buckets[index].Prefix == prefix {
			return buckets[index], true
		}
	}
	return SearchExactBucketReferenceV1{}, false
}

func (service *SearchService) loadRankedCandidateIDs(ctx context.Context, tokens []string, receipt *searchLoadReceipt) ([]uint32, bool, bool, error) {
	type routeGroup struct {
		keys   []string
		prefix bool
		fuzzy  bool
	}
	groups := make([]routeGroup, 0, len(tokens))
	postingOrdinals := make(map[uint16]struct{})
	for _, token := range tokens {
		routes := postingRoutes(service.directory.TokenRoutes, token, true)
		if len(routes) > 0 {
			group := routeGroup{keys: make([]string, 0, len(routes)), prefix: len(routes) != 1 || routes[0].Key != token}
			for _, route := range routes {
				group.keys = append(group.keys, route.Key)
				postingOrdinals[route.Segment] = struct{}{}
			}
			groups = append(groups, group)
			continue
		}
		routes = fuzzyPostingRoutes(service.directory.TokenRoutes, token, maxSearchFuzzyTokenRoutes)
		if len(routes) == 0 {
			return nil, false, false, nil
		}
		group := routeGroup{keys: make([]string, 0, len(routes)), fuzzy: true}
		for _, route := range routes {
			group.keys = append(group.keys, route.Key)
			postingOrdinals[route.Segment] = struct{}{}
		}
		groups = append(groups, group)
	}
	if len(postingOrdinals) > maxSearchTokenSegments {
		return nil, false, false, fmt.Errorf("%w: posting segment fanout", ErrQueryTooBroad)
	}
	postingEntries, err := service.loadPostingEntries(ctx, service.directory.PostingSegments, postingOrdinals, "search-posting", receipt)
	if err != nil {
		return nil, false, false, err
	}
	var candidates []uint32
	anyPrefix, anyFuzzy := false, false
	for groupIndex, group := range groups {
		var groupCandidates []uint32
		for _, key := range group.keys {
			groupCandidates = unionRecordIDs(groupCandidates, postingEntries[key])
		}
		if groupIndex == 0 {
			candidates = groupCandidates
		} else {
			candidates = intersectRecordIDs(candidates, groupCandidates)
		}
		anyPrefix = anyPrefix || group.prefix
		anyFuzzy = anyFuzzy || group.fuzzy
		if len(candidates) == 0 {
			break
		}
	}
	return candidates, anyPrefix, anyFuzzy, nil
}

func fuzzyPostingRoutes(routes []SearchPostingRouteV1, token string, limit int) []SearchPostingRouteV1 {
	tokenRunes := []rune(token)
	if len(tokenRunes) < 4 || limit <= 0 {
		return nil
	}
	maxDistance := 1
	if len(tokenRunes) >= 8 {
		maxDistance = 2
	}
	first := string(tokenRunes[:1])
	start := sort.Search(len(routes), func(index int) bool { return routes[index].Key >= first })
	type fuzzyRoute struct {
		route    SearchPostingRouteV1
		distance int
	}
	matches := make([]fuzzyRoute, 0, limit)
	for index := start; index < len(routes) && strings.HasPrefix(routes[index].Key, first); index++ {
		candidate := []rune(routes[index].Key)
		if difference := len(candidate) - len(tokenRunes); difference > maxDistance || difference < -maxDistance {
			continue
		}
		distance, matched := boundedDamerauLevenshtein(tokenRunes, candidate, maxDistance)
		if matched {
			matches = append(matches, fuzzyRoute{route: routes[index], distance: distance})
		}
	}
	sort.Slice(matches, func(left, right int) bool {
		if matches[left].distance != matches[right].distance {
			return matches[left].distance < matches[right].distance
		}
		return matches[left].route.Key < matches[right].route.Key
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	result := make([]SearchPostingRouteV1, len(matches))
	for index, match := range matches {
		result[index] = match.route
	}
	return result
}

func boundedDamerauLevenshtein(left, right []rune, limit int) (int, bool) {
	if difference := len(left) - len(right); difference > limit || difference < -limit {
		return limit + 1, false
	}
	previousPrevious := make([]int, len(right)+1)
	previous := make([]int, len(right)+1)
	for index := range previous {
		previous[index] = index
	}
	for leftIndex := 1; leftIndex <= len(left); leftIndex++ {
		current := make([]int, len(right)+1)
		current[0] = leftIndex
		for rightIndex := 1; rightIndex <= len(right); rightIndex++ {
			cost := 1
			if left[leftIndex-1] == right[rightIndex-1] {
				cost = 0
			}
			current[rightIndex] = min(
				previous[rightIndex]+1,
				current[rightIndex-1]+1,
				previous[rightIndex-1]+cost,
			)
			if leftIndex > 1 && rightIndex > 1 && left[leftIndex-1] == right[rightIndex-2] && left[leftIndex-2] == right[rightIndex-1] {
				current[rightIndex] = min(current[rightIndex], previousPrevious[rightIndex-2]+1)
			}
		}
		previousPrevious, previous = previous, current
	}
	distance := previous[len(right)]
	return distance, distance <= limit
}

func (service *SearchService) loadPostingEntries(ctx context.Context, references []SearchSegmentReferenceV1, ordinals map[uint16]struct{}, kind string, receipt *searchLoadReceipt) (map[string][]uint32, error) {
	ordered := make([]int, 0, len(ordinals))
	for ordinal := range ordinals {
		if int(ordinal) >= len(references) {
			return nil, fmt.Errorf("search posting segment ordinal is invalid")
		}
		ordered = append(ordered, int(ordinal))
	}
	sort.Ints(ordered)
	for _, ordinal := range ordered {
		if err := reserveSearchReference(receipt, references[ordinal], maxSearchSegments); err != nil {
			return nil, err
		}
	}
	result := make(map[string][]uint32)
	for _, ordinal := range ordered {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		reference := references[ordinal]
		child, err := service.searchChild(ctx, reference.Path, kind, reference.Length, reference.SHA256)
		if err != nil {
			return nil, err
		}
		var segment SearchPostingSegmentV1
		if err := decodeCanonicalSearchChild(child.Bytes, &segment); err != nil {
			return nil, fmt.Errorf("decode posting search segment %q: %w", reference.Path, err)
		}
		if segment.SchemaVersion != 1 || segment.SearchVersion != searchVersion || len(segment.Entries) != int(reference.Entries) || countPostingRecords(segment.Entries) != reference.Postings {
			return nil, fmt.Errorf("posting search segment %q is invalid", reference.Path)
		}
		for _, entry := range segment.Entries {
			result[entry.Key] = entry.Records
		}
	}
	return result, nil
}

func (service *SearchService) loadSearchRecords(ctx context.Context, recordIDs []uint32, receipt *searchLoadReceipt) ([]SearchRecordV1, error) {
	if len(recordIDs) == 0 {
		return []SearchRecordV1{}, nil
	}
	referenceIndexes := make(map[int]struct{})
	for _, recordID := range recordIDs {
		index := sort.Search(len(service.directory.RecordSegments), func(index int) bool {
			reference := service.directory.RecordSegments[index]
			return uint64(reference.FirstRecord)+uint64(reference.Records) > uint64(recordID)
		})
		if index == len(service.directory.RecordSegments) || recordID < service.directory.RecordSegments[index].FirstRecord {
			return nil, fmt.Errorf("search record ordinal %d is invalid", recordID)
		}
		referenceIndexes[index] = struct{}{}
	}
	ordered := make([]int, 0, len(referenceIndexes))
	for index := range referenceIndexes {
		ordered = append(ordered, index)
	}
	sort.Ints(ordered)
	for _, index := range ordered {
		reference := service.directory.RecordSegments[index]
		if err := reserveSearchRecordReference(receipt, reference); err != nil {
			return nil, err
		}
	}
	byID := make(map[uint32]SearchRecordV1, len(recordIDs))
	for _, index := range ordered {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		reference := service.directory.RecordSegments[index]
		child, err := service.searchChild(ctx, reference.Path, "search-record", reference.Length, reference.SHA256)
		if err != nil {
			return nil, err
		}
		cached, ok := service.recordSegments[reference.Path]
		segment := cached.segment
		if !ok || cached.reference != reference {
			segment, err = decodeSearchRecordSegment(child, reference)
			if err != nil {
				return nil, err
			}
		}
		for offset, record := range segment.Records {
			byID[segment.FirstRecord+uint32(offset)] = record
		}
	}
	result := make([]SearchRecordV1, 0, len(recordIDs))
	for _, recordID := range recordIDs {
		record, exists := byID[recordID]
		if !exists {
			return nil, fmt.Errorf("search record ordinal %d is missing", recordID)
		}
		result = append(result, record)
	}
	return result, nil
}

func prepareSearchRecordSegments(children map[string]ChildArtifact, references []SearchRecordSegmentReferenceV1) (map[string]searchRecordSegmentCacheEntry, error) {
	segments := make(map[string]searchRecordSegmentCacheEntry, len(references))
	for _, reference := range references {
		child, exists := children[reference.Path]
		if !exists || child.Kind != "search-record" {
			return nil, fmt.Errorf("search child %q is missing", reference.Path)
		}
		if err := verifySearchChild(child, reference.Length, reference.SHA256); err != nil {
			return nil, err
		}
		segment, err := decodeSearchRecordSegment(child, reference)
		if err != nil {
			return nil, err
		}
		segments[reference.Path] = searchRecordSegmentCacheEntry{reference: reference, segment: segment}
	}
	return segments, nil
}

func decodeSearchRecordSegment(child ChildArtifact, reference SearchRecordSegmentReferenceV1) (SearchRecordSegmentV1, error) {
	var segment SearchRecordSegmentV1
	if err := decodeCanonicalSearchChild(child.Bytes, &segment); err != nil {
		return SearchRecordSegmentV1{}, fmt.Errorf("decode search record segment %q: %w", reference.Path, err)
	}
	if segment.SchemaVersion != 1 || segment.SearchVersion != searchVersion || segment.FirstRecord != reference.FirstRecord || len(segment.Records) != int(reference.Records) {
		return SearchRecordSegmentV1{}, fmt.Errorf("search record segment %q is invalid", reference.Path)
	}
	return segment, nil
}

func reserveSearchReference(receipt *searchLoadReceipt, reference SearchSegmentReferenceV1, maxSegments int) error {
	if _, exists := receipt.loaded[reference.Path]; exists {
		return nil
	}
	if int(receipt.segments)+1 > maxSegments {
		return fmt.Errorf("%w: decoded segments", ErrQueryTooBroad)
	}
	if receipt.bytes+reference.Length > maxSearchDecodedBytes {
		return fmt.Errorf("%w: decoded bytes", ErrQueryTooBroad)
	}
	if receipt.postings+uint64(reference.Postings) > maxSearchPostings {
		return fmt.Errorf("%w: postings", ErrQueryTooBroad)
	}
	receipt.loaded[reference.Path] = struct{}{}
	receipt.segments++
	receipt.bytes += reference.Length
	receipt.postings += uint64(reference.Postings)
	return nil
}

func reserveSearchRecordReference(receipt *searchLoadReceipt, reference SearchRecordSegmentReferenceV1) error {
	if _, exists := receipt.loaded[reference.Path]; exists {
		return nil
	}
	if int(receipt.recordSegments)+1 > maxSearchRecordSegments {
		return fmt.Errorf("%w: decoded record segments", ErrQueryTooBroad)
	}
	if receipt.bytes+reference.Length > maxSearchDecodedBytes {
		return fmt.Errorf("%w: decoded record bytes", ErrQueryTooBroad)
	}
	receipt.loaded[reference.Path] = struct{}{}
	receipt.segments++
	receipt.recordSegments++
	receipt.bytes += reference.Length
	return nil
}

func (service *SearchService) searchChild(ctx context.Context, pathValue, kind string, length uint64, digest string) (ChildArtifact, error) {
	if service.children != nil {
		child, exists := service.children[pathValue]
		if !exists || child.Kind != kind {
			return ChildArtifact{}, fmt.Errorf("search child %q is missing", pathValue)
		}
		if err := verifySearchChild(child, length, digest); err != nil {
			return ChildArtifact{}, err
		}
		return child, nil
	}
	if service.child == nil {
		return ChildArtifact{}, fmt.Errorf("search child loader is unavailable")
	}
	return service.child(ctx, pathValue, kind, length, digest)
}

func postingRoutes(routes []SearchPostingRouteV1, key string, allowPrefix bool) []SearchPostingRouteV1 {
	start := sort.Search(len(routes), func(index int) bool { return routes[index].Key >= key })
	if start < len(routes) && routes[start].Key == key {
		return routes[start : start+1]
	}
	if !allowPrefix {
		return nil
	}
	end := start
	for end < len(routes) && strings.HasPrefix(routes[end].Key, key) {
		end++
	}
	return routes[start:end]
}

func exactPostingRoute(routes []SearchPostingRouteV1, key string) (SearchPostingRouteV1, bool) {
	matched := postingRoutes(routes, key, false)
	if len(matched) == 0 {
		return SearchPostingRouteV1{}, false
	}
	return matched[0], true
}

func unionRecordIDs(left, right []uint32) []uint32 {
	result := make([]uint32, 0, len(left)+len(right))
	i, j := 0, 0
	for i < len(left) || j < len(right) {
		if j == len(right) || (i < len(left) && left[i] < right[j]) {
			result = append(result, left[i])
			i++
		} else if i == len(left) || right[j] < left[i] {
			result = append(result, right[j])
			j++
		} else {
			result = append(result, left[i])
			i++
			j++
		}
	}
	return result
}

func intersectRecordIDs(left, right []uint32) []uint32 {
	result := make([]uint32, 0, min(len(left), len(right)))
	for i, j := 0, 0; i < len(left) && j < len(right); {
		if left[i] < right[j] {
			i++
		} else if right[j] < left[i] {
			j++
		} else {
			result = append(result, left[i])
			i++
			j++
		}
	}
	return result
}

func deduplicateRecordIDs(values []uint32) []uint32 {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func countExactMatches(entries []SearchExactEntryV1) uint32 {
	var count uint32
	for _, entry := range entries {
		count += uint32(len(entry.Matches))
	}
	return count
}

func countPostingRecords(entries []SearchPostingEntryV1) uint32 {
	var count uint32
	for _, entry := range entries {
		count += uint32(len(entry.Records))
	}
	return count
}

func validateSearchDirectory(value SearchDirectoryV1) error {
	for index, bucket := range value.ExactBuckets {
		if len(bucket.Prefix) == 0 || len(bucket.Prefix) > sha256.Size*2 || !isLowerHex(bucket.Prefix) || (index > 0 && value.ExactBuckets[index-1].Prefix >= bucket.Prefix) {
			return fmt.Errorf("search exact buckets are invalid")
		}
	}
	for routeSet, routes := range [][]SearchPostingRouteV1{value.TokenRoutes, value.TrigramRoutes} {
		segmentCount := len(value.PostingSegments)
		if routeSet == 1 {
			segmentCount = len(value.TrigramSegments)
		}
		for index, route := range routes {
			if route.Key == "" || int(route.Segment) >= segmentCount || (index > 0 && routes[index-1].Key >= route.Key) {
				return fmt.Errorf("search posting routes are invalid")
			}
		}
	}
	for index, reference := range value.RecordSegments {
		if reference.Records == 0 || (index > 0 && uint64(value.RecordSegments[index-1].FirstRecord)+uint64(value.RecordSegments[index-1].Records) != uint64(reference.FirstRecord)) {
			return fmt.Errorf("search record segments are invalid")
		}
	}
	var recordCount uint64
	for _, reference := range value.RecordSegments {
		recordCount += uint64(reference.Records)
	}
	if recordCount != uint64(len(value.Ranks)) {
		return fmt.Errorf("search rank records are invalid")
	}
	for _, rank := range value.Ranks {
		if rank.Title == "" {
			return fmt.Errorf("search rank record title is empty")
		}
		if rank.Kind != "" && rank.Kind != "operation" && rank.Kind != "schema" {
			return fmt.Errorf("search rank record kind is invalid")
		}
	}
	return nil
}

func isLowerHex(value string) bool {
	for index := 0; index < len(value); index++ {
		if !strings.ContainsRune("0123456789abcdef", rune(value[index])) {
			return false
		}
	}
	return true
}

func decodeCanonicalSearchChild(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(canonical, data) {
		return fmt.Errorf("search JSON is non-canonical")
	}
	return nil
}

func verifySearchChild(child ChildArtifact, length uint64, digest string) error {
	if child.Length != length || uint64(len(child.Bytes)) != length || child.SHA256 != digest {
		return fmt.Errorf("search child %q metadata differs", child.Path)
	}
	actual := sha256.Sum256(child.Bytes)
	if hex.EncodeToString(actual[:]) != digest {
		return fmt.Errorf("search child %q digest differs", child.Path)
	}
	return nil
}

func searchError(parent, bounded context.Context, err error) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	if errors.Is(bounded.Err(), context.DeadlineExceeded) {
		return ErrSearchDeadline
	}
	return err
}
