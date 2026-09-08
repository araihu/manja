//go:build !manja_runtime

package selfhosted

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	htmlstd "html"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/araihu/manja/application/catalog"
	artifact "github.com/araihu/manja/application/htmlartifact"
	artifactstore "github.com/araihu/manja/internal/adapters/htmlartifact"
	"github.com/araihu/manja/internal/localdocs"
	localbrowser "github.com/araihu/manja/internal/localdocs/browser"
	localrender "github.com/araihu/manja/internal/localdocs/render"
	"github.com/araihu/manja/renderer"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type exportFragmentIdentity struct {
	Document        catalog.DocumentDirectoryV1 `json:"document"`
	Child           catalog.ChildIdentityV1     `json:"child"`
	Schemas         []catalog.ShardReferenceV1  `json:"schemas"`
	PublicationBase string                      `json:"publicationBase"`
}

type lazySchemaHTMLFragment struct {
	Resource string
	HTML     []byte
}

type sidebarOperationGroup struct {
	ID         string
	Label      string
	Operations []catalog.OperationDirectoryV1
}

type sidebarOperationGroupSummary struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Count      int    `json:"count"`
	Collection string `json:"collection"`
}

func emitCatalogHTMLFragments(ctx context.Context, writer *exportTreeWriter, active renderer.ActivationReceipt, descriptor localdocs.DescriptorV1, manifest catalog.ManifestV1, manifestBytes, catalogBytes []byte, directory catalog.CatalogArtifactV1, cacheRoot string, profile artifact.BuildProfile, fragmentWorkers uint32) error {
	binaryIdentity, err := exportBinaryIdentity()
	if err != nil {
		return err
	}
	loader := func(childPath string) ([]byte, error) {
		child, ok := manifestChild(manifest, childPath)
		if !ok || child.Kind != "detail" && child.Kind != "schema-node" {
			return nil, fmt.Errorf("fragment child %q is not a projection child", childPath)
		}
		_, outputPath, ok := exportedChildPath(active, directory, child)
		if !ok {
			return nil, fmt.Errorf("fragment child %q has no static path", childPath)
		}
		return os.ReadFile(filepath.Join(writer.root, filepath.FromSlash(outputPath)))
	}
	baseBrowser, err := localbrowser.PrepareWithLoader(descriptor, manifestBytes, catalogBytes, loader)
	if err != nil {
		return fmt.Errorf("prepare HTML fragment renderer for catalog %q: %w", active.CatalogID, err)
	}
	store := artifactstore.New(writer.root)
	var cacheStore *artifactstore.Store
	if cacheRoot != "" {
		cacheStore = artifactstore.New(cacheRoot)
	}
	for _, document := range directory.Documents {
		if err := emitSidebarHTMLChunks(ctx, writer, store, cacheStore, cacheRoot, active, descriptor, manifest, document, binaryIdentity, profile); err != nil {
			return err
		}
		if err := emitPathHTMLFragments(ctx, writer, store, cacheStore, cacheRoot, active, descriptor, manifest, document, binaryIdentity); err != nil {
			return err
		}
		type detailJob struct {
			child    catalog.ChildIdentityV1
			kind     artifact.FragmentKind
			resource string
		}
		jobs := make([]detailJob, 0, len(document.Operations)+len(document.Schemas))
		for _, operation := range document.Operations {
			child, ok := manifestChild(manifest, operation.DetailChild)
			if !ok {
				return fmt.Errorf("operation %q detail child is missing", operation.DetailID)
			}
			jobs = append(jobs, detailJob{child: child, kind: artifact.FragmentOperation, resource: string(operation.DetailID)})
		}
		for _, schema := range document.Schemas {
			child, ok := manifestChild(manifest, schema.DetailChild)
			if !ok {
				return fmt.Errorf("schema %q detail child is missing", schema.DetailID)
			}
			jobs = append(jobs, detailJob{child: child, kind: artifact.FragmentSchema, resource: string(schema.DetailID)})
		}
		if len(jobs) == 0 {
			continue
		}
		workerCount := int(fragmentWorkers)
		if workerCount < 1 {
			workerCount = 1
		}
		if workerCount > len(jobs) {
			workerCount = len(jobs)
		}
		jobChannel := make(chan detailJob)
		errChannel := make(chan error, workerCount)
		workerContext, cancel := context.WithCancel(ctx)
		var workers sync.WaitGroup
		for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
			workerBrowser := baseBrowser
			if workerIndex > 0 {
				workerBrowser, err = baseBrowser.ForkWithLoader(loader)
				if err != nil {
					cancel()
					return err
				}
			}
			workers.Add(1)
			go func(browser *localbrowser.Browser) {
				defer workers.Done()
				for job := range jobChannel {
					if workerContext.Err() != nil {
						return
					}
					if err := emitDetailHTMLFragment(workerContext, writer, store, cacheStore, cacheRoot, browser, active, descriptor, manifest, document, job.child, job.kind, job.resource, binaryIdentity); err != nil {
						select {
						case errChannel <- err:
						default:
						}
						cancel()
						return
					}
				}
			}(workerBrowser)
		}
		for _, job := range jobs {
			select {
			case jobChannel <- job:
			case <-workerContext.Done():
				break
			}
			if workerContext.Err() != nil {
				break
			}
		}
		close(jobChannel)
		workers.Wait()
		cancel()
		select {
		case workerErr := <-errChannel:
			return workerErr
		default:
		}
	}
	return nil
}

