//go:build !manja_runtime

package selfhosted

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/araihu/manja/application/catalog"
	artifact "github.com/araihu/manja/application/htmlartifact"
	"github.com/araihu/manja/internal/adapters/catalogjson"
	markdownadapter "github.com/araihu/manja/internal/adapters/markdown"
	"github.com/araihu/manja/internal/adapters/schemacache"
	"github.com/araihu/manja/internal/localdocs"
	localrender "github.com/araihu/manja/internal/localdocs/render"
	"github.com/araihu/manja/internal/web"
	"github.com/araihu/manja/renderer"
)

const (
	exportIdentityPath = "_manja/identity.json"
	exportManifestPath = "_manja/export.json"
)

type ExportOptions struct {
	RendererOptions
	Output           string
	BasePath         string
	SidebarChunkSize uint32
	FragmentWorkers  uint32
	// SchemaCacheBytes defaults to 128 MiB when nil; a pointer to zero disables retention.
	SchemaCacheBytes *uint64
	// Progress receives periodic progress events. A nil callback keeps export
	// output and execution on the default quiet path.
	Progress ExportProgress
}

type ExportReceipt struct {
	SchemaVersion uint32                 `json:"schemaVersion"`
	BasePath      string                 `json:"basePath"`
	Catalogs      []ExportCatalogReceipt `json:"catalogs"`
	Manifest      string                 `json:"manifest"`
}

type ExportCatalogReceipt struct {
	CatalogID      string `json:"catalogId"`
	Mount          string `json:"mount"`
	PublicationKey string `json:"publicationKey"`
	RevisionID     string `json:"revisionId"`
	SnapshotID     string `json:"snapshotId"`
}

func ExportRenderer(ctx context.Context, options ExportOptions) (receipt ExportReceipt, err error) {
	progress := newExportProgress(ctx, options.Progress)
	defer func() { progress.finish(err) }()
	ctx = withExportProgress(ctx, progress)
	ctx = localrender.WithDescriptionRenderer(ctx, markdownadapter.DescriptionComponent)
	if err := canonicalExportBasePath(options.BasePath); err != nil {
		return ExportReceipt{}, err
	}
	if strings.TrimSpace(options.Output) == "" {
		return ExportReceipt{}, errors.New("output directory is required")
	}
	handler, receipts, err := NewRenderer(ctx, options.RendererOptions)
	if err != nil {
		return ExportReceipt{}, err
	}
	progress.setCatalogTotals(len(receipts))
	progress.phase("materialize")
	for _, receipt := range receipts {
		if receipt.Degraded {
			return ExportReceipt{}, fmt.Errorf("export catalog %q: %s", receipt.CatalogID, receipt.Diagnostic)
		}
		if receipt.SnapshotID == "" {
			return ExportReceipt{}, fmt.Errorf("export catalog %q produced no active snapshot", receipt.CatalogID)
		}
	}
	workers := options.FragmentWorkers
	if workers == 0 {
		workers = 4
	}
	if workers > 32 {
		return ExportReceipt{}, errors.New("fragment workers must be between 1 and 32")
	}
	budget := uint64(128 << 20)
	if options.SchemaCacheBytes != nil {
		budget = *options.SchemaCacheBytes
	}
	return exportFromHandlerWithCache(ctx, handler, receipts, options.Output, options.BasePath, artifact.BuildProfile{SidebarChunkSize: options.SidebarChunkSize}, workers, schemacache.New(budget))
}

func exportFromHandler(ctx context.Context, handler http.Handler, receipts []renderer.ActivationReceipt, output, basePath string) (receipt ExportReceipt, err error) {
	return exportFromHandlerWithProfile(ctx, handler, receipts, output, basePath, artifact.BuildProfile{}, 4)
}

func exportFromHandlerWithProfile(ctx context.Context, handler http.Handler, receipts []renderer.ActivationReceipt, output, basePath string, profile artifact.BuildProfile, fragmentWorkers uint32) (receipt ExportReceipt, err error) {
	return exportFromHandlerWithCache(ctx, handler, receipts, output, basePath, profile, fragmentWorkers, schemacache.New(128<<20))
}

