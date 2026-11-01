# GitHub static browser fixture

This test-only OpenAPI corpus contains seven real GitHub REST
operations and their complete local `$ref` closure. Six operations come from
the GHEC snapshot; the runner-group operation retains its original GHES 3.0
Markdown description, including the inline enum values that newer upstream
prose removed. Its component references use a `legacy-` namespace. The corpus supplies the Actions cache usage example,
runner labels and visibility, webhook cursor/status parameters,
bodyless responses, and Markdown tables/code needed by the static browser tests.
The retained artifact/retention schema exercises long sidebar labels.

The source is `github/rest-api-description`, commit
`320307e220815d528224ca891cf8a9e3c0243678`, file
`descriptions/ghec/ghec.json`. `provenance.json` records source/fixture/license
sizes and hashes, the source Git blob, the legacy source revision and the
selected operations. The upstream MIT license is retained in `LICENSE.md`. The production GHES 3.0 fixture under
`internal/adapters/openapi/testdata/` is independent of this corpus.

Run the tests offline, without an existing preview or a Manja HTTP server:

```bash
go test ./internal/selfhosted \
  -run 'TestStaticGitHubFixture|TestStaticResponse|TestGitHubMarkdownStaticPreview' \
  -count=1 -v
```

Each browser test generates and verifies its own static export, serves it through
a generic HTTP file server, and resolves content IDs from the exported directory
using upstream `operationId` values. `go test -short` skips browser cases.
`MANJA_GITHUB_MARKDOWN_PREVIEW` remains an optional override for testing a complete
external catalog; that catalog must contain the selected operations.

To reproduce the retained JSON, download the source and `LICENSE.md` from the
immutable commit above, then run:

```bash
python3 internal/selfhosted/testdata/github-static/generate.py \
  --source /tmp/ghec.json --license /tmp/LICENSE.md
```

The generator checks the GHEC, GHES 3.0 and license SHA-256 values, follows
transitive component references, rejects external/unresolved references, and writes deterministic JSON.
Regeneration requires only the Python standard library; tests read tracked files
and never download an upstream description.
