# Operation Markdown

Manja renders operation and schema descriptions with the Margo Go module.
This includes standalone schema headers/nodes and nested property/item
descriptions supplied through SchemaTree's `DescriptionContent` slot. The
integration targets the checked-in GitHub REST fixture; parameter-level and
response-level descriptions and catalog READMEs retain their existing behavior.
Path parameters use SchemaTree, including Margo-rendered descriptions, required
markers, schema constraints, defaults, and examples. Query and header parameter
groups retain their existing presentation.

The host injects a component factory into the portable renderer's context.
Static export renders the same component ahead of time; browser navigation
fetches verified HTML and never compiles Markdown. Margo is not imported by
the portable/WASM renderer. Without a host factory, descriptions are plain text.

The dependency is pinned to Margo commit `149877346da8`, which supports the
Goshtoso v0.2.10 code-block contract. Margo v0.0.23 predates that contract.

## Composition and assets

Manja uses `Compile`, `Render`, and `RenderHTML`, not Margo's standalone shell
or site generator. Headings, footnotes, and component references receive
operation- or schema-row-scoped IDs. The host serves Margo's embedded document stylesheet once
at `/manja-assets/margo/document.css`; the static manifest verifies those bytes.
Manja supplies document typography/layout tokens without adopting another shell.

Code-copy controls use Manja's existing Goshtoso delegated runtime, with their
target IDs remapped alongside the rendered code. Tables remain static prose
tables in this first integration. Margo's standalone copy/sorting scripts are
not loaded. Unexpected extension dependencies fail rendering instead of being
silently omitted.

## Safety and links

- HTTPS and mail links remain clickable. In-fragment anchors are namespaced.
- Relative links without a known upstream base, missing fragment targets, and
  other destinations remain visible text including the original destination.
  They are not guessed to be Manja routes or rewritten to HTTPS.
- Raw HTML, images, unsupported extensions, and other content rejected by the
  host/compiler retain their escaped plain-text description. Fallback is marked
  with `data-manja-markdown-fallback="plain-text"` for inspection.
- Rendering does not fetch upstream resources or loosen static export validation.

The existing binary identity included in static fragment build keys covers the
pinned Margo dependency and adapter code. Shared stylesheet bytes participate in
the export manifest. There is no unbounded Markdown/document cache.

## Verification

`TestMargoGitHubOperationDescriptions` renders all 529 nonempty operation
descriptions in the fixture directly, requiring success without plain-text
fallback. Adapter tests cover composition, safe fallback, determinism, and
cancellation. To check a generated GitHub deployment in Chromium:

```sh
MANJA_GITHUB_MARKDOWN_PREVIEW=/path/to/static-output \
  GOWORK=off go test ./internal/selfhosted -run TestGitHubMarkdownStaticPreview -v
```

The browser test uses an ordinary file server, verifies lazy operation content,
tables, code copying, one shared stylesheet, network errors, and narrow layout.