func exportFromHandlerWithCache(ctx context.Context, handler http.Handler, receipts []renderer.ActivationReceipt, output, basePath string, profile artifact.BuildProfile, fragmentWorkers uint32, schemaCache *schemacache.Cache) (receipt ExportReceipt, err error) {
	progress := exportProgressFromContext(ctx)
	progress.phase("materialize")
	output, err = filepath.Abs(output)
	if err != nil {
		return ExportReceipt{}, fmt.Errorf("resolve output: %w", err)
	}
	reuseOutput, err := prepareExportOutput(ctx, output)
	if err != nil {
		return ExportReceipt{}, err
	}
	cacheRoot := ""
	if reuseOutput {
		cacheRoot = output
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return ExportReceipt{}, fmt.Errorf("create output parent: %w", err)
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), "."+filepath.Base(output)+"-export-*")
	if err != nil {
		return ExportReceipt{}, fmt.Errorf("create export staging directory: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(stage)
		}
	}()

	writer := exportTreeWriter{root: stage, entries: make(map[string]exportFileEntry), progress: progress}
	if err = captureShell(ctx, handler, &writer, "/", "index.html"); err != nil {
		return ExportReceipt{}, err
	}
	for _, assetPath := range staticExportAssetPaths() {
		if err = captureResource(ctx, handler, &writer, assetPath, strings.TrimPrefix(assetPath, "/"), 0, ""); err != nil {
			return ExportReceipt{}, err
		}
	}
	worker, err := captureHTTP(ctx, handler, "/manja-assets/local-docs/sw.js", 0, "")
	if err != nil {
		return ExportReceipt{}, err
	}
	if err = writer.write("sw.js", worker.body, worker.mediaType); err != nil {
		return ExportReceipt{}, err
	}

	catalogReceipts := make([]ExportCatalogReceipt, 0, len(receipts))
	searchCatalogs := make([]deploymentSearchCatalogV1, 0, len(receipts))
	rootCatalog := false
	for _, active := range receipts {
		catalogReceipt, searchCatalog, captureErr := captureCatalog(ctx, handler, &writer, active, basePath, cacheRoot, profile.Resolved(), fragmentWorkers, schemaCache)
		if captureErr != nil {
			return ExportReceipt{}, captureErr
		}
		rootCatalog = rootCatalog || active.Mount == "/"
		catalogReceipts = append(catalogReceipts, catalogReceipt)
		searchCatalogs = append(searchCatalogs, searchCatalog)
	}
	if !writer.has("search/index.html") {
		if err = writer.copy("index.html", "search/index.html"); err != nil {
			return ExportReceipt{}, err
		}
	}
	if !rootCatalog {
		if err = writer.rewriteHTML("index.html", basePath, nil); err != nil {
			return ExportReceipt{}, err
		}
		if err = writer.rewriteHTML("search/index.html", basePath, nil); err != nil {
			return ExportReceipt{}, err
		}
	}
	progress.phase("hash/manifest")
	searchDirectory, err := encodeDeploymentSearchDirectory(searchCatalogs)
	if err != nil {
		return ExportReceipt{}, err
	}
	if err = writer.write(deploymentSearchDirectoryPath, searchDirectory, "application/json"); err != nil {
		return ExportReceipt{}, err
	}
	searchEntry := writer.entries[deploymentSearchDirectoryPath]
	for _, entry := range writer.sortedEntries() {
		if entry.MediaType != "text/html" {
			continue
		}
		if err = writer.installDeploymentSearch(entry.Path, deploymentSearchDirectoryURL(basePath), searchEntry); err != nil {
			return ExportReceipt{}, fmt.Errorf("install deployment search in %q: %w", entry.Path, err)
		}
	}
	sort.Slice(catalogReceipts, func(i, j int) bool { return catalogReceipts[i].CatalogID < catalogReceipts[j].CatalogID })
	if err = writer.bindWorkerToShells(); err != nil {
		return ExportReceipt{}, err
	}
	identityBytes, err := encodeExportIdentity(exportIdentity{SchemaVersion: 1, BasePath: basePath, Catalogs: catalogReceipts})
	if err != nil {
		return ExportReceipt{}, err
	}
	if err = writer.write(exportIdentityPath, identityBytes, "application/json"); err != nil {
		return ExportReceipt{}, err
	}
	manifest := exportManifest{SchemaVersion: 1, Rendering: "html", BasePath: basePath, Catalogs: catalogReceipts, Files: writer.sortedEntries()}
	manifestBytes, err := encodeExportManifest(manifest)
	if err != nil {
		return ExportReceipt{}, err
	}
	if err = writer.write(exportManifestPath, manifestBytes, "application/json"); err != nil {
		return ExportReceipt{}, err
	}
	progress.phase("verify")
	entries := writer.sortedEntries()
	var totalBytes uint64
	for _, entry := range entries {
		totalBytes += entry.Length
	}
	progress.setFinalTotals(uint64(len(entries)), totalBytes)
	if _, err = VerifyExport(ctx, stage); err != nil {
		return ExportReceipt{}, fmt.Errorf("verify staging export: %w", err)
	}
	if reuseOutput {
		if err = replaceExportOutput(output, stage); err != nil {
			return ExportReceipt{}, err
		}
	} else {
		if err = removeEmptyExportOutput(output); err != nil {
			return ExportReceipt{}, err
		}
		if err = os.Rename(stage, output); err != nil {
			return ExportReceipt{}, fmt.Errorf("publish export: %w", err)
		}
	}
	return exportReceipt(manifest), nil
}

