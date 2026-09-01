//go:build !manja_runtime

package selfhosted

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	htmlstd "html"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"github.com/araihu/manja/application/catalog"
	artifact "github.com/araihu/manja/application/htmlartifact"
	artifactstore "github.com/araihu/manja/internal/adapters/htmlartifact"
	"github.com/araihu/manja/internal/localdocs"
	localbrowser "github.com/araihu/manja/internal/localdocs/browser"
	"github.com/araihu/manja/renderer"
	xhtml "golang.org/x/net/html"
)

type exportFragmentIdentity struct {
	Document catalog.DocumentDirectoryV1 `json:"document"`
	Child    catalog.ChildIdentityV1     `json:"child"`
	Schemas  []catalog.ShardReferenceV1  `json:"schemas"`
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
					if err := emitDetailHTMLFragment(workerContext, writer, store, cacheStore, cacheRoot, browser, active, manifest, document, job.child, job.kind, job.resource, binaryIdentity); err != nil {
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
			Document   string                         `json:"document"`
			Target     string                         `json:"target"`
			Operations []catalog.OperationDirectoryV1 `json:"operations"`
		}{document.Key, target, operations})
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
	chunks := (len(document.Operations) + chunkSize - 1) / chunkSize
	if chunks == 0 {
		chunks = 1
	}
	for chunk := 0; chunk < chunks; chunk++ {
		start := chunk * chunkSize
		end := start + chunkSize
		if end > len(document.Operations) {
			end = len(document.Operations)
		}
		resource := document.Key + ":operations:" + strconv.Itoa(chunk)
		identity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSidebar, Resource: resource}
		htmlPath, _, err := artifact.FragmentLocation(active.Mount, document.Key, identity)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(struct {
			Document string                         `json:"document"`
			Chunk    int                            `json:"chunk"`
			Size     int                            `json:"size"`
			Items    []catalog.OperationDirectoryV1 `json:"items"`
		}{document.Key, chunk, chunkSize, document.Operations[start:end]})
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
		output.WriteString(`<ul data-manja-sidebar-operation-chunk="` + strconv.Itoa(chunk) + `" class="grid gap-1">`)
		for _, operation := range document.Operations[start:end] {
			href := descriptor.PublicationBase + "documents/" + url.PathEscape(document.Key) + "/?selected=" + url.QueryEscape(string(operation.DetailID)) + "#" + url.PathEscape(string(operation.DetailID))
			label := strings.TrimSpace(operation.Title)
			if label == "" {
				label = operation.OperationID
			}
			method := strings.ToUpper(strings.TrimSpace(operation.Method))
			output.WriteString(`<li><a data-manja-static-route="true" data-catalog-sidebar-item="true" data-catalog-sidebar-operation="true" data-catalog-method="` + htmlstd.EscapeString(method) + `" href="` + htmlstd.EscapeString(href) + `" class="flex min-w-0 items-center rounded px-2 py-1.5"><span class="mr-2 shrink-0 font-mono text-xs font-bold">` + htmlstd.EscapeString(method) + `</span><span class="min-w-0 flex-1 truncate">` + htmlstd.EscapeString(label) + `</span></a></li>`)
		}
		output.WriteString(`</ul>`)
		if chunk+1 < chunks {
			nextIdentity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSidebar, Resource: document.Key + ":operations:" + strconv.Itoa(chunk+1)}
			nextPath, _, err := artifact.FragmentLocation(active.Mount, document.Key, nextIdentity)
			if err != nil {
				return err
			}
			nextURL := prefixExportBase(descriptor.Static.DeploymentBase, "/"+nextPath)
			output.WriteString(`<div aria-hidden="true" data-manja-sidebar-next-chunk="true" hx-get="` + htmlstd.EscapeString(nextURL) + `" hx-trigger="revealed" hx-swap="outerHTML"></div>`)
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
	return emitSidebarSchemaHTMLChunks(ctx, writer, store, cacheStore, cacheRoot, active, descriptor, manifest, document, binaryIdentity, profile, chunkSize)
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
			Document string                      `json:"document"`
			Chunk    int                         `json:"chunk"`
			Size     int                         `json:"size"`
			Items    []catalog.SchemaDirectoryV1 `json:"items"`
		}{document.Key, chunk, chunkSize, document.Schemas[start:end]})
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
			output.WriteString(`<li><a data-manja-static-route="true" data-catalog-sidebar-item="true" href="` + htmlstd.EscapeString(href) + `" class="flex min-w-0 items-center rounded px-2 py-1.5"><span class="min-w-0 flex-1 truncate">` + htmlstd.EscapeString(schema.Name) + `</span></a></li>`)
		}
		output.WriteString(`</ul>`)
		if chunk+1 < chunks {
			nextIdentity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: artifact.FragmentSidebar, Resource: document.Key + ":schemas:" + strconv.Itoa(chunk+1)}
			nextPath, _, err := artifact.FragmentLocation(active.Mount, document.Key, nextIdentity)
			if err != nil {
				return err
			}
			nextURL := prefixExportBase(descriptor.Static.DeploymentBase, "/"+nextPath)
			output.WriteString(`<div aria-hidden="true" data-manja-sidebar-next-chunk="true" hx-get="` + htmlstd.EscapeString(nextURL) + `" hx-trigger="revealed" hx-swap="outerHTML"></div>`)
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

func emitDetailHTMLFragment(ctx context.Context, writer *exportTreeWriter, store, cacheStore *artifactstore.Store, cacheRoot string, browser *localbrowser.Browser, active renderer.ActivationReceipt, manifest catalog.ManifestV1, document catalog.DocumentDirectoryV1, child catalog.ChildIdentityV1, kind artifact.FragmentKind, resource, binaryIdentity string) error {
	identity := artifact.FragmentIdentity{Format: artifact.FragmentFormatV2, Kind: kind, Resource: resource}
	htmlPath, _, err := artifact.FragmentLocation(active.Mount, document.Key, identity)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(exportFragmentIdentity{Document: document, Child: child, Schemas: document.SchemaNodeShards})
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
			return linkCachedHTMLArtifact(writer, cacheRoot, htmlPath, cached.Manifest)
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