func emitPathHTMLFragments(ctx context.Context, writer *exportTreeWriter, store, cacheStore *artifactstore.Store, cacheRoot string, active renderer.ActivationReceipt, descriptor localdocs.DescriptorV1, manifest catalog.ManifestV1, document catalog.DocumentDirectoryV1, binaryIdentity string) error {
	byTarget := make(map[string][]catalog.OperationDirectoryV1)
	order := make([]string, 0)
	for _, operation := range document.Operations {
		target := operation.EffectiveRequestTarget()
		if _, ok := byTarget[target]; !ok {
			order = append(order, target)
		}
		byTarget[target] = append(byTarget[target], operation)
	}
	for _, target := range order {
		operations := byTarget[target]
		identity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentPath, Resource: target}
		payload, err := json.Marshal(struct {
			Document        string                         `json:"document"`
			Target          string                         `json:"target"`
			Operations      []catalog.OperationDirectoryV1 `json:"operations"`
			PublicationBase string                         `json:"publicationBase"`
		}{document.Key, target, operations, descriptor.PublicationBase})
		if err != nil {
			return err
		}
		var output strings.Builder
		output.WriteString(`<section data-manja-path-fragment="true"><h2 class="font-title text-2xl font-bold"><code>` + htmlstd.EscapeString(target) + `</code></h2><ul class="mt-4 grid gap-2">`)
		for _, operation := range operations {
			href := descriptor.PublicationBase + "documents/" + url.PathEscape(document.Key) + "/?selected=" + url.QueryEscape(string(operation.DetailID)) + "#" + url.PathEscape(string(operation.DetailID))
			label := strings.TrimSpace(operation.Title)
			if label == "" {
				label = operation.OperationID
			}
			output.WriteString(`<li><a href="` + htmlstd.EscapeString(href) + `"><span class="font-mono font-bold">` + htmlstd.EscapeString(strings.ToUpper(operation.Method)) + `</span> ` + htmlstd.EscapeString(label) + `</a></li>`)
		}
		output.WriteString(`</ul></section>`)
		if err := emitStaticBytesArtifact(ctx, writer, store, cacheStore, cacheRoot, active, manifest, document.Key, identity, payload, []byte(output.String()), binaryIdentity, artifact.BuildProfile{}); err != nil {
			return err
		}
	}
	return nil
}