type capturedHTTP struct {
	body      []byte
	mediaType string
}

func captureCatalog(ctx context.Context, handler http.Handler, writer *exportTreeWriter, active renderer.ActivationReceipt, basePath, cacheRoot string, profile artifact.BuildProfile, fragmentWorkers uint32, schemaCache *schemacache.Cache) (ExportCatalogReceipt, deploymentSearchCatalogV1, error) {
	progress := exportProgressFromContext(ctx)
	progress.catalogStarted(active.CatalogID)
	progress.phase("materialize")
	mountPrefix := strings.Trim(active.Mount, "/")
	shellPath := path.Join(mountPrefix, "index.html")
	if shellPath == "." {
		shellPath = "index.html"
	}
	overviewRoute := catalogRoute(active.Mount)
	if overviewRoute != "/" {
		overviewRoute += "/"
	}
	if err := captureShell(ctx, handler, writer, overviewRoute, shellPath); err != nil {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, err
	}
	if err := captureShell(ctx, handler, writer, catalogRoute(active.Mount, "search"), path.Join(mountPrefix, "search/index.html")); err != nil {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, err
	}
	if err := captureResource(ctx, handler, writer, catalogRoute(active.Mount, "llms.txt"), path.Join(mountPrefix, "llms.txt"), 0, ""); err != nil {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, err
	}

	snapshotBase := catalogRoute(active.Mount, "snapshots", active.SnapshotID)
	manifestCapture, err := captureHTTP(ctx, handler, snapshotBase+"/manifest.json", 0, "")
	if err != nil {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, err
	}
	manifest, err := catalogjson.DecodeManifest(manifestCapture.body)
	if err != nil {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, fmt.Errorf("catalog %q manifest: %w", active.CatalogID, err)
	}
	if manifest.Identity.CatalogID != active.CatalogID || string(manifest.SnapshotID) != active.SnapshotID || manifest.Identity.RevisionID != active.RevisionID {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, fmt.Errorf("catalog %q activation differs from manifest", active.CatalogID)
	}
	manifestOutput := path.Join(mountPrefix, "snapshots", active.SnapshotID, "manifest.json")
	if err := writer.write(manifestOutput, manifestCapture.body, manifestCapture.mediaType); err != nil {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, err
	}

	catalogCapture, err := captureHTTP(ctx, handler, snapshotBase+"/catalog.json", 0, "")
	if err != nil {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, err
	}
	directory, err := catalogjson.DecodeCatalogWithResourceLimits(catalogCapture.body, false)
	if err != nil || directory.CatalogID != active.CatalogID {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, fmt.Errorf("catalog %q directory is invalid", active.CatalogID)
	}
	var operationTotal, schemaTotal int
	for _, document := range directory.Documents {
		operationTotal += len(document.Operations)
		schemaTotal += len(document.Schemas)
	}
	progress.addCatalogTotals(len(directory.Documents), operationTotal, schemaTotal)
	if child, ok := manifestChild(manifest, "catalog.json"); !ok || !matchesChild(child, catalogCapture.body) {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, fmt.Errorf("catalog %q directory differs from manifest", active.CatalogID)
	}
	if err := writer.write(path.Join(mountPrefix, "snapshots", active.SnapshotID, "catalog.json"), catalogCapture.body, catalogCapture.mediaType); err != nil {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, err
	}

	for _, document := range directory.Documents {
		if err := captureShell(ctx, handler, writer, catalogRoute(active.Mount, "documents", document.Key)+"/", path.Join(mountPrefix, "documents", document.Key, "index.html")); err != nil {
			return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, err
		}
	}
	if err := writer.copy(shellPath, path.Join(mountPrefix, "_manja/offline-shell/index.html")); err != nil {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, err
	}
	for _, child := range manifest.Children {
		if child.Path == "catalog.json" || child.Kind == "detail" || child.Kind == "schema-node" {
			continue
		}
		requestPath, outputPath, ok := exportedChildPath(active, directory, child)
		if !ok {
			return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, fmt.Errorf("catalog %q child %q cannot be exported", active.CatalogID, child.Path)
		}
		if err := captureResource(ctx, handler, writer, requestPath, outputPath, child.Length, child.SHA256); err != nil {
			return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, err
		}
	}
	publicationBase := prefixExportBase(basePath, catalogRoute(active.Mount))
	if !strings.HasSuffix(publicationBase, "/") {
		publicationBase += "/"
	}
	descriptor, ok := localdocs.PrepareStaticDescriptor(active.CatalogID, catalog.RuntimeSnapshot{ID: manifest.SnapshotID, Directory: directory, Manifest: manifest}, publicationBase, basePath)
	if !ok {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, fmt.Errorf("catalog %q static descriptor is invalid", active.CatalogID)
	}

	inputs, err := os.MkdirTemp(filepath.Dir(writer.root), ".manja-export-inputs-")
	if err != nil {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, err
	}
	defer os.RemoveAll(inputs)
	loader := newExportProjectionLoader(ctx, handler, active, manifest, directory, inputs)
	progress.phase("render")
	if err := emitCatalogHTMLFragments(ctx, writer, active, descriptor, manifest, manifestCapture.body, catalogCapture.body, directory, cacheRoot, profile, fragmentWorkers, schemaCache, loader); err != nil {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, fmt.Errorf("catalog %q HTML fragments: %w", active.CatalogID, err)
	}
	progress.phase("materialize")
	descriptor.Static.HTMLOnly = true
	htmlContext := &exportHTMLCatalog{Mount: active.Mount, SnapshotID: active.SnapshotID, Directory: directory, Descriptor: descriptor}
	htmlPaths := []string{shellPath, path.Join(mountPrefix, "search/index.html"), path.Join(mountPrefix, "_manja/offline-shell/index.html")}
	for _, document := range directory.Documents {
		htmlPaths = append(htmlPaths, path.Join(mountPrefix, "documents", document.Key, "index.html"))
	}
	for _, htmlPath := range htmlPaths {
		if err := writer.rewriteHTML(htmlPath, basePath, htmlContext); err != nil {
			return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, fmt.Errorf("rewrite catalog %q shell %q: %w", active.CatalogID, htmlPath, err)
		}
	}
	searchCatalog, err := deploymentSearchCatalog(active, basePath, directory, manifest)
	if err != nil {
		return ExportCatalogReceipt{}, deploymentSearchCatalogV1{}, err
	}
	progress.catalogCompleted()
	return ExportCatalogReceipt{CatalogID: active.CatalogID, Mount: active.Mount, PublicationKey: active.CatalogID, RevisionID: active.RevisionID, SnapshotID: active.SnapshotID}, searchCatalog, nil
}

