package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/domain"
)

const maxCatalogSearchJSONBytes = 64 << 10

type catalogSearchResponse struct {
	CatalogID  string                `json:"catalogId"`
	SnapshotID catalog.SnapshotID    `json:"snapshotId"`
	Version    uint32                `json:"searchVersion"`
	Query      string                `json:"query"`
	Results    []catalogSearchResult `json:"results"`
}

type catalogSearchResult struct {
	DetailID    domain.DetailID `json:"detailId"`
	DocumentKey string          `json:"documentKey"`
	Kind        string          `json:"kind"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Href        string          `json:"href"`
	OperationID string          `json:"operationId,omitempty"`
	Method      string          `json:"method,omitempty"`
	Path        string          `json:"path,omitempty"`
	SchemaName  string          `json:"schemaName,omitempty"`
	Occurrences uint32          `json:"occurrences"`
	Documents   []string        `json:"documents"`
	Section     string          `json:"section,omitempty"`
}

const maxGlobalSearchResults = 20

type globalSearchCandidate struct {
	record    catalog.SearchRecordV1
	mount     string
	section   string
	localRank int
	exactID   bool
}

type globalSearchResult struct {
	Query           string
	SearchVersion   uint32
	Results         []globalSearchCandidate
	PostingsScanned uint64
	SegmentsDecoded uint64
	BytesDecoded    uint64
}

func (handler *CatalogHandler) searchCatalog(ctx context.Context, snapshot catalog.RuntimeSnapshot, mount string, query catalog.CanonicalSearchQuery) (catalog.SearchResult, error) {
	service, err := catalog.NewRuntimeSearchService(snapshot, handler.search, func(loadContext context.Context, childPath string) ([]byte, catalog.ChildIdentityV1, error) {
		return handler.children.ReadChild(loadContext, snapshot, childPath)
	})
	if err != nil {
		return catalog.SearchResult{}, err
	}
	result, err := service.SearchCanonical(ctx, snapshot.ID, query)
	if err != nil {
		return catalog.SearchResult{}, err
	}
	for index := range result.Results {
		href, err := catalogSearchHref(mount, result.Results[index].Href)
		if err != nil {
			return catalog.SearchResult{}, err
		}
		result.Results[index].Href = href
	}
	return result, nil
}

func (handler *CatalogHandler) searchGlobal(ctx context.Context, query, contextMount, contextDocument string) (globalSearchResult, error) {
	canonical, err := catalog.CanonicalizeSearchQuery(query)
	if err != nil {
		return globalSearchResult{}, err
	}
	if contextMount != "" && !handler.runtime.HasMount(contextMount) {
		contextMount = ""
	}
	exactDetailID := canonicalDetailIDQuery(canonical)
	// Published detail IDs are already bound by each admitted catalog
	// directory, so resolve them before deadline-bound search child loading.
	if exactDetailID {
		if exact, found, err := handler.searchGlobalExact(canonical, contextMount, contextDocument); err != nil {
			return globalSearchResult{}, err
		} else if found {
			return exact, nil
		}
	}

	result := globalSearchResult{SearchVersion: 1, Results: make([]globalSearchCandidate, 0)}

	mounts := handler.runtime.MountNames()
	sort.Strings(mounts)
	for _, mount := range mounts {
		admission, err := handler.runtime.Admit(mount)
		if err != nil {
			continue
		}
		snapshot := admission.Snapshot
		documentLabels := make(map[string]string, len(snapshot.Directory.Documents))
		for _, document := range snapshot.Directory.Documents {
			documentLabels[document.Key] = catalogDocumentLabel(document)
		}
		local, searchErr := handler.searchCatalog(ctx, snapshot, mount, canonical)
		if searchErr != nil {
			admission.Release()
			return globalSearchResult{}, searchErr
		}
		if result.Query == "" {
			result.Query = local.Query
		}
		result.PostingsScanned += local.PostingsScanned
		result.SegmentsDecoded += local.SegmentsDecoded
		result.BytesDecoded += local.BytesDecoded
		for index, record := range local.Results {
			result.Results = append(result.Results, globalSearchCandidate{
				record: record, mount: mount, section: catalogSearchSection(documentLabels, snapshot.Directory.Title, record.DocumentKey), localRank: index,
				exactID: globalSearchRecordMatchesDetailID(record, result.Query),
			})
		}
		if exactDetailID {
			exactRecord, exactFound, exactErr := globalSearchExactRecord(snapshot.Directory, mount, result.Query)
			if exactErr != nil {
				admission.Release()
				return globalSearchResult{}, exactErr
			}
			if exactFound && !globalSearchResultsContainDetail(local.Results, exactRecord.DetailID) {
				result.Results = append(result.Results, globalSearchCandidate{
					record: exactRecord, mount: mount, section: catalogSearchSection(documentLabels, snapshot.Directory.Title, exactRecord.DocumentKey), localRank: -1, exactID: true,
				})
			}
		}
		for index, document := range snapshot.Directory.Documents {
			if !globalDocumentMatches(result.Query, snapshot.Directory.Title, document) {
				continue
			}
			record, recordErr := globalDocumentSearchRecord(mount, document)
			if recordErr != nil {
				admission.Release()
				return globalSearchResult{}, recordErr
			}
			result.Results = append(result.Results, globalSearchCandidate{
				record: record, mount: mount, section: catalogSearchSection(documentLabels, snapshot.Directory.Title, record.DocumentKey), localRank: len(local.Results) + index,
				exactID: globalSearchRecordMatchesDetailID(record, result.Query),
			})
		}
		admission.Release()
	}

	if result.Query == "" {
		result.Query = canonical.String()
	}
	rankGlobalSearchCandidates(result.Results, contextMount, contextDocument)
	if len(result.Results) > maxGlobalSearchResults {
		result.Results = result.Results[:maxGlobalSearchResults]
	}
	return result, nil
}

func canonicalDetailIDQuery(query catalog.CanonicalSearchQuery) bool {
	const prefix = "detail-sha256-"
	value := query.String()
	if len(value) != len(prefix)+64 || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, character := range value[len(prefix):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func (handler *CatalogHandler) searchGlobalExact(query catalog.CanonicalSearchQuery, contextMount, contextDocument string) (globalSearchResult, bool, error) {
	result := globalSearchResult{
		Query:         query.String(),
		SearchVersion: 1,
		Results:       make([]globalSearchCandidate, 0),
	}
	mounts := handler.runtime.MountNames()
	sort.Strings(mounts)
	for _, mount := range mounts {
		admission, err := handler.runtime.Admit(mount)
		if err != nil {
			continue
		}
		record, found, exactErr := globalSearchExactRecord(admission.Snapshot.Directory, mount, result.Query)
		if exactErr != nil {
			admission.Release()
			return globalSearchResult{}, false, exactErr
		}
		if found {
			section := catalogDocumentLabelForKey(admission.Snapshot.Directory, record.DocumentKey)
			result.Results = append(result.Results, globalSearchCandidate{
				record: record, mount: mount, section: section, localRank: -1, exactID: true,
			})
		}
		admission.Release()
	}
	if len(result.Results) == 0 {
		return globalSearchResult{}, false, nil
	}
	rankGlobalSearchCandidates(result.Results, contextMount, contextDocument)
	if len(result.Results) > maxGlobalSearchResults {
		result.Results = result.Results[:maxGlobalSearchResults]
	}
	return result, true, nil
}

func catalogDocumentLabelForKey(directory catalog.CatalogArtifactV1, key string) string {
	for _, document := range directory.Documents {
		if document.Key == key {
			return catalogDocumentLabel(document)
		}
	}
	if key = strings.TrimSpace(key); key != "" {
		return key
	}
	if title := strings.TrimSpace(directory.Title); title != "" {
		return title
	}
	return "Untitled document"
}

func catalogSearchSection(labels map[string]string, catalogTitle, documentKey string) string {
	if label := strings.TrimSpace(labels[documentKey]); label != "" {
		return label
	}
	return catalogDocumentLabelForKey(catalog.CatalogArtifactV1{Title: catalogTitle}, documentKey)
}

func globalSearchRecordMatchesDetailID(record catalog.SearchRecordV1, query string) bool {
	return query == string(record.DetailID)
}

func globalSearchResultsContainDetail(results []catalog.SearchRecordV1, detailID domain.DetailID) bool {
	for _, record := range results {
		if record.DetailID == detailID {
			return true
		}
	}
	return false
}

func globalSearchExactRecord(directory catalog.CatalogArtifactV1, mount, query string) (catalog.SearchRecordV1, bool, error) {
	for _, document := range directory.Documents {
		for _, operation := range document.Operations {
			if query != string(operation.DetailID) {
				continue
			}
			href, err := catalogSearchHref(mount, operation.Href)
			if err != nil {
				return catalog.SearchRecordV1{}, false, err
			}
			return catalog.SearchRecordV1{
				DetailID: operation.DetailID, DocumentKey: document.Key, Kind: "operation",
				Title: operation.Title, Description: operation.Description, Href: href,
				OperationID: operation.OperationID, Method: operation.Method, Path: operation.Path,
				Occurrences: 1, Documents: []string{document.Key},
			}, true, nil
		}
		for _, schema := range document.Schemas {
			if query != string(schema.DetailID) {
				continue
			}
			href, err := catalogSearchHref(mount, schema.Href)
			if err != nil {
				return catalog.SearchRecordV1{}, false, err
			}
			return catalog.SearchRecordV1{
				DetailID: schema.DetailID, DocumentKey: document.Key, Kind: "schema",
				Title: schema.Name, Description: schema.Description, Href: href, SchemaName: schema.Name,
				Occurrences: 1, Documents: []string{document.Key},
			}, true, nil
		}
	}
	return catalog.SearchRecordV1{}, false, nil
}

func globalDocumentSearchRecord(mount string, document catalog.DocumentDirectoryV1) (catalog.SearchRecordV1, error) {
	href, err := catalogURL(mount, "documents", document.Key)
	if err != nil {
		return catalog.SearchRecordV1{}, err
	}
	slug := strings.NewReplacer("/", "-", "\\", "-").Replace(strings.Trim(mount, "/"))
	if slug == "" {
		slug = "root"
	}
	return catalog.SearchRecordV1{
		DetailID:    domain.DetailID("document-" + slug + "-" + document.Key),
		DocumentKey: document.Key, Kind: "document", Title: catalogDocumentLabel(document),
		Description: document.Overview.Description, Href: href + "/", Occurrences: 1,
		Documents: []string{document.Key},
	}, nil
}

func globalDocumentMatches(query, catalogTitle string, document catalog.DocumentDirectoryV1) bool {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return false
	}
	haystack := strings.ToLower(strings.Join([]string{catalogTitle, document.Key, document.Title, document.APIVersion}, " "))
	if strings.Contains(haystack, needle) {
		return true
	}
	for _, token := range strings.Fields(needle) {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

func globalSearchKindWeight(kind string) int64 {
	switch strings.ToLower(kind) {
	case "operation":
		return 3
	case "document":
		return 2
	case "schema":
		return 1
	default:
		return 0
	}
}

func globalSearchKindOrder(kind string) int {
	switch strings.ToLower(kind) {
	case "operation":
		return 0
	case "document":
		return 1
	case "schema":
		return 2
	default:
		return 3
	}
}

func globalSearchCandidateBelongsToDocument(candidate globalSearchCandidate, documentKey string) bool {
	if candidate.record.DocumentKey == documentKey {
		return true
	}
	for _, key := range candidate.record.Documents {
		if key == documentKey {
			return true
		}
	}
	return false
}

func globalSearchScore(candidate globalSearchCandidate, contextMount, contextDocument string) int64 {
	score := globalSearchKindWeight(candidate.record.Kind) * 1_000_000
	if candidate.exactID {
		score += 10_000_000
	}
	if contextMount != "" && candidate.mount == contextMount {
		score += 100_000
	}
	if contextMount != "" && contextDocument != "" && candidate.mount == contextMount && globalSearchCandidateBelongsToDocument(candidate, contextDocument) {
		score += 200_000
	}
	return score - int64(candidate.localRank)
}

func rankGlobalSearchCandidates(candidates []globalSearchCandidate, contextMount, contextDocument string) {
	sort.SliceStable(candidates, func(left, right int) bool {
		leftScore := globalSearchScore(candidates[left], contextMount, contextDocument)
		rightScore := globalSearchScore(candidates[right], contextMount, contextDocument)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		leftKind := globalSearchKindOrder(candidates[left].record.Kind)
		rightKind := globalSearchKindOrder(candidates[right].record.Kind)
		if leftKind != rightKind {
			return leftKind < rightKind
		}
		leftTitle := strings.ToLower(candidates[left].record.Title)
		rightTitle := strings.ToLower(candidates[right].record.Title)
		if leftTitle != rightTitle {
			return leftTitle < rightTitle
		}
		if candidates[left].mount != candidates[right].mount {
			return candidates[left].mount < candidates[right].mount
		}
		return candidates[left].record.DetailID < candidates[right].record.DetailID
	})
}

func catalogSearchResultFromCandidate(candidate globalSearchCandidate) catalogSearchResult {
	record := candidate.record
	return catalogSearchResult{
		DetailID: record.DetailID, DocumentKey: record.DocumentKey, Kind: record.Kind,
		Title: record.Title, Description: record.Description, Href: record.Href,
		OperationID: record.OperationID, Method: record.Method, Path: record.Path, SchemaName: record.SchemaName,
		Occurrences: record.Occurrences, Documents: append([]string(nil), record.Documents...), Section: candidate.section,
	}
}

func (handler *CatalogHandler) serveSearchJSON(response http.ResponseWriter, request *http.Request, _ catalog.RuntimeSnapshot, mount string) {
	handler.serveGlobalSearchJSON(response, request, mount, request.URL.Query().Get("context_document"))
}

func (handler *CatalogHandler) serveGlobalSearchJSON(response http.ResponseWriter, request *http.Request, contextMount, contextDocument string) {
	payload := catalogSearchResponse{
		CatalogID: "global", Version: 1, Query: request.URL.Query().Get("q"), Results: make([]catalogSearchResult, 0),
	}
	if payload.Query != "" {
		result, err := handler.searchGlobal(request.Context(), payload.Query, contextMount, contextDocument)
		if err != nil {
			writeCatalogSearchError(response, err)
			return
		}
		payload.Query = result.Query
		payload.Version = result.SearchVersion
		for _, candidate := range result.Results {
			payload.Results = append(payload.Results, catalogSearchResultFromCandidate(candidate))
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(response, "render catalog search", http.StatusInternalServerError)
		return
	}
	if len(body) > maxCatalogSearchJSONBytes {
		http.Error(response, "catalog search response exceeds byte limit", http.StatusInternalServerError)
		return
	}
	writeCatalogRepresentation(response, request, body, "application/json; charset=utf-8")
}

func writeCatalogSearchError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, catalog.ErrInvalidQuery):
		http.Error(response, "invalid search query", http.StatusBadRequest)
	case errors.Is(err, catalog.ErrQueryTooBroad):
		http.Error(response, "search query is too broad", http.StatusUnprocessableEntity)
	case errors.Is(err, catalog.ErrSearchDeadline):
		response.Header().Set("Retry-After", "1")
		http.Error(response, "search temporarily unavailable", http.StatusServiceUnavailable)
	default:
		http.Error(response, "search temporarily unavailable", http.StatusServiceUnavailable)
	}
}