func emitSidebarHTMLChunks(ctx context.Context, writer *exportTreeWriter, store, cacheStore *artifactstore.Store, cacheRoot string, active renderer.ActivationReceipt, descriptor localdocs.DescriptorV1, manifest catalog.ManifestV1, document catalog.DocumentDirectoryV1, binaryIdentity string, profile artifact.BuildProfile) error {
	chunkSize := int(profile.Resolved().SidebarChunkSize)
	if chunkSize <= 0 {
		return fmt.Errorf("sidebar chunk size is invalid")
	}
	groups := orderedSidebarOperationGroups(document.Operations)
	chunks := (len(groups) + chunkSize - 1) / chunkSize
	if chunks == 0 {
		chunks = 1
	}
	for chunk := 0; chunk < chunks; chunk++ {
		start := chunk * chunkSize
		end := start + chunkSize
		if end > len(groups) {
			end = len(groups)
		}
		resource := document.Key + ":operations:" + strconv.Itoa(chunk)
		identity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSidebar, Resource: resource}
		htmlPath, _, err := artifact.FragmentLocation(active.Mount, document.Key, identity)
		if err != nil {
			return err
		}
		summaries := make([]sidebarOperationGroupSummary, 0, end-start)
		for _, group := range groups[start:end] {
			summaries = append(summaries, sidebarOperationGroupSummary{
				ID: group.ID, Label: group.Label, Count: len(group.Operations), Collection: sidebarOperationGroupCollection(group.ID),
			})
		}
		payload, err := json.Marshal(struct {
			Document        string                         `json:"document"`
			Chunk           int                            `json:"chunk"`
			Size            int                            `json:"size"`
			Groups          []sidebarOperationGroupSummary `json:"groups"`
			PublicationBase string                         `json:"publicationBase"`
		}{document.Key, chunk, chunkSize, summaries, descriptor.PublicationBase})
		if err != nil {
			return err
		}
		compilerIdentity, _ := json.Marshal(manifest.Identity.Versions)
		buildKey, err := artifact.NewBuildKey(artifact.BuildKeyInput{
			Fragment: identity, CanonicalPayloadSHA256: artifact.PayloadSHA256(payload),
			ManjaVersion: binaryIdentity, RendererFingerprint: binaryIdentity, UIFingerprint: binaryIdentity,
			CompilerIdentity: string(compilerIdentity), NormalizerIdentity: manifest.Identity.SourceManifestSHA256, Profile: profile,
		})
		if err != nil {
			return err
		}
		expectation := artifact.Expectation{Fragment: identity, BuildKey: buildKey}
		if cacheStore != nil {
			cached, err := cacheStore.VerifyHTML(ctx, htmlPath, expectation)
			if err != nil {
				return err
			}
			if cached.Hit() {
				if err := linkCachedHTMLArtifact(writer, cacheRoot, htmlPath, cached.Manifest); err != nil {
					return err
				}
				continue
			}
		}
		var output strings.Builder
		for index, group := range groups[start:end] {
			initiallyOpen := chunk == 0 && index == 0
			hidden := ` hidden`
			if initiallyOpen {
				hidden = ""
			}
			collection := sidebarOperationGroupCollection(group.ID)
			output.WriteString(`<section data-manja-sidebar-group="` + group.ID + `" data-manja-sidebar-group-label="` + htmlstd.EscapeString(group.Label) + `"><button type="button" data-manja-static-group="` + group.ID + `" data-catalog-group-control="true" aria-expanded="` + strconv.FormatBool(initiallyOpen) + `" aria-controls="` + group.ID + `-items" class="flex w-full items-center gap-2 border-l border-outline py-2 pl-3 text-left text-sm font-semibold text-on-surface transition hover:border-l-2 hover:text-on-surface-strong dark:border-outline-dark dark:text-on-surface-dark dark:hover:text-on-surface-dark-strong"><span class="min-w-0 flex-1 truncate">` + htmlstd.EscapeString(group.Label) + `</span><span class="shrink-0 text-xs tabular-nums text-on-surface-muted dark:text-on-surface-dark-muted">` + strconv.Itoa(len(group.Operations)) + `</span></button><div id="` + group.ID + `-items" data-manja-sidebar-items="true"` + hidden + `><div aria-hidden="true" data-manja-sidebar-next-chunk="true" data-manja-sidebar-group-load="true" data-manja-sidebar-collection="` + collection + `" data-manja-sidebar-chunk="0"></div></div></section>`)
		}
		if chunk+1 < chunks {
			output.WriteString(`<div aria-hidden="true" data-manja-sidebar-next-chunk="true" data-manja-sidebar-collection="operations" data-manja-sidebar-chunk="` + strconv.Itoa(chunk+1) + `"></div>`)
		}
		if _, err := store.CommitHTML(ctx, htmlPath, expectation, func(target io.Writer) error {
			_, err := io.WriteString(target, output.String())
			return err
		}); err != nil {
			return err
		}
		if err := writer.registerExisting(htmlPath, "text/html"); err != nil {
			return err
		}
		if err := writer.registerExisting(htmlPath+".meta.json", "application/json"); err != nil {
			return err
		}
	}
	for _, group := range groups {
		if err := emitSidebarOperationGroupHTMLChunks(ctx, writer, store, cacheStore, cacheRoot, active, descriptor, manifest, document, group, binaryIdentity, profile, chunkSize); err != nil {
			return err
		}
	}
	return emitSidebarSchemaHTMLChunks(ctx, writer, store, cacheStore, cacheRoot, active, descriptor, manifest, document, binaryIdentity, profile, chunkSize)
}