func exportedChildPath(active renderer.ActivationReceipt, directory catalog.CatalogArtifactV1, child catalog.ChildIdentityV1) (string, string, bool) {
	prefix := strings.Trim(active.Mount, "/")
	snapshot := []string{prefix, "snapshots", active.SnapshotID}
	for _, document := range directory.Documents {
		if document.SourceChild == child.Path {
			name := document.Key + path.Ext(document.SourceChild)
			return catalogRoute(active.Mount, "snapshots", active.SnapshotID, "openapi", name), path.Join(append(snapshot, "openapi", name)...), true
		}
	}
	switch {
	case strings.HasPrefix(child.Path, "search/") && strings.HasPrefix(child.Kind, "search-"):
		return catalogRoute(active.Mount, "snapshots", active.SnapshotID, "search-data", child.Path), path.Join(append(snapshot, "search-data", child.Path)...), true
	case strings.HasPrefix(child.Path, "details/") && child.Kind == "detail", strings.HasPrefix(child.Path, "schema-nodes/") && child.Kind == "schema-node":
		return catalogRoute(active.Mount, "snapshots", active.SnapshotID, "projection-data", child.Path), path.Join(append(snapshot, "projection-data", child.Path)...), true
	default:
		return "", "", false
	}
}

func captureShell(ctx context.Context, handler http.Handler, writer *exportTreeWriter, requestPath, outputPath string) error {
	if writer.has(outputPath) {
		return nil
	}
	return captureResource(ctx, handler, writer, requestPath, outputPath, 0, "")
}

