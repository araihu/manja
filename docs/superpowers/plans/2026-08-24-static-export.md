# Static Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `manja export` and `manja export verify` so every configured catalog can be published as a deterministic, same-origin static site at `/` or a canonical subpath, with complete offline local-docs navigation and search.

**Architecture:** The self-hosted adapter builds the existing renderer, captures only its public handler responses plus embedded assets, rewrites parsed HTML for a deployment base, writes a canonical export manifest into a sibling staging directory, verifies the exact tree, and atomically renames it. Static-only descriptor fields activate a complete browser generation and local Wasm rendering; their absence preserves the existing SSR/hybrid path.

**Tech Stack:** Go 1.25, `net/http`, `io/fs`, `encoding/json`, existing `golang.org/x/net/html`, templ/Goshtoso, JavaScript, Service Worker, IndexedDB/CacheStorage, Go WebAssembly, Playwright browser tests.

**Spec:** `docs/superpowers/specs/2026-08-24-static-export-design.md`

## Global Constraints

- Export every validated renderer catalog. Never consult `localDocs.public`, `localDocs.anonymous`, or `localDocs.publicationKey` for selection.
- Static publication keys equal catalog IDs. Existing server descriptors and withdrawal behavior remain unchanged.
- Do not decode or walk the private durable store. Capture the active renderer HTTP surface and validate snapshot children against `manifest.json`.
- Do not pre-render operations or schemas. The static shell plus complete projection/search children must render unseen states in Wasm.
- Do not hand-edit generated `*_templ.go`. Run templ generation only after `.templ` edits.
- Do not add a dependency: `golang.org/x/net/html` is already in the module graph.
- Every task ends with its focused check and a commit. Merge, push, release, deploy, and upload are out of scope.

---

### Task 1: CLI contract and export model

**Files:**
- Create: `cmd/manja/export.go`
- Modify: `cmd/manja/main.go`
- Create: `cmd/manja/export_test.go`
- Create: `internal/selfhosted/export.go`
- Create: `internal/selfhosted/export_test.go`

- [ ] **Step 1: Write failing CLI tests**

Cover creation requiring exactly `--renderer-config`, `--data-dir`, `--output`, and `--base-path`; verification requiring only `--output`; positional/unknown arguments; JSON receipt passthrough; exit codes 0/1/2; and the explicit disclosure warning on stderr.

Use replaceable package variables matching the existing `buildRenderer` seam:

```go
var exportRenderer = app.ExportRenderer
var verifyExport = app.VerifyExport
```

Run: `GOWORK=off go test ./cmd/manja -run 'TestExport' -count=1`
Expected: FAIL because the command does not exist.

- [ ] **Step 2: Write failing model/base-path tests**

Define only the public adapter types required by the CLI:

```go
type ExportOptions struct {
    RendererOptions
    Output   string
    BasePath string
}

type ExportReceipt struct {
    SchemaVersion uint32                 `json:"schemaVersion"`
    BasePath      string                 `json:"basePath"`
    Catalogs      []ExportCatalogReceipt `json:"catalogs"`
    Manifest      string                 `json:"manifest"`
}
```

Table-test `/`, `/group/project/`, and rejection of missing slash, duplicate slash, dot segments, backslash, percent escape, query, fragment, whitespace, and control characters.

Run: `GOWORK=off go test ./internal/selfhosted -run 'TestExport(BasePath|Receipt)' -count=1`
Expected: FAIL because the model/validation does not exist.

- [ ] **Step 3: Implement the thin command and canonical base validation**

Dispatch `export` after `environment.Load()` so `MANJA_RESOURCE_LIMITS` is preserved. Keep `runBuild` and server parsing untouched. Print a bounded warning that all configured catalogs are exported regardless of visibility, then encode the receipt with `json.Encoder`.

Implement canonical base validation with `strings`, `path.Clean`, and rune checks; no URL normalization that would accept invalid input.

- [ ] **Step 4: Run focused tests**

Run:

```bash
GOWORK=off go test ./cmd/manja ./internal/selfhosted -run 'TestExport' -count=1
git diff --check
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/manja internal/selfhosted/export.go internal/selfhosted/export_test.go
git commit -m "feat: add static export command contract"
```

### Task 2: Safe staging, artifact capture, and deterministic manifest

**Files:**
- Modify: `internal/selfhosted/export.go`
- Create: `internal/selfhosted/export_manifest.go`
- Create: `internal/selfhosted/export_manifest_test.go`
- Modify: `internal/web/catalog_assets.go`
- Create: `internal/web/catalog_assets_export_test.go`

- [ ] **Step 1: Expose the existing embedded public asset inventory**

Refactor `NewCatalogAssetsHandler` to reuse one internal inventory helper and expose:

```go
func CatalogAssetPaths() []string
```

It returns a sorted copy of exact `/assets/...` and `/manja-assets/...` regular-file paths already admitted by the handlers. Add a test that every returned path is GET-able and that order is stable.

- [ ] **Step 2: Write failing staging and capture tests**

Use a deterministic `httptest` handler fixture. Cover: absent/empty output accepted; non-empty output rejected without mutation; symlink/non-regular response targets rejected; cancellation removes only the named sibling staging directory; handler non-200, redirect, oversized body, digest mismatch, and undeclared manifest child fail closed.

Run: `GOWORK=off go test ./internal/selfhosted -run 'TestExport(Stage|Capture|Output)' -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement the smallest safe writer and handler capture**

Use `os.MkdirTemp(filepath.Dir(output), "."+filepath.Base(output)+"-export-*")`, `os.OpenFile(..., O_CREATE|O_EXCL|O_WRONLY, 0o644)`, and explicit under-root path checks. Reject redirects and require status 200. Bound bodies using the existing resource limits where available and hard artifact identity lengths from snapshot manifests.

Build with `NewRenderer`; reject any degraded or empty activation receipt. Capture organization/catalog shells, `llms.txt`, immutable manifest/catalog/search/projection/source children, and all public assets through the active handler. Write root `sw.js` from the captured worker bytes; retain the normal embedded path too.

- [ ] **Step 4: Write failing manifest/verifier tests**

Define private canonical structs for schema version, base path, catalog identities, shell routes, descriptors, and sorted file entries. The manifest excludes `_manja/export.json` from entries. Cover deterministic bytes, exact file-set equality, unknown/duplicate JSON fields, mutated bytes, wrong length/hash/media type, missing required files, undeclared extras, and symlinks/devices.

Run: `GOWORK=off go test ./internal/selfhosted -run 'TestExport(Manifest|Verify)' -count=1`
Expected: FAIL.

- [ ] **Step 5: Implement manifest creation and standalone verification**

Use typed `json.Marshal` for compact output and a strict `json.Decoder` with `DisallowUnknownFields` plus EOF check for verification. Sort catalogs and entries before encoding. Walk with `filepath.WalkDir`, use `Lstat`, hash exact regular bytes, and compare the declared set plus the manifest path to the actual set.

Export flow: build/capture → rewrite (Task 3) → create manifest → verify staging → remove an accepted empty output directory → rename staging to output. `VerifyExport` never contacts sources.

- [ ] **Step 6: Run focused tests and commit**

```bash
GOWORK=off go test ./internal/web ./internal/selfhosted -run 'Test(CatalogAssetPaths|Export)' -count=1
git diff --check
git add internal/selfhosted internal/web/catalog_assets.go internal/web/catalog_assets_export_test.go
git commit -m "feat: capture and verify static export artifacts"
```

### Task 3: Static descriptor and parsed HTML rewriting

**Files:**
- Modify: `internal/localdocs/descriptor.go`
- Modify: `internal/localdocs/descriptor_prepare.go`
- Modify: `internal/localdocs/activation.go`
- Modify: `internal/localdocs/descriptor_test.go`
- Modify: `internal/localdocs/activation_test.go`
- Create: `internal/selfhosted/export_html.go`
- Create: `internal/selfhosted/export_html_test.go`
- Modify: `internal/selfhosted/export.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Write failing descriptor tests**