func orderedSidebarOperationGroups(operations []catalog.OperationDirectoryV1) []sidebarOperationGroup {
	byLabel := make(map[string][]catalog.OperationDirectoryV1)
	for _, operation := range operations {
		label := localrender.OperationGroupLabel(operation)
		byLabel[label] = append(byLabel[label], operation)
	}
	labels := make([]string, 0, len(byLabel))
	for label := range byLabel {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	groups := make([]sidebarOperationGroup, 0, len(labels))
	for _, label := range labels {
		groups = append(groups, sidebarOperationGroup{ID: staticSidebarGroupID("operations-" + label), Label: label, Operations: byLabel[label]})
	}
	return groups
}

func sidebarOperationGroupCollection(groupID string) string {
	return "operation-group-" + groupID
}

func emitSidebarOperationGroupHTMLChunks(ctx context.Context, writer *exportTreeWriter, store, cacheStore *artifactstore.Store, cacheRoot string, active renderer.ActivationReceipt, descriptor localdocs.DescriptorV1, manifest catalog.ManifestV1, document catalog.DocumentDirectoryV1, group sidebarOperationGroup, binaryIdentity string, profile artifact.BuildProfile, chunkSize int) error {
	chunks := (len(group.Operations) + chunkSize - 1) / chunkSize
	for chunk := 0; chunk < chunks; chunk++ {
		start := chunk * chunkSize
		end := start + chunkSize
		if end > len(group.Operations) {
			end = len(group.Operations)
		}
		collection := sidebarOperationGroupCollection(group.ID)
		resource := document.Key + ":" + collection + ":" + strconv.Itoa(chunk)
		identity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSidebar, Resource: resource}
		payload, err := json.Marshal(struct {
			Document        string                         `json:"document"`
			GroupID         string                         `json:"groupId"`
			GroupLabel      string                         `json:"groupLabel"`
			Chunk           int                            `json:"chunk"`
			Size            int                            `json:"size"`
			Operations      []catalog.OperationDirectoryV1 `json:"operations"`
			PublicationBase string                         `json:"publicationBase"`
		}{document.Key, group.ID, group.Label, chunk, chunkSize, group.Operations[start:end], descriptor.PublicationBase})
		if err != nil {
			return err
		}
		var output strings.Builder
		output.WriteString(`<ul data-manja-sidebar-operation-group-chunk="` + strconv.Itoa(chunk) + `" class="grid gap-1 pl-2">`)
		for _, operation := range group.Operations[start:end] {
			href := descriptor.PublicationBase + "documents/" + url.PathEscape(document.Key) + "/?selected=" + url.QueryEscape(string(operation.DetailID)) + "#" + url.PathEscape(string(operation.DetailID))
			label := strings.TrimSpace(operation.Title)
			if label == "" {
				label = operation.OperationID
			}
			method := strings.ToUpper(strings.TrimSpace(operation.Method))
			output.WriteString(`<li><a data-manja-static-route="true" data-catalog-sidebar-item="true" data-catalog-sidebar-operation="true" data-catalog-method="` + htmlstd.EscapeString(method) + `" href="` + htmlstd.EscapeString(href) + `" title="` + htmlstd.EscapeString(label) + `" class="flex min-w-0 items-center gap-2 rounded px-2 py-1.5"><span class="min-w-0 flex-1 truncate">` + htmlstd.EscapeString(label) + `</span><span aria-hidden="true" class="catalog-method-` + staticSidebarMethodClass(method) + ` ml-auto shrink-0 static inline-flex min-h-5 items-center justify-center rounded-radius px-1.5 py-0.5 font-mono text-[10px] font-bold leading-none">` + htmlstd.EscapeString(method) + `</span></a></li>`)
		}
		output.WriteString(`</ul>`)
		if chunk+1 < chunks {
			output.WriteString(`<div aria-hidden="true" data-manja-sidebar-next-chunk="true" data-manja-sidebar-collection="` + collection + `" data-manja-sidebar-chunk="` + strconv.Itoa(chunk+1) + `"></div>`)
		}
		if err := emitStaticBytesArtifact(ctx, writer, store, cacheStore, cacheRoot, active, manifest, document.Key, identity, payload, []byte(output.String()), binaryIdentity, profile); err != nil {
			return err
		}
	}
	return nil
}

func staticSidebarGroupID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "group-" + hex.EncodeToString(digest[:6])
}

func staticSidebarMethodClass(method string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET":
		return "get"
	case "POST":
		return "post"
	case "DELETE":
		return "delete"
	case "PUT", "PATCH":
		return "warning"
	default:
		return "neutral"
	}
}

func emitSidebarSchemaHTMLChunks(ctx context.Context, writer *exportTreeWriter, store, cacheStore *artifactstore.Store, cacheRoot string, active renderer.ActivationReceipt, descriptor localdocs.DescriptorV1, manifest catalog.ManifestV1, document catalog.DocumentDirectoryV1, binaryIdentity string, profile artifact.BuildProfile, chunkSize int) error {
	chunks := (len(document.Schemas) + chunkSize - 1) / chunkSize
	if chunks == 0 {
		chunks = 1
	}
	for chunk := 0; chunk < chunks; chunk++ {
		start := chunk * chunkSize
		end := start + chunkSize
		if end > len(document.Schemas) {
			end = len(document.Schemas)
		}
		resource := document.Key + ":schemas:" + strconv.Itoa(chunk)
		identity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSidebar, Resource: resource}
		htmlPath, _, err := artifact.FragmentLocation(active.Mount, document.Key, identity)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(struct {
			Document        string                      `json:"document"`
			Chunk           int                         `json:"chunk"`
			Size            int                         `json:"size"`
			Items           []catalog.SchemaDirectoryV1 `json:"items"`
			PublicationBase string                      `json:"publicationBase"`
		}{document.Key, chunk, chunkSize, document.Schemas[start:end], descriptor.PublicationBase})
		if err != nil {
			return err
		}
		compilerIdentity, _ := json.Marshal(manifest.Identity.Versions)
		buildKey, err := artifact.NewBuildKey(artifact.BuildKeyInput{
			Fragment: identity, CanonicalPayloadSHA256: artifact.PayloadSHA256(payload),
			ManjaVersion: binaryIdentity, RendererFingerprint: binaryIdentity, UIFingerprint: binaryIdentity,
			CompilerIdentity: string(compilerIdentity), NormalizerIdentity: manifest.Identity.SourceManifestSHA256, Profile: profile,
		})
		if err != nil {
			return err
		}
		expectation := artifact.Expectation{Fragment: identity, BuildKey: buildKey}
		if cacheStore != nil {
			cached, err := cacheStore.VerifyHTML(ctx, htmlPath, expectation)
			if err != nil {
				return err
			}
			if cached.Hit() {
				if err := linkCachedHTMLArtifact(writer, cacheRoot, htmlPath, cached.Manifest); err != nil {
					return err
				}
				continue
			}
		}
		var output strings.Builder
		output.WriteString(`<ul data-manja-sidebar-schema-chunk="` + strconv.Itoa(chunk) + `" class="grid gap-1">`)
		for _, schema := range document.Schemas[start:end] {
			href := descriptor.PublicationBase + "documents/" + url.PathEscape(document.Key) + "/?selected=" + url.QueryEscape(string(schema.DetailID)) + "#" + url.PathEscape(string(schema.DetailID))
			output.WriteString(`<li><a data-manja-static-route="true" data-catalog-sidebar-item="true" href="` + htmlstd.EscapeString(href) + `" title="` + htmlstd.EscapeString(schema.Name) + `" class="flex min-w-0 items-center rounded px-2 py-1.5"><span class="min-w-0 flex-1 truncate">` + htmlstd.EscapeString(schema.Name) + `</span></a></li>`)
		}
		output.WriteString(`</ul>`)
		if chunk+1 < chunks {
			output.WriteString(`<div aria-hidden="true" data-manja-sidebar-next-chunk="true" data-manja-sidebar-collection="schemas" data-manja-sidebar-chunk="` + strconv.Itoa(chunk+1) + `"></div>`)
		}
		if _, err := store.CommitHTML(ctx, htmlPath, expectation, func(target io.Writer) error {
			_, err := io.WriteString(target, output.String())
			return err
		}); err != nil {
			return err
		}
		if err := writer.registerExisting(htmlPath, "text/html"); err != nil {
			return err
		}
		if err := writer.registerExisting(htmlPath+".meta.json", "application/json"); err != nil {
			return err
		}
	}
	return nil
}