func captureResource(ctx context.Context, handler http.Handler, writer *exportTreeWriter, requestPath, outputPath string, length uint64, digest string) error {
	captured, err := captureHTTP(ctx, handler, requestPath, length, digest)
	if err != nil {
		return err
	}
	return writer.write(outputPath, captured.body, captured.mediaType)
}

func captureHTTP(ctx context.Context, handler http.Handler, requestPath string, length uint64, digest string) (capturedHTTP, error) {
	request := httptest.NewRequest(http.MethodGet, requestPath, nil).WithContext(ctx)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		if diagnostic := captureHTTPErrorDiagnostic(response); diagnostic != "" {
			return capturedHTTP{}, fmt.Errorf("capture %q: status %d: %s", requestPath, response.Code, diagnostic)
		}
		return capturedHTTP{}, fmt.Errorf("capture %q: status %d", requestPath, response.Code)
	}
	if response.Header().Get("Location") != "" {
		return capturedHTTP{}, fmt.Errorf("capture %q: redirects are not exportable", requestPath)
	}
	body, err := io.ReadAll(io.LimitReader(response.Result().Body, (64<<20)+1))
	if err != nil {
		return capturedHTTP{}, fmt.Errorf("capture %q: %w", requestPath, err)
	}
	if len(body) > 64<<20 {
		return capturedHTTP{}, fmt.Errorf("capture %q exceeds byte limit", requestPath)
	}
	if length != 0 && uint64(len(body)) != length {
		return capturedHTTP{}, fmt.Errorf("capture %q length differs", requestPath)
	}
	if digest != "" {
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != digest {
			return capturedHTTP{}, fmt.Errorf("capture %q digest differs", requestPath)
		}
	}
	mediaType := strings.TrimSpace(strings.Split(response.Header().Get("Content-Type"), ";")[0])
	if mediaType == "" {
		mediaType = mediaTypeForPath(requestPath)
	}
	return capturedHTTP{body: body, mediaType: mediaType}, nil
}

func captureHTTPErrorDiagnostic(response *httptest.ResponseRecorder) string {
	const (
		maxDiagnosticBytes = 4 << 10
		maxDiagnosticRunes = 512
	)
	body := response.Body.Bytes()
	if len(body) > maxDiagnosticBytes {
		body = body[:maxDiagnosticBytes]
	}
	diagnostic := strings.TrimSpace(strings.ToValidUTF8(string(body), "�"))
	diagnostic = strings.Join(strings.Fields(diagnostic), " ")
	if runes := []rune(diagnostic); len(runes) > maxDiagnosticRunes {
		diagnostic = string(runes[:maxDiagnosticRunes]) + "…"
	}
	return diagnostic
}

func catalogRoute(mount string, segments ...string) string {
	parts := make([]string, 0, len(segments)+1)
	if mount != "/" {
		parts = append(parts, strings.Trim(mount, "/"))
	}
	parts = append(parts, segments...)
	if len(parts) == 0 {
		return "/"
	}
	return "/" + path.Join(parts...)
}

