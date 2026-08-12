package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/araihu/manja/application/catalog"
	"github.com/araihu/manja/internal/web/templates"
)

type catalogChildReader interface {
	ReadChild(context.Context, catalog.RuntimeSnapshot, string) ([]byte, catalog.ChildIdentityV1, error)
}

type CatalogHandler struct {
	runtime      *catalog.Runtime
	children     catalogChildReader
	details      *catalog.ByteCache
	search       *catalog.ByteCache
	presentation map[string]CatalogPresentation
	organization OrganizationPresentation
	enhancement  CatalogEnhancementPolicy
}

type CatalogPresentation struct {
	Description         string
	Readme              string
	License             CatalogLicensePresentation
	CanonicalBase       string
	SocialImage         string
	SocialImageMIMEType string
	SocialImageAlt      string
}

type CatalogLicensePresentation struct {
	Name string
	URL  string
}

type OrganizationPresentation struct {
	Title   string
	Readme  string
	License OrganizationLicensePresentation
	Sources []OrganizationSourcePresentation
	SEO     CatalogPresentation
}

type OrganizationLicensePresentation struct {
	Name string
	URL  string
}

type OrganizationSourcePresentation struct {
	Name     string
	Kind     string
	Location string
	URL      string
}

// Keep full catalog documents bounded while accommodating deeply described
// public API operations such as GitHub's.
const maxCatalogPageBytes = 1 << 20

var errCatalogPageTooLarge = errors.New("catalog representation exceeds byte limit")

type catalogPageBuffer struct {
	bytes.Buffer
	exceeded bool
}

func (buffer *catalogPageBuffer) Write(data []byte) (int, error) {
	remaining := maxCatalogPageBytes - buffer.Len()
	if remaining <= 0 {
		buffer.exceeded = true
		return 0, errCatalogPageTooLarge
	}
	if len(data) <= remaining {
		return buffer.Buffer.Write(data)
	}
	written, _ := buffer.Buffer.Write(data[:remaining])
	buffer.exceeded = true
	return written, errCatalogPageTooLarge
}

func NewCatalogHandler(runtime *catalog.Runtime, children catalogChildReader) http.Handler {
	return NewCatalogHandlerWithPresentation(runtime, children, nil)
}

func NewCatalogHandlerWithPresentation(runtime *catalog.Runtime, children catalogChildReader, presentation map[string]CatalogPresentation) http.Handler {
	return NewCatalogHandlerWithOrganization(runtime, children, presentation, OrganizationPresentation{})
}

func NewCatalogHandlerWithOrganization(runtime *catalog.Runtime, children catalogChildReader, presentation map[string]CatalogPresentation, organization OrganizationPresentation) http.Handler {
	return NewCatalogHandlerWithOrganizationAndEnhancement(runtime, children, presentation, organization, CatalogEnhancementPolicy{})
}

func NewCatalogHandlerWithOrganizationAndEnhancement(runtime *catalog.Runtime, children catalogChildReader, presentation map[string]CatalogPresentation, organization OrganizationPresentation, enhancement CatalogEnhancementPolicy) http.Handler {
	copyPresentation := make(map[string]CatalogPresentation, len(presentation))
	for mount, value := range presentation {
		copyPresentation[mount] = value
	}
	copyPublications := make(map[string]CatalogPublicEligibility, len(enhancement.Publications))
	for mount, value := range enhancement.Publications {
		copyPublications[mount] = value
	}
	enhancement.Publications = copyPublications
	organization.Sources = append([]OrganizationSourcePresentation(nil), organization.Sources...)
	return &CatalogHandler{runtime: runtime, children: children, details: catalog.NewDetailCache(), search: catalog.NewSearchCache(), presentation: copyPresentation, organization: organization, enhancement: enhancement}
}

// CatalogFlightReservationBytes reports the maximum encoded-plus-decoded
// weight already reserved by loaders that may briefly outlive a canceled HTTP
// waiter. Once request admission is quiesced, this value cannot increase.
func (handler *CatalogHandler) CatalogFlightReservationBytes() uint64 {
	detail := handler.details.Stats().InFlightBytes
	search := handler.search.Stats().InFlightBytes
	if detail > ^uint64(0)-search {
		return ^uint64(0)
	}
	return detail + search
}