func linkCachedHTMLArtifact(writer *exportTreeWriter, cacheRoot, htmlPath string, manifest artifact.Manifest) error {
	if err := writer.linkExisting(cacheRoot, htmlPath, "text/html", manifest.Content.Length, manifest.Content.SHA256); err != nil {
		return err
	}
	sidecarPath := htmlPath + ".meta.json"
	data, err := os.ReadFile(filepath.Join(cacheRoot, filepath.FromSlash(sidecarPath)))
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	return writer.linkExisting(cacheRoot, sidecarPath, "application/json", uint64(len(data)), hex.EncodeToString(digest[:]))
}

func emitDetailHTMLFragment(ctx context.Context, writer *exportTreeWriter, store, cacheStore *artifactstore.Store, cacheRoot string, browser *localbrowser.Browser, active renderer.ActivationReceipt, descriptor localdocs.DescriptorV1, manifest catalog.ManifestV1, document catalog.DocumentDirectoryV1, child catalog.ChildIdentityV1, kind artifact.FragmentKind, resource, binaryIdentity string) error {
	identity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: kind, Resource: resource}
	htmlPath, _, err := artifact.FragmentLocation(active.Mount, document.Key, identity)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(exportFragmentIdentity{Document: document, Child: child, Schemas: document.SchemaNodeShards, PublicationBase: descriptor.PublicationBase})
	if err != nil {
		return fmt.Errorf("encode fragment identity: %w", err)
	}
	compilerIdentity, err := json.Marshal(manifest.Identity.Versions)
	if err != nil {
		return fmt.Errorf("encode fragment compiler identity: %w", err)
	}
	buildKey, err := artifact.NewBuildKey(artifact.BuildKeyInput{
		Fragment: identity, CanonicalPayloadSHA256: artifact.PayloadSHA256(payload),
		ManjaVersion: binaryIdentity, RendererFingerprint: binaryIdentity, UIFingerprint: binaryIdentity,
		CompilerIdentity: string(compilerIdentity), NormalizerIdentity: manifest.Identity.SourceManifestSHA256,
	})
	if err != nil {
		return err
	}
	expectation := artifact.Expectation{Fragment: identity, BuildKey: buildKey}
	if cacheStore != nil {
		cached, cacheErr := cacheStore.VerifyHTML(ctx, htmlPath, expectation)
		if cacheErr != nil {
			return cacheErr
		}
		if cached.Hit() {
			if kind != artifact.FragmentOperation {
				return linkCachedHTMLArtifact(writer, cacheRoot, htmlPath, cached.Manifest)
			}
			linked, linkErr := linkCachedOperationArtifacts(ctx, writer, cacheStore, cacheRoot, htmlPath, cached.Manifest, resource)
			if linkErr != nil {
				return linkErr
			}
			if linked {
				return nil
			}
		}
	}
	verification, err := store.VerifyHTML(ctx, htmlPath, expectation)
	if err != nil {
		return err
	}
	if !verification.Hit() {
		page, renderErr := browser.Render(ctx, localbrowser.Route{DocumentKey: document.Key, Selected: resource})
		browser.ReleaseChildren()
		if renderErr != nil {
			page.MainHTML = fallbackDetailHTML(document, kind, resource)
		}
		fragmentBytes, rewriteErr := rewriteExportFragmentHTML([]byte(page.MainHTML), "/", nil)
		if rewriteErr != nil {
			return fmt.Errorf("rewrite %s fragment %q: %w", kind, resource, rewriteErr)
		}
		if kind == artifact.FragmentOperation {
			var schemas []lazySchemaHTMLFragment
			fragmentBytes, schemas, rewriteErr = extractLazySchemaHTMLFragments(fragmentBytes)
			if rewriteErr != nil {
				return fmt.Errorf("extract lazy schemas from operation fragment %q: %w", resource, rewriteErr)
			}
			for _, schema := range schemas {
				schemaIdentity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSchema, Resource: schema.Resource}
				if err := emitStaticBytesArtifact(ctx, writer, store, cacheStore, cacheRoot, active, manifest, document.Key, schemaIdentity, schema.HTML, schema.HTML, binaryIdentity, artifact.BuildProfile{}); err != nil {
					return err
				}
			}
		}
		_, err = store.CommitHTML(ctx, htmlPath, expectation, func(output io.Writer) error {
			_, writeErr := output.Write(fragmentBytes)
			return writeErr
		})
		if err != nil {
			return err
		}
	}
	if err := writer.registerExisting(htmlPath, "text/html"); err != nil {
		return err
	}
	if err := writer.registerExisting(htmlPath+".meta.json", "application/json"); err != nil {
		return err
	}
	if kind == artifact.FragmentOperation {
		operationHTML, err := os.ReadFile(filepath.Join(writer.root, filepath.FromSlash(htmlPath)))
		if err != nil {
			return err
		}
		exampleHTML, ok, err := extractHTMLElement(operationHTML, "data-manja-request-samples", "true")
		if err != nil {
			return err
		}
		if ok {
			exampleIdentity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentExample, Resource: resource}
			examplePayload := append(append([]byte(nil), payload...), []byte("\nexample-fragment-v2")...)
			return emitStaticBytesArtifact(ctx, writer, store, cacheStore, cacheRoot, active, manifest, document.Key, exampleIdentity, examplePayload, exampleHTML, binaryIdentity, artifact.BuildProfile{})
		}
	}
	return nil
}