func manifestChild(manifest catalog.ManifestV1, name string) (catalog.ChildIdentityV1, bool) {
	for _, child := range manifest.Children {
		if child.Path == name {
			return child, true
		}
	}
	return catalog.ChildIdentityV1{}, false
}

func matchesChild(child catalog.ChildIdentityV1, data []byte) bool {
	if uint64(len(data)) != child.Length {
		return false
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]) == child.SHA256
}

func prepareExportOutput(ctx context.Context, output string) (bool, error) {
	info, err := os.Lstat(output)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect output: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("output must be absent, an empty directory, or a verified Manja export")
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		return false, fmt.Errorf("inspect output: %w", err)
	}
	if len(entries) == 0 {
		return false, nil
	}
	if _, err := VerifyExport(ctx, output); err != nil {
		return false, fmt.Errorf("output directory is not an intact Manja export: %w", err)
	}
	return true, nil
}

func replaceExportOutput(output, stage string) error {
	backup, err := os.MkdirTemp(filepath.Dir(output), "."+filepath.Base(output)+"-previous-*")
	if err != nil {
		return fmt.Errorf("create export replacement marker: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("prepare export replacement marker: %w", err)
	}
	if err := os.Rename(output, backup); err != nil {
		return fmt.Errorf("retain previous export: %w", err)
	}
	if err := os.Rename(stage, output); err != nil {
		_ = os.Rename(backup, output)
		return fmt.Errorf("publish replacement export: %w", err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove previous export after publication: %w", err)
	}
	return nil
}

func removeEmptyExportOutput(output string) error {
	if _, err := os.Lstat(output); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect empty output directory: %w", err)
	}
	if err := os.Remove(output); err != nil {
		return fmt.Errorf("remove empty output directory: %w", err)
	}
	return nil
}

type exportTreeWriter struct {
	root     string
	entries  map[string]exportFileEntry
	progress *exportProgress
	mu       sync.Mutex
}

func (writer *exportTreeWriter) has(name string) bool {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	_, ok := writer.entries[name]
	return ok
}

func (writer *exportTreeWriter) copy(source, target string) error {
	data, err := os.ReadFile(filepath.Join(writer.root, filepath.FromSlash(source)))
	if err != nil {
		return err
	}
	return writer.write(target, data, writer.entries[source].MediaType)
}

func (writer *exportTreeWriter) rewriteHTML(name, basePath string, catalogContext *exportHTMLCatalog) error {
	filename := filepath.Join(writer.root, filepath.FromSlash(name))
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	rewritten, err := rewriteExportHTML(data, basePath, catalogContext)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filename, rewritten, 0o644); err != nil {
		return err
	}
	digest := sha256.Sum256(rewritten)
	writer.entries[name] = exportFileEntry{Path: name, Length: uint64(len(rewritten)), MediaType: "text/html", SHA256: hex.EncodeToString(digest[:])}
	return nil
}

func (writer *exportTreeWriter) installDeploymentSearch(name, directoryURL string, entry exportFileEntry) error {
	filename := filepath.Join(writer.root, filepath.FromSlash(name))
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	rewritten, err := installDeploymentSearchHTML(data, directoryURL, entry)
	if err != nil {
		return err
	}
	if bytes.Equal(data, rewritten) {
		return nil
	}
	if err := os.WriteFile(filename, rewritten, 0o644); err != nil {
		return err
	}
	digest := sha256.Sum256(rewritten)
	writer.entries[name] = exportFileEntry{Path: name, Length: uint64(len(rewritten)), MediaType: "text/html", SHA256: hex.EncodeToString(digest[:])}
	return nil
}

func (writer *exportTreeWriter) bindWorkerToShells() error {
	shells := sha256.New()
	for _, entry := range writer.sortedEntries() {
		if entry.MediaType == "text/html" {
			_, _ = fmt.Fprintf(shells, "%s\x00%d\x00%s\n", entry.Path, entry.Length, entry.SHA256)
		}
	}
	filename := filepath.Join(writer.root, "sw.js")
	worker, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	generation := hex.EncodeToString(shells.Sum(nil))
	worker = append(worker, []byte("\n// manja-static-shells-sha256="+generation+"\n")...)
	if err := os.WriteFile(filename, worker, 0o644); err != nil {
		return err
	}
	digest := sha256.Sum256(worker)
	writer.entries["sw.js"] = exportFileEntry{Path: "sw.js", Length: uint64(len(worker)), MediaType: "text/javascript", SHA256: hex.EncodeToString(digest[:])}
	return nil
}