func (handler *CatalogHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.runtime == nil || handler.children == nil {
		http.Error(response, "catalog unavailable", http.StatusServiceUnavailable)
		return
	}
	if IsCatalogComponentPath(request.URL.Path) {
		handler.serveCatalogDocumentCombobox(response, request)
		return
	}
	if request.URL.Path == "/search.json" && handler.servesOrganizationRoot() {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			response.Header().Set("Allow", "GET, HEAD")
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if hasEncodedSeparator(request.URL.EscapedPath()) {
			http.Error(response, "invalid path", http.StatusBadRequest)
			return
		}
		handler.serveGlobalSearchJSON(response, request, request.URL.Query().Get("context_mount"), request.URL.Query().Get("context_document"))
		return
	}
	// The root is an organization landing page only when every publication is
	// mounted below it. A deliberate "/" catalog owns the root route and must
	// retain its own overview instead of looping through a synthetic card.
	if request.URL.Path == "/" && handler.servesOrganizationRoot() {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			response.Header().Set("Allow", "GET, HEAD")
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if hasEncodedSeparator(request.URL.EscapedPath()) {
			http.Error(response, "invalid path", http.StatusBadRequest)
			return
		}
		handler.serveOrganizationRoot(response, request)
		return
	}
	mount, exactMount := handler.matchMount(request.URL.Path)
	if mount == "" {
		http.NotFound(response, request)
		return
	}
	if exactMount && mount != "/" {
		http.Redirect(response, request, mount+"/", http.StatusPermanentRedirect)
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		response.Header().Set("Allow", "GET, HEAD")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if hasEncodedSeparator(request.URL.EscapedPath()) {
		http.Error(response, "invalid path", http.StatusBadRequest)
		return
	}
	relative := strings.TrimPrefix(request.URL.Path, mount)
	if mount == "/" {
		relative = strings.TrimPrefix(request.URL.Path, "/")
	} else {
		relative = strings.TrimPrefix(relative, "/")
	}
	requestedSnapshot := snapshotIDFromCatalogRoute(relative)
	var admission *catalog.Admission
	var err error
	if requestedSnapshot == "" {
		admission, err = handler.runtime.Admit(mount)
	} else {
		admission, err = handler.runtime.AdmitSnapshot(mount, requestedSnapshot)
	}
	if err != nil {
		http.NotFound(response, request)
		return
	}
	defer admission.Release()
	handler.serveAdmitted(response, request, admission.Snapshot, mount, relative)
}

func (handler *CatalogHandler) servesOrganizationRoot() bool {
	if handler == nil || handler.runtime == nil {
		return false
	}
	mounts := handler.runtime.MountNames()
	if len(mounts) == 0 {
		return false
	}
	for _, mount := range mounts {
		if mount == "/" {
			return false
		}
	}
	return true
}

func (handler *CatalogHandler) serveOrganizationRoot(response http.ResponseWriter, request *http.Request) {
	title := strings.TrimSpace(handler.organization.Title)
	if title == "" {
		title = "Manja"
	}
	data := templates.CatalogPageData{
		OrganizationRoot: true,
		Mount:            "/",
		SearchGlobal:     true,
		SearchHref:       "/",
		SearchJSONHref:   "/search.json",
		SearchMount:      "/",
		SearchScopeLabel: "All catalogs",
		Directory: catalog.CatalogArtifactV1{
			Title: title,
			Branding: catalog.BrandingV1{
				DisplayName: title,
				LogoSrc:     "/manja-assets/manja-mark.svg",
				LogoAlt:     title,
			},
		},
		OrganizationNav: handler.catalogOrganizationNav("", true),
		Organization: templates.CatalogOrganizationPageData{
			Title:  title,
			Readme: strings.TrimSpace(handler.organization.Readme),
			License: templates.CatalogOrganizationLicenseData{
				Name: strings.TrimSpace(handler.organization.License.Name),
				URL:  strings.TrimSpace(handler.organization.License.URL),
			},
			Sources: make([]templates.CatalogOrganizationSourceData, len(handler.organization.Sources)),
		},
	}
	for index, source := range handler.organization.Sources {
		data.Organization.Sources[index] = templates.CatalogOrganizationSourceData{
			Name: strings.TrimSpace(source.Name), Kind: strings.TrimSpace(source.Kind),
			Location: strings.TrimSpace(source.Location), URL: strings.TrimSpace(source.URL),
		}
	}
	mounts := handler.runtime.MountNames()
	sort.Strings(mounts)
	for _, mount := range mounts {
		admission, err := handler.runtime.Admit(mount)
		if err != nil {
			continue
		}
		for _, document := range admission.Snapshot.Directory.Documents {
			href, err := catalogURL(mount, "documents", document.Key)
			if err != nil {
				continue
			}
			data.Documents = append(data.Documents, templates.CatalogDocumentOption{
				Key: document.Key, Label: catalogDocumentLabel(document), Version: document.APIVersion,
				Operations: len(document.Operations), Schemas: len(document.Schemas), Href: href + "/",
				SearchText: strings.ToLower(document.Key + " " + document.APIVersion),
				AvatarSrc:  document.Branding.LogoSrc, AvatarAlt: document.Branding.LogoAlt,
			})
		}
		admission.Release()
	}
	if query := request.URL.Query().Get("q"); query != "" {
		data.Search = &templates.CatalogSearchData{Query: query}
		if err := handler.populateGlobalSearchData(request.Context(), &data, query, "", ""); err != nil {
			writeCatalogSearchError(response, err)
			return
		}
	}
	handler.renderCatalogPage(response, request, data)
}

func (handler *CatalogHandler) matchMount(requestPath string) (string, bool) {
	mounts := handler.runtime.MountNames()
	sort.Slice(mounts, func(left, right int) bool { return len(mounts[left]) > len(mounts[right]) })
	for _, mount := range mounts {
		if mount == "/" {
			return mount, requestPath == "/"
		}
		if requestPath == mount {
			return mount, true
		}
		if strings.HasPrefix(requestPath, mount+"/") {
			return mount, false
		}
	}
	return "", false
}

func (handler *CatalogHandler) serveAdmitted(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount, relative string) {
	switch {
	case relative == "":
		if document := request.URL.Query().Get("document"); document != "" {
			target, err := catalogURL(mount, "documents", document)
			if err != nil || !catalogDocumentExists(snapshot.Directory, document) {
				http.NotFound(response, request)
				return
			}
			http.Redirect(response, request, target+"/", http.StatusSeeOther)
			return
		}
		handler.serveOverview(response, request, snapshot, mount)
	case relative == "catalog.json":
		handler.redirectStable(response, request, snapshot, mount, "catalog.json")
	case relative == "llms.txt":
		writePageMarkdown(response, request, catalogLLMsText(snapshot.Directory, mount))
	case relative == "search":
		handler.serveSearch(response, request, snapshot, mount)
	case relative == "search.json":
		handler.serveSearchJSON(response, request, snapshot, mount)
	case strings.HasPrefix(relative, "documents/"):
		documentPath := strings.TrimPrefix(relative, "documents/")
		key := strings.TrimSuffix(documentPath, "/")
		if key == "" || strings.Contains(key, "/") {
			http.NotFound(response, request)
			return
		}
		handler.serveDocument(response, request, snapshot, mount, key)
	case strings.HasPrefix(relative, "openapi/"):
		handler.serveStableSource(response, request, snapshot, mount, strings.TrimPrefix(relative, "openapi/"))
	case strings.HasPrefix(relative, "snapshots/"):
		handler.serveSnapshotResource(response, request, snapshot, mount, relative)
	case strings.HasSuffix(relative, "/") && !strings.Contains(strings.TrimSuffix(relative, "/"), "/"):
		handler.serveDocument(response, request, snapshot, mount, strings.TrimSuffix(relative, "/"))
	case !strings.Contains(relative, "/"):
		if !catalogDocumentExists(snapshot.Directory, relative) {
			http.NotFound(response, request)
			return
		}
		target, err := catalogURL(mount, relative)
		if err != nil {
			http.NotFound(response, request)
			return
		}
		http.Redirect(response, request, target+"/", http.StatusPermanentRedirect)
	default:
		http.NotFound(response, request)
	}
}

func (handler *CatalogHandler) serveSearch(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount string) {
	data, err := handler.catalogPageData(request.Context(), snapshot, mount, "", "", "", "", "")
	if err != nil {
		http.Error(response, "catalog temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	query := request.URL.Query().Get("q")
	data.Search = &templates.CatalogSearchData{Query: query}
	if query != "" {
		if err := handler.populateGlobalSearchData(request.Context(), &data, query, mount, request.URL.Query().Get("context_document")); err != nil {
			writeCatalogSearchError(response, err)
			return
		}
	}
	handler.renderCatalogPage(response, request, data)
}

func (handler *CatalogHandler) populateGlobalSearchData(ctx context.Context, data *templates.CatalogPageData, query, contextMount, contextDocument string) error {
	result, err := handler.searchGlobal(ctx, query, contextMount, contextDocument)
	if err != nil {
		return err
	}
	if data.Search == nil {
		data.Search = &templates.CatalogSearchData{}
	}
	data.Search.Query = result.Query
	data.Search.PostingsScanned = result.PostingsScanned
	data.Search.SegmentsDecoded = result.SegmentsDecoded
	data.Search.BytesDecoded = result.BytesDecoded
	data.Search.Results = data.Search.Results[:0]
	for _, candidate := range result.Results {
		data.Search.Results = append(data.Search.Results, templates.CatalogSearchResultData{Record: candidate.record, Href: candidate.record.Href})
	}
	return nil
}

func (handler *CatalogHandler) serveOverview(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount string) {
	data, err := handler.catalogPageData(request.Context(), snapshot, mount, "", "", "", "", "")
	if err != nil {
		http.Error(response, "catalog temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := sortCatalogDocuments(data.Documents, request.URL.Query().Get("order_by"), request.URL.Query().Get("order_dir")); err != nil {
		http.Error(response, "invalid catalog document sort", http.StatusBadRequest)
		return
	}
	data.DocumentSortBy = request.URL.Query().Get("order_by")
	data.DocumentSortDir = request.URL.Query().Get("order_dir")
	if request.URL.Query().Get("table_id") == "catalog-documents-table" {
		handler.renderCatalogDocumentTable(response, request, data)
		return
	}
	handler.renderCatalogPage(response, request, data)
}

func sortCatalogDocuments(documents []templates.CatalogDocumentOption, orderBy, orderDir string) error {
	if orderBy == "" && orderDir == "" {
		return nil
	}
	if orderDir != "asc" && orderDir != "desc" {
		return fmt.Errorf("unsupported sort direction %q", orderDir)
	}
	if orderBy != "name" && orderBy != "version" && orderBy != "operations" && orderBy != "schemas" {
		return fmt.Errorf("unsupported sort column %q", orderBy)
	}
	descending := orderDir == "desc"
	sort.SliceStable(documents, func(left, right int) bool {
		comparison := 0
		switch orderBy {
		case "name":
			comparison = strings.Compare(strings.ToLower(documents[left].Label), strings.ToLower(documents[right].Label))
		case "version":
			comparison = strings.Compare(strings.ToLower(documents[left].Version), strings.ToLower(documents[right].Version))
		case "operations":
			comparison = documents[left].Operations - documents[right].Operations
		case "schemas":
			comparison = documents[left].Schemas - documents[right].Schemas
		}
		if comparison == 0 {
			comparison = strings.Compare(documents[left].Key, documents[right].Key)
		}
		if descending {
			return comparison > 0
		}
		return comparison < 0
	})
	return nil
}

func (handler *CatalogHandler) renderCatalogDocumentTable(response http.ResponseWriter, request *http.Request, data templates.CatalogPageData) {
	var body catalogPageBuffer
	err := templates.CatalogDocumentTableFragment(data).Render(request.Context(), &body)
	if body.exceeded || errors.Is(err, errCatalogPageTooLarge) {
		http.Error(response, "catalog representation exceeds byte limit", http.StatusInternalServerError)
		return
	}
	if err != nil {
		http.Error(response, "render catalog document table", http.StatusInternalServerError)
		return
	}
	writeCatalogRepresentation(response, request, body.Bytes(), "text/html; charset=utf-8")
}

func (handler *CatalogHandler) serveDocument(response http.ResponseWriter, request *http.Request, snapshot catalog.RuntimeSnapshot, mount, key string) {
	query := request.URL.Query()
	expandedGroups, groupsExplicit := query["group"]
	data, err := handler.catalogPageDataWithSidebarQuery(
		request.Context(), snapshot, mount, key, query.Get("selected"), query.Get("node"),
		catalogSidebarQuery{groups: expandedGroups, explicit: groupsExplicit, pages: query["page"]},
	)
	if err != nil {
		if errors.Is(err, errCatalogPageNotFound) {
			http.NotFound(response, request)
			return
		}
		http.Error(response, "catalog temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	data.PageMarkdownHref = selectedPageMarkdownHref(request.URL.Path, query.Get("selected"))
	if query.Get("format") == "markdown" {
		document, ok := catalogPageMarkdown(data)
		if !ok {
			http.NotFound(response, request)
			return
		}
		writePageMarkdown(response, request, document)
		return
	}
	handler.renderCatalogPage(response, request, data)
}

func (handler *CatalogHandler) renderCatalogPage(response http.ResponseWriter, request *http.Request, data templates.CatalogPageData) {
	data.CapabilityFooter = handler.catalogCapabilityFooter(data)
	data.Metadata = handler.catalogPageMetadata(request, data)
	var body catalogPageBuffer
	var err error
	switch catalogFragmentTarget(request) {
	case "catalog-main-content":
		err = templates.CatalogMainFragment(data).Render(request.Context(), &body)
	case "catalog-sidebar-groups":
		err = templates.CatalogSidebarGroupsFragment(data).Render(request.Context(), &body)
	case "schema-node-panel":
		err = templates.CatalogSchemaNodeFragment(data).Render(request.Context(), &body)
	default:
		err = templates.CatalogPage(data).Render(request.Context(), &body)
	}
	if body.exceeded || errors.Is(err, errCatalogPageTooLarge) {
		http.Error(response, "catalog representation exceeds byte limit", http.StatusInternalServerError)
		return
	}
	if err != nil {
		http.Error(response, "render catalog", http.StatusInternalServerError)
		return
	}
	writeCatalogRepresentation(response, request, body.Bytes(), "text/html; charset=utf-8")
}

func catalogFragmentTarget(request *http.Request) string {
	if !strings.EqualFold(strings.TrimSpace(request.Header.Get("HX-Request")), "true") ||
		strings.EqualFold(strings.TrimSpace(request.Header.Get("HX-Boosted")), "true") ||
		strings.EqualFold(strings.TrimSpace(request.Header.Get("HX-History-Restore-Request")), "true") {
		return ""
	}
	target := strings.TrimSpace(request.Header.Get("HX-Target"))
	switch target {
	case "catalog-main-content", "catalog-sidebar-groups", "schema-node-panel":
		return target
	default:
		return ""
	}
}

func (handler *CatalogHandler) catalogCapabilityFooter(data templates.CatalogPageData) templates.CapabilityFooterData {
	footer := templates.CapabilityFooterData{}
	if !data.OrganizationRoot {
		footer.LLMsHref, _ = catalogURL(data.Mount, "llms.txt")
	}
	for _, source := range handler.organization.Sources {
		if source.Kind != "git" || strings.TrimSpace(source.URL) == "" {
			continue
		}
		footer.Clone = &templates.CapabilityFooterLink{
			Label: "Clone", Href: strings.TrimSpace(source.URL),
			Title: "Open source and ref guidance for " + strings.TrimSpace(source.Location),
		}
		break
	}
	if data.DownloadHref != "" {
		label := "Catalog JSON"
		title := "Export the catalog directory as JSON"
		download := false
		if data.Document != nil {
			label = "OpenAPI JSON"
			title = "Download the OpenAPI document as JSON"
			download = true
		}
		footer.Exports = append(footer.Exports, templates.CapabilityFooterLink{
			Label: label, Href: data.DownloadHref, Title: title, Download: download,
		})
	}
	if data.PageMarkdownHref != "" && data.Selected != nil {
		footer.Exports = append(footer.Exports, templates.CapabilityFooterLink{
			Label: "Page Markdown", Href: data.PageMarkdownHref, Title: "View this page as Markdown",
		})
	}
	return footer
}

func writeCatalogRepresentation(response http.ResponseWriter, request *http.Request, body []byte, contentType string) {
	digest := sha256.Sum256(body)
	etag := `"sha256-` + hex.EncodeToString(digest[:]) + `"`
	response.Header().Set("Cache-Control", "private, no-cache")
	response.Header().Set("Content-Type", contentType)
	response.Header().Set("ETag", etag)
	response.Header().Set("Vary", "HX-Request, HX-Boosted, HX-Target, HX-History-Restore-Request, Accept-Encoding")
	response.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; object-src 'none'; base-uri 'none'")
	if request.Header.Get("If-None-Match") == etag {
		response.WriteHeader(http.StatusNotModified)
		return
	}
	response.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	response.WriteHeader(http.StatusOK)
	if request.Method != http.MethodHead {
		_, _ = response.Write(body)
	}
}

func catalogURL(mount string, segments ...string) (string, error) {
	if mount != "/" && (mount == "" || !strings.HasPrefix(mount, "/") || strings.HasSuffix(mount, "/") || strings.ContainsAny(mount, `\?#%`) || strings.Contains(mount, "//")) {
		return "", fmt.Errorf("invalid catalog mount %q", mount)
	}
	base := mount
	if base == "/" {
		base = ""
	}
	if len(segments) == 0 {
		return base + "/", nil
	}
	encoded := make([]string, len(segments))
	for index, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, `/\?#`) {
			return "", fmt.Errorf("invalid catalog URL segment %q", segment)
		}
		encoded[index] = url.PathEscape(segment)
	}
	return base + "/" + strings.Join(encoded, "/"), nil
}

func snapshotIDFromCatalogRoute(relative string) catalog.SnapshotID {
	parts := strings.Split(relative, "/")
	if len(parts) >= 2 && parts[0] == "snapshots" {
		return catalog.SnapshotID(parts[1])
	}
	return ""
}

func hasEncodedSeparator(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c")
}