func linkCachedOperationArtifacts(ctx context.Context, writer *exportTreeWriter, cacheStore *artifactstore.Store, cacheRoot, operationPath string, operationManifest artifact.Manifest, resource string) (bool, error) {
	operationHTML, err := os.ReadFile(filepath.Join(cacheRoot, filepath.FromSlash(operationPath)))
	if err != nil {
		return false, nil
	}
	resources, err := lazySchemaHTMLResources(operationHTML)
	if err != nil {
		return false, err
	}
	type cachedDependency struct {
		path     string
		manifest artifact.Manifest
	}
	dependencies := make([]cachedDependency, 0, len(resources)+1)
	for _, schemaResource := range resources {
		identity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSchema, Resource: schemaResource}
		dependency, ok, verifyErr := verifiedCachedHTMLArtifact(ctx, cacheStore, cacheRoot, operationPath, identity)
		if verifyErr != nil {
			return false, verifyErr
		}
		if !ok {
			return false, nil
		}
		dependencies = append(dependencies, dependency)
	}
	if _, ok, extractErr := extractHTMLElement(operationHTML, "data-manja-request-samples", "true"); extractErr != nil {
		return false, extractErr
	} else if ok {
		identity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentExample, Resource: resource}
		dependency, verified, verifyErr := verifiedCachedHTMLArtifact(ctx, cacheStore, cacheRoot, operationPath, identity)
		if verifyErr != nil {
			return false, verifyErr
		}
		if !verified {
			return false, nil
		}
		dependencies = append(dependencies, dependency)
	}
	for _, dependency := range dependencies {
		if err := linkCachedHTMLArtifact(writer, cacheRoot, dependency.path, dependency.manifest); err != nil {
			return false, err
		}
	}
	if err := linkCachedHTMLArtifact(writer, cacheRoot, operationPath, operationManifest); err != nil {
		return false, err
	}
	return true, nil
}

func verifiedCachedHTMLArtifact(ctx context.Context, cacheStore *artifactstore.Store, cacheRoot, operationPath string, identity artifact.FragmentIdentity) (struct {
	path     string
	manifest artifact.Manifest
}, bool, error) {
	var result struct {
		path     string
		manifest artifact.Manifest
	}
	fragmentRoot := path.Dir(path.Dir(filepath.ToSlash(operationPath)))
	syntheticPath, _, err := artifact.FragmentLocation("/", "document", identity)
	if err != nil {
		return result, false, err
	}
	directory, ok := map[artifact.FragmentKind]string{
		artifact.FragmentSchema:  "schemas",
		artifact.FragmentExample: "examples",
	}[identity.Kind]
	if !ok {
		return result, false, fmt.Errorf("cached operation dependency kind %q is invalid", identity.Kind)
	}
	htmlPath := path.Join(fragmentRoot, directory, path.Base(syntheticPath))
	sidecar, err := os.ReadFile(filepath.Join(cacheRoot, filepath.FromSlash(htmlPath+".meta.json")))
	if err != nil {
		return result, false, nil
	}
	if err := json.Unmarshal(sidecar, &result.manifest); err != nil || result.manifest.Validate() != nil || result.manifest.Fragment != identity {
		return result, false, nil
	}
	verification, err := cacheStore.VerifyHTML(ctx, htmlPath, artifact.Expectation{Fragment: identity, BuildKey: result.manifest.BuildKey})
	if err != nil {
		return result, false, err
	}
	if !verification.Hit() {
		return result, false, nil
	}
	result.path = htmlPath
	result.manifest = verification.Manifest
	return result, true, nil
}