Add optional static fields to `DescriptorV1` only:

```go
type StaticDescriptorV1 struct {
    DeploymentBase    string `json:"deploymentBase"`
    WorkerURL         string `json:"workerUrl"`
    WorkerScope       string `json:"workerScope"`
    OfflineShellURL   string `json:"offlineShellUrl"`
    ExportManifestURL string `json:"exportManifestUrl"`
}
```

Test a new `PrepareStaticDescriptor` from decoded snapshot manifest/directory plus catalog ID, mount, and deployment base. It must set public/anonymous true and publication key to catalog ID without accepting visibility input. Existing `PrepareDescriptor` JSON and validation tests must remain byte-compatible when `Static == nil`.

- [ ] **Step 2: Implement static descriptor preparation and admission**

Reuse the existing identity/digest validation. Permit the trailing static offline-shell URL only when `Static != nil`; retain the existing hybrid computed URL contract otherwise. Validate every static URL as same-origin canonical and inside deployment base.

- [ ] **Step 3: Write failing parsed rewrite tests**

Using `golang.org/x/net/html`, cover root/subpath rewriting for `href`, `src`, canonical links, descriptor injection/replacement, stable catalog/OpenAPI links to immutable files, omission of Page Markdown, preservation of external HTTPS and fragments, and rejection of `/manage`, `/api`, dynamic `search.json`, unknown absolute paths, queries on resource URLs, or escaping the deployment base.

Run: `GOWORK=off go test ./internal/localdocs ./internal/selfhosted -run 'Test(PrepareStaticDescriptor|RewriteExportHTML)' -count=1`
Expected: FAIL.

- [ ] **Step 4: Implement one parsed HTML rewriter**

Parse the document, classify all internal URL-bearing attributes, prefix the deployment base, rewrite known stable downloads from the current catalog/snapshot identity, remove only the static Page Markdown action, and serialize deterministically. Inject `manja-local-docs-descriptor` as escaped JSON script content. Write route shells to directory `index.html`, including trailing `_manja/offline-shell/index.html`.

Run `GOWORK=off go mod tidy` so the already-present `x/net/html` module is marked direct.

- [ ] **Step 5: Run focused tests and commit**

```bash
GOWORK=off go test ./internal/localdocs ./internal/selfhosted -run 'Test(PrepareStaticDescriptor|RewriteExportHTML|Export)' -count=1
git diff --check
git add internal/localdocs internal/selfhosted go.mod go.sum
git commit -m "feat: rewrite static export shells"
```

### Task 4: Complete local generation and Wasm rendering

**Files:**
- Create: `internal/localdocs/browser.go`
- Create: `internal/localdocs/browser_test.go`
- Modify: `cmd/manja-local-docs/main_wasm.go`
- Create: `cmd/manja-local-docs-assets/main.go`
- Create: `cmd/manja-local-docs-assets/main_test.go`
- Modify: `architecture/projection_wasm_build_test.go`

- [ ] **Step 1: Write failing local runtime tests**

Construct one admitted activation from existing projection fixtures, then supply its catalog, search, detail, and schema-node child bytes. Cover complete-generation rejection on missing/extra/mutated children; catalog/document overview; unseen operation; unseen schema; sidebar group state; and search. Assert escaped rendered HTML and canonical href/title metadata.

Run: `GOWORK=off go test ./internal/localdocs -run 'TestBrowser' -count=1`
Expected: FAIL.

- [ ] **Step 2: Implement a single browser runtime composition type**

Add no interface or registry. One `Browser` value owns the admitted `Activation`, decoded catalog, exact child byte map, and existing runtime search service. Reuse `SelectDetail`, `SelectSchemaNode`, `application/catalog` search, and existing `internal/localdocs/render` preparation/components. Return a small typed result containing main HTML, sidebar HTML, title, and canonical path.

Load each referenced schema-node shard once per render into bounded slices. Keep projection bytes untrusted until the existing length/hash/type checks pass. Do not add a browser OpenAPI parser or a second template system.

