package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

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

func (handler *CatalogHandler) serveSearchJSON(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount string) {
	payload := catalogSearchResponse{
		CatalogID: snapshot.Directory.CatalogID, SnapshotID: snapshot.ID,
		Version: snapshot.Search.SearchVersion, Query: request.URL.Query().Get("q"), Results: make([]catalogSearchResult, 0),
	}
	if payload.Query != "" {
		result, err := handler.searchCatalog(request.Context(), snapshot, mount, payload.Query)
		if err != nil {
			writeCatalogSearchError(response, err)
			return
		}
		payload.Query = result.Query
		payload.Version = result.SearchVersion
		for _, record := range result.Results {
			payload.Results = append(payload.Results, catalogSearchResult{
				DetailID: record.DetailID, DocumentKey: record.DocumentKey, Kind: record.Kind,
				Title: record.Title, Description: record.Description, Href: record.Href,
				OperationID: record.OperationID, Method: record.Method, Path: record.Path, SchemaName: record.SchemaName,
				Occurrences: record.Occurrences, Documents: append([]string(nil), record.Documents...),
			})
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