func (writer *exportTreeWriter) write(name string, data []byte, mediaType string) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	name = path.Clean(name)
	if name == "." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return fmt.Errorf("invalid export path %q", name)
	}
	if _, exists := writer.entries[name]; exists {
		return fmt.Errorf("duplicate export path %q", name)
	}
	filename := filepath.Join(writer.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := io.Copy(file, bytes.NewReader(data))
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	digest := sha256.Sum256(data)
	writer.entries[name] = exportFileEntry{Path: name, Length: uint64(len(data)), MediaType: mediaType, SHA256: hex.EncodeToString(digest[:])}
	writer.progress.materialized(uint64(len(data)))
	return nil
}

func (writer *exportTreeWriter) registerExisting(name, mediaType string) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	name = path.Clean(name)
	if name == "." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return fmt.Errorf("invalid export path %q", name)
	}
	if _, exists := writer.entries[name]; exists {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(writer.root, filepath.FromSlash(name)))
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	writer.entries[name] = exportFileEntry{Path: name, Length: uint64(len(data)), MediaType: mediaType, SHA256: hex.EncodeToString(digest[:])}
	writer.progress.materialized(uint64(len(data)))
	return nil
}

func (writer *exportTreeWriter) linkExisting(sourceRoot, name, mediaType string, length uint64, digest string) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	name = path.Clean(name)
	if name == "." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) {
		return fmt.Errorf("invalid export path %q", name)
	}
	if _, exists := writer.entries[name]; exists {
		return nil
	}
	target := filepath.Join(writer.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	source := filepath.Join(sourceRoot, filepath.FromSlash(name))
	if err := os.Link(source, target); err != nil {
		data, readErr := os.ReadFile(source)
		if readErr != nil {
			return readErr
		}
		if uint64(len(data)) != length {
			return fmt.Errorf("cached export path %q length differs", name)
		}
		file, openErr := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if openErr != nil {
			return openErr
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	writer.entries[name] = exportFileEntry{Path: name, Length: length, MediaType: mediaType, SHA256: digest}
	writer.progress.materialized(length)
	return nil
}

func (writer *exportTreeWriter) sortedEntries() []exportFileEntry {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	entries := make([]exportFileEntry, 0, len(writer.entries))
	for _, entry := range writer.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}

func mediaTypeForPath(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".html":
		return "text/html"
	case ".json":
		return "application/json"
	case ".js":
		return "text/javascript"
	case ".wasm":
		return "application/wasm"
	case ".txt":
		return "text/plain"
	}
	if value := mime.TypeByExtension(path.Ext(name)); value != "" {
		return strings.Split(value, ";")[0]
	}
	return "application/octet-stream"
}

func canonicalExportBasePath(value string) error {
	if value == "/" {
		return nil
	}
	if len(value) < 3 || len(value) > 1024 || !strings.HasPrefix(value, "/") || !strings.HasSuffix(value, "/") || strings.Contains(value, `\`) || strings.ContainsAny(value, "%?#") {
		return errors.New("base path must be / or a canonical absolute path ending in /")
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return errors.New("base path must not contain whitespace or control characters")
		}
	}
	trimmed := strings.TrimSuffix(value, "/")
	if path.Clean(trimmed) != trimmed || strings.Contains(trimmed, "//") {
		return errors.New("base path must not contain duplicate slashes or dot segments")
	}
	return nil
}

func staticExportAssetPaths() []string {
	var result []string
	for _, name := range web.CatalogAssetPaths() {
		if strings.HasSuffix(name, "/manja.wasm") || strings.HasSuffix(name, "/manja.wasm.br") || strings.HasSuffix(name, "/wasm_exec.js") {
			continue
		}
		result = append(result, name)
	}
	return result
}