- [ ] **Step 3: Write failing Wasm ABI tests/build assertions**

Extend the architecture test to require the local browser composition package and continue rejecting filesystem, HTTP-server, and unsafe HTML imports. Add JS-facing functions `prepare`, `render`, and `search`; preserve `activate`, `allows`, and `resolve`.

- [ ] **Step 4: Implement the compact JS/Wasm bridge**

Convert only bounded plain JS objects/byte arrays. `prepare` receives the already-admitted descriptor/manifest plus catalog and exact child bytes, constructs the browser runtime, and swaps global active state only on success. `render` accepts pathname/search/hash state; `search` accepts a bounded query. All failures return `{ok:false,error:"bounded message"}`.

- [ ] **Step 5: Add the missing deterministic asset generator and regenerate Wasm assets**

Add one narrow Go command that reads the fixed local-docs asset list, computes lengths/SHA-256 values, and writes the stable `runtime-assets.js` template. Test deterministic sorted output. Then run:

```bash
GOOS=js GOARCH=wasm GOWORK=off go build -trimpath -o internal/web/static/local-docs/manja.wasm ./cmd/manja-local-docs
brotli -q 11 -w 22 -f -o internal/web/static/local-docs/manja.wasm.br internal/web/static/local-docs/manja.wasm
GOWORK=off go run ./cmd/manja-local-docs-assets -dir internal/web/static/local-docs
```

Never hand-edit digest values.

- [ ] **Step 6: Run focused gates and commit**

```bash
GOWORK=off go test ./internal/localdocs ./architecture -run 'Test(Browser|.*Wasm.*)' -count=1
GOOS=js GOARCH=wasm GOWORK=off go build -trimpath -o /tmp/manja-local-docs.wasm ./cmd/manja-local-docs
git diff --check
git add internal/localdocs cmd/manja-local-docs cmd/manja-local-docs-assets architecture internal/web/static/local-docs/manja.wasm internal/web/static/local-docs/manja.wasm.br internal/web/static/local-docs/runtime-assets.js
git commit -m "feat: render complete local docs in wasm"
```

### Task 5: Static browser activation, routing, storage, and worker

**Files:**
- Modify: `internal/web/static/local-docs.js`
- Modify: `internal/web/static/local-docs/storage.js`
- Modify: `internal/web/static/local-docs/sw.js`
- Modify: `internal/web/static/local-docs/_storage.test.mjs`
- Modify: `internal/web/static/local-docs/_worker.test.mjs`
- Create: `internal/web/static/local-docs/_static.test.mjs`

- [ ] **Step 1: Write failing JavaScript tests**

Cover static descriptor/base validation; complete child preload and hashing; export-manifest identity equality; no-ready state before every search/projection child is committed; initial direct-route render; operation/schema links; sidebar groups; search; `pushState`/`popstate`; HTMX-compatible swaps; failure leaving the shell visible; and zero server-only fetches.

Run:

```bash
node --test internal/web/static/local-docs/_storage.test.mjs internal/web/static/local-docs/_worker.test.mjs internal/web/static/local-docs/_static.test.mjs
```

Expected: FAIL.

- [ ] **Step 2: Extend storage with complete-generation lookup**

Reuse the current IndexedDB metadata and CacheStorage generation namespace. Store validated immutable catalog/search/projection responses under exact URLs. Commit metadata last. Add only the read/list operation required to feed Wasm after reload; no migration abstraction.

- [ ] **Step 3: Implement static activation and router**

Branch only when descriptor static fields are present. Resolve runtime/worker URLs from `deploymentBase`; fetch and validate `_manja/export.json`; preload every current search/projection child; call Wasm `prepare`; then mark ready. Intercept only catalog/document/search links owned by the descriptor, render URL state, update main/sidebar/title/history, and handle `popstate`. Hybrid code remains on its existing path.

- [ ] **Step 4: Implement deployment-aware root worker behavior**