func extractLazySchemaHTMLFragments(input []byte) ([]byte, []lazySchemaHTMLFragment, error) {
	container := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := xhtml.ParseFragment(bytes.NewReader(input), container)
	if err != nil {
		return nil, nil, err
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}
	var trees []*xhtml.Node
	var collect func(*xhtml.Node)
	collect = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && hasHTMLClass(node, "manja-schema-tree") {
			trees = append(trees, node)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(container)
	fragments := make([]lazySchemaHTMLFragment, 0, len(trees))
	seen := make(map[string]struct{}, len(trees))
	for _, tree := range trees {
		label := htmlAttribute(tree, "aria-label")
		if label == "" {
			label = "Schema tree"
		}
		canonicalizeStandaloneSchemaHTML(tree)
		var rendered bytes.Buffer
		if err := xhtml.Render(&rendered, tree); err != nil {
			return nil, nil, err
		}
		hash := sha256.Sum256(rendered.Bytes())
		resource := "tree-sha256-" + hex.EncodeToString(hash[:])
		if _, ok := seen[resource]; !ok {
			fragments = append(fragments, lazySchemaHTMLFragment{Resource: resource, HTML: append([]byte(nil), rendered.Bytes()...)})
			seen[resource] = struct{}{}
		}
		placeholder := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", Attr: []xhtml.Attribute{
			{Key: "data-manja-static-schema-fragment", Val: "true"},
			{Key: "data-manja-schema-resource", Val: resource},
			{Key: "data-manja-schema-state", Val: "idle"},
			{Key: "aria-label", Val: label},
			{Key: "aria-busy", Val: "false"},
			{Key: "class", Val: "min-h-12 rounded-radius border border-outline p-3 dark:border-outline-dark"},
		}}
		message := &xhtml.Node{Type: xhtml.ElementNode, Data: "p", Attr: []xhtml.Attribute{{Key: "data-manja-schema-placeholder", Val: "true"}, {Key: "class", Val: "text-sm text-on-surface-muted dark:text-on-surface-dark-muted"}}}
		message.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: "Schema loads when visible."})
		placeholder.AppendChild(message)
		retry := &xhtml.Node{Type: xhtml.ElementNode, Data: "button", Attr: []xhtml.Attribute{{Key: "type", Val: "button"}, {Key: "data-manja-schema-retry", Val: "true"}, {Key: "hidden", Val: ""}}}
		retry.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: "Retry schema"})
		placeholder.AppendChild(retry)
		parent := tree.Parent
		if parent == nil {
			return nil, nil, errors.New("schema tree has no parent")
		}
		parent.InsertBefore(placeholder, tree)
		parent.RemoveChild(tree)
	}
	var output bytes.Buffer
	for child := container.FirstChild; child != nil; child = child.NextSibling {
		if err := xhtml.Render(&output, child); err != nil {
			return nil, nil, err
		}
	}
	return output.Bytes(), fragments, nil
}

