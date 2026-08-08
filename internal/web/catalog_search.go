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
}

type globalSearchResult struct {
	Query           string
	SearchVersion   uint32
	Results         []globalSearchCandidate
	PostingsScanned uint64
	SegmentsDecoded uint64
	BytesDecoded    uint64
}

func (handler *CatalogHandler) searchCatalog(ctx context.Context, snapshot catalog.RuntimeSnapshot, mount, query string) (catalog.SearchResult, error) {
	service, err := catalog.NewRuntimeSearchService(snapshot, handler.search, func(loadContext context.Context, childPath string) ([]byte, catalog.ChildIdentityV1, error) {
		return handler.children.ReadChild(loadContext, snapshot, childPath)
	})
	if err != nil {
		return catalog.SearchResult{}, err
	}
	result, err := service.Search(ctx, snapshot.ID, query)
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
	result := globalSearchResult{SearchVersion: 1, Results: make([]globalSearchCandidate, 0)}
	if contextMount != "" && !handler.runtime.HasMount(contextMount) {
		contextMount = ""
	}

	mounts := handler.runtime.MountNames()
	sort.Strings(mounts)
	for _, mount := range mounts {
		admission, err := handler.runtime.Admit(mount)
		if err != nil {
			continue
		}
		snapshot := admission.Snapshot
		local, searchErr := handler.searchCatalog(ctx, snapshot, mount, query)
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
				record: record, mount: mount, section: snapshot.Directory.Title, localRank: index,
			})
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
				record: record, mount: mount, section: snapshot.Directory.Title, localRank: len(local.Results) + index,
			})
		}
		admission.Release()
	}

	if result.Query == "" {
		result.Query = strings.ToLower(strings.TrimSpace(query))
	}
	rankGlobalSearchCandidates(result.Results, contextMount, contextDocument)
	if len(result.Results) > maxGlobalSearchResults {
		result.Results = result.Results[:maxGlobalSearchResults]
	}
	return result, nil
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
		DocumentKey: document.Key, Kind: "document", Title: document.Key,
		Description: document.Title, Href: href + "/", Occurrences: 1,
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