Derive asset prefix and scope from the root worker script URL plus validated descriptor messages. Cache the static shell and admitted same-origin resources. Serve cached navigation shell only for known exported publication routes; pass through unknown, cross-origin, non-GET, out-of-base, management, and API requests. Keep current hybrid network-first fallback unchanged.

- [ ] **Step 5: Run JS tests and commit**

```bash
node --test internal/web/static/local-docs/_storage.test.mjs internal/web/static/local-docs/_worker.test.mjs internal/web/static/local-docs/_static.test.mjs
git diff --check
git add internal/web/static/local-docs
git commit -m "feat: activate static local docs in browser"
```

### Task 6: Verifier links and generic static-server browser acceptance

**Files:**
- Modify: `internal/selfhosted/export_manifest.go`
- Modify: `internal/selfhosted/export_manifest_test.go`
- Create: `internal/selfhosted/export_browser_test.go`
- Modify: `internal/web/static/local_docs_browser_test.go`

- [ ] **Step 1: Write failing internal-link verifier tests**

Cover parsed HTML `href`/`src`, descriptor URLs, worker scope, deployment-prefix removal, query/fragment handling for navigation, exact resolution to a file or route `index.html`, and rejection of runtime-only paths. Include root and `/group/project/` fixtures.

- [ ] **Step 2: Implement verifier URL checks by reusing the Task 3 classifier**

Do not create a second route parser. For each HTML file, parse and classify its internal references, remove the manifest base prefix, and require a declared target. Validate descriptor identities against manifest catalog records and require all runtime/Wasm/search/projection/source files.

- [ ] **Step 3: Write generic static-server browser acceptance**

Export a small two-catalog renderer fixture with local-docs visibility disabled for at least one catalog, then stop using the renderer handler. Serve only the output directory with `http.FileServer` plus correct `.wasm` media type. For `/` and `/group/project/`, assert direct document load/reload, unseen operation, unseen schema, sidebar expansion, search, offline navigation after browser network disconnection, no CDN request, and no Manja/server-only request.

- [ ] **Step 4: Run acceptance and commit**

```bash
GOWORK=off go test ./internal/selfhosted -run 'TestExport(Verify|Browser)' -count=1
GOWORK=off go test ./internal/web -run 'TestLocalDocs' -count=1
git diff --check
git add internal/selfhosted internal/web/static/local_docs_browser_test.go
git commit -m "test: prove static export without manja"
```

### Task 7: Documentation, deterministic generation, and final gates

**Files:**
- Modify: `README.md`
- Modify: `docs/` command documentation selected by existing navigation
- Modify: generated files only if their source changed

- [ ] **Step 1: Document the command and disclosure boundary**

Show creation and verification examples, root/subpath hosting, `.wasm` media-type requirement, exact output replacement semantics, and a prominent statement that every configured catalog is exported regardless of visibility and becomes readable to the static host audience.

- [ ] **Step 2: Run focused command/docs tests**

```bash
GOWORK=off go test ./cmd/manja ./internal/selfhosted ./internal/localdocs ./internal/web ./architecture -count=1
node --test internal/web/static/local-docs/*.test.mjs
```

- [ ] **Step 3: Regenerate and prove stability**

```bash
go run github.com/a-h/templ/cmd/templ generate
git diff --check
```

Run templ generation a second time and require `git diff` to be unchanged from the first generated state.

- [ ] **Step 4: Run full repository gates**

```bash
GOWORK=off go test ./... -count=1
(cd site && GOWORK=off go test ./... -count=1)
GOWORK=off go vet ./...
git diff --check
```

Expected: PASS.

- [ ] **Step 5: Review exact branch diff**

Review `origin/main...HEAD` plus the final working-tree diff against the approved spec. Confirm no visibility filtering, no SSR/hybrid regression, no undeclared output, no placeholder, no generated drift, and no authority expansion.

- [ ] **Step 6: Commit final docs/generated changes**

```bash
git add README.md docs internal/web/static/local-docs
git commit -m "docs: explain static catalog export"
```