func lazySchemaHTMLResources(input []byte) ([]string, error) {
	container := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := xhtml.ParseFragment(bytes.NewReader(input), container)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		container.AppendChild(node)
	}
	var resources []string
	seen := make(map[string]struct{})
	var visit func(*xhtml.Node) error
	visit = func(node *xhtml.Node) error {
		if node.Type == xhtml.ElementNode && hasHTMLAttribute(node, "data-manja-static-schema-fragment", "true") {
			resource := htmlAttribute(node, "data-manja-schema-resource")
			if len(resource) != len("tree-sha256-")+sha256.Size*2 || !strings.HasPrefix(resource, "tree-sha256-") {
				return errors.New("lazy schema resource is invalid")
			}
			if _, err := hex.DecodeString(strings.TrimPrefix(resource, "tree-sha256-")); err != nil {
				return errors.New("lazy schema resource is invalid")
			}
			if _, ok := seen[resource]; !ok {
				resources = append(resources, resource)
				seen[resource] = struct{}{}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	return resources, visit(container)
}

func canonicalizeStandaloneSchemaHTML(root *xhtml.Node) {
	setHTMLAttribute(root, "aria-label", "Schema tree")
	ids := make(map[string]string)
	var idOrder []string
	var collect func(*xhtml.Node)
	collect = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode {
			if id := htmlAttribute(node, "id"); id != "" {
				if _, ok := ids[id]; !ok {
					idOrder = append(idOrder, id)
					ids[id] = "manja-schema-tree-id-" + strconv.Itoa(len(idOrder))
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(root)
	var rewrite func(*xhtml.Node)
	rewrite = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode {
			for index := range node.Attr {
				attribute := &node.Attr[index]
				if attribute.Key == "id" {
					attribute.Val = ids[attribute.Val]
					continue
				}
				if attribute.Key == "href" && strings.HasPrefix(attribute.Val, "#") {
					if replacement, ok := ids[strings.TrimPrefix(attribute.Val, "#")]; ok {
						attribute.Val = "#" + replacement
					}
					continue
				}
				switch attribute.Key {
				case "aria-controls", "aria-describedby", "aria-labelledby", "data-tooltip-content-id", "for":
					parts := strings.Fields(attribute.Val)
					for partIndex, part := range parts {
						if replacement, ok := ids[part]; ok {
							parts[partIndex] = replacement
						}
					}
					attribute.Val = strings.Join(parts, " ")
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			rewrite(child)
		}
	}
	rewrite(root)
}

func hasHTMLClass(node *xhtml.Node, class string) bool {
	for _, candidate := range strings.Fields(htmlAttribute(node, "class")) {
		if candidate == class {
			return true
		}
	}
	return false
}

func htmlAttribute(node *xhtml.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

func fallbackDetailHTML(document catalog.DocumentDirectoryV1, kind artifact.FragmentKind, resource string) string {
	title := resource
	description := ""
	meta := ""
	if kind == artifact.FragmentOperation {
		for _, operation := range document.Operations {
			if string(operation.DetailID) != resource {
				continue
			}
			title = strings.TrimSpace(operation.Title)
			if title == "" {
				title = operation.OperationID
			}
			description = operation.Description
			meta = `<p><strong>` + htmlstd.EscapeString(strings.ToUpper(operation.Method)) + `</strong> <code>` + htmlstd.EscapeString(operation.EffectiveRequestTarget()) + `</code></p>`
			break
		}
	} else if kind == artifact.FragmentSchema {
		for _, schema := range document.Schemas {
			if string(schema.DetailID) == resource {
				title, description = schema.Name, schema.Description
				break
			}
		}
	}
	return `<div data-manja-local-main="true" data-manja-fragment-degraded="true"><article><h1 tabindex="-1" data-manja-settled-focus="true" class="font-title text-3xl font-bold">` + htmlstd.EscapeString(title) + `</h1>` + meta + `<p>` + htmlstd.EscapeString(description) + `</p></article></div>`
}

func emitStaticBytesArtifact(ctx context.Context, writer *exportTreeWriter, store, cacheStore *artifactstore.Store, cacheRoot string, active renderer.ActivationReceipt, manifest catalog.ManifestV1, documentKey string, identity artifact.FragmentIdentity, payload, htmlBytes []byte, binaryIdentity string, profile artifact.BuildProfile) error {
	htmlPath, _, err := artifact.FragmentLocation(active.Mount, documentKey, identity)
	if err != nil {
		return err
	}
	compilerIdentity, err := json.Marshal(manifest.Identity.Versions)
	if err != nil {
		return err
	}
	buildKey, err := artifact.NewBuildKey(artifact.BuildKeyInput{
		Fragment: identity, CanonicalPayloadSHA256: artifact.PayloadSHA256(payload),
		ManjaVersion: binaryIdentity, RendererFingerprint: binaryIdentity, UIFingerprint: binaryIdentity,
		CompilerIdentity: string(compilerIdentity), NormalizerIdentity: manifest.Identity.SourceManifestSHA256, Profile: profile,
	})
	if err != nil {
		return err
	}
	expectation := artifact.Expectation{Fragment: identity, BuildKey: buildKey}
	if cacheStore != nil {
		cached, err := cacheStore.VerifyHTML(ctx, htmlPath, expectation)
		if err != nil {
			return err
		}
		if cached.Hit() {
			return linkCachedHTMLArtifact(writer, cacheRoot, htmlPath, cached.Manifest)
		}
	}
	if _, err := store.CommitHTML(ctx, htmlPath, expectation, func(target io.Writer) error {
		_, err := target.Write(htmlBytes)
		return err
	}); err != nil {
		return err
	}
	if err := writer.registerExisting(htmlPath, "text/html"); err != nil {
		return err
	}
	return writer.registerExisting(htmlPath+".meta.json", "application/json")
}

func extractHTMLElement(input []byte, attribute, value string) ([]byte, bool, error) {
	document, err := xhtml.Parse(strings.NewReader(string(input)))
	if err != nil {
		return nil, false, err
	}
	var found *xhtml.Node
	var visit func(*xhtml.Node)
	visit = func(node *xhtml.Node) {
		if found != nil {
			return
		}
		if node.Type == xhtml.ElementNode && hasHTMLAttribute(node, attribute, value) {
			found = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	if found == nil {
		return nil, false, nil
	}
	var output strings.Builder
	if err := xhtml.Render(&output, found); err != nil {
		return nil, false, err
	}
	return []byte(output.String()), true, nil
}

func exportBinaryIdentity() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve Manja executable: %w", err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		return "", fmt.Errorf("read Manja executable: %w", err)
	}
	digest := sha256.Sum256(data)
	version := "devel"
	if info, ok := debug.ReadBuildInfo(); ok && strings.TrimSpace(info.Main.Version) != "" {
		version = info.Main.Version
	}
	return "manja:" + version + ":binary-sha256:" + hex.EncodeToString(digest[:]), nil
}
