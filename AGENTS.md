# AGENTS.md - Manja

This file gives external contributors and optional AI coding tools the small
set of repo rules that matter most. `CLAUDE.md` is a symlink to this file for
harnesses that look for that name. `docs/superpowers/` contains agent-facing
specs and plans; use it as implementation context, not public product docs. The
curated `.agents/skills/` entries for Alpine.js, HTMX, Tailwind CSS, and templ
stay tracked as optional core-stack references.

## Project

Manja is a hosted OpenAPI renderer and publisher built with Goshtoso. It loads
OpenAPI specs from file and Git-like sources, indexes operations and schemas,
stores publication metadata locally, and renders Goshtoso-native public docs.

The current public docs surface is read-only, search-first, and server-rendered.
It should not grow a v1 "try it" console or server-side proxy for upstream API
requests.

## Commands

```bash
# Install Node tools for Redocly, httpsnippet, and openapi-sampler.
npm ci

# Verify the API description and regenerate the untracked bundled spec.
npm run api:bundle
npm run api:lint

# Regenerate API Go types after changing api/*.yaml.
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen \
  -generate types,strict-server \
  -package web \
  -o internal/web/api.gen.go \
  api/dist/openapi.yaml

# Regenerate templ output after editing .templ files.
go run github.com/a-h/templ/cmd/templ generate

# Unit and E2E tests.
go test ./...

# Integration tests that require container runtime access.
go test -tags=integration ./internal/integration -v

# Run the local public docs server with Air reload and per-worktree ports.
npm run dev
```

`npm run dev` prints the raw renderer URL, the Air reload proxy URL, and the
product site URL. Open and share the product site URL when the task asks for the
site; use the Air proxy URL when you are inspecting the standalone renderer. By
default the renderer uses the GitHub REST fixture from
`internal/adapters/openapi/testdata/github-v3-rest.json`; pass extra `manja`
arguments after `--`, for example:

```bash
npm run dev -- -spec internal/adapters/openapi/testdata/github-v3-rest.json
```

Use `go run ./cmd/manja -data-dir .manja/data` only when you deliberately need a
non-reloading raw server.

## Air Development Workflow

Use Air for local auto reload. Do not default to `templ generate --watch` for
normal sessions: templ's watcher handles `.templ` and Go rebuilds, but Manja's
dev loop also needs schema-example bundling and optional Tailwind CSS output.

The standard command is:

```bash
npm run dev
```

This runs `scripts/dev-server.mjs`, which chooses stable available ports per
worktree, passes the renderer ports to Air, runs the Air proxy, and starts the
product site server from the `site/` module. Inspect the chosen ports without
starting the servers with:

```bash
node scripts/dev-server.mjs --print-ports
```

Rules:

- Open and share the product site URL for site/docs work. Open the Air proxy
  URL, not the raw app URL, when inspecting the standalone renderer so browser
  reload is active.
- Let the launcher allocate ports for parallel worktrees. Pin ports only when a
  task explicitly needs it, using `MANJA_DEV_APP_PORT`,
  `MANJA_DEV_PROXY_PORT`, `MANJA_DEV_SITE_PORT`, `--app-port`, `--proxy-port`,
  or `--site-port`.
- Keep `.air.toml` as the single watcher config. Its build command is
  `npm run dev:build`, which regenerates templ output, rebuilds the schema
  example and request composer assets, runs `npm run css:build --if-present`,
  and builds `./tmp/manja-dev`.
- Never add generated outputs to Air watch triggers. In particular, keep
  `_templ.go`, `internal/web/static/schema-example.js`,
  `internal/web/static/request-composer.js`, generated CSS, `api/dist`,
  `.manja`, `tmp`, `vendor`, `node_modules`, and `.git` excluded unless you
  are deliberately changing the dev-loop contract.
- If you change `.air.toml`, `scripts/dev-server.mjs`, generated output paths,
  templ generation, schema-example bundling, request-composer bundling, or CSS
  build behavior, stress-test server startup and rebuild loops before merging. Run
  `npm run dev`, wait for the initial build to settle, touch representative
  source files (`.templ`, source CSS/JS, or Go), and confirm generated writes do
  not trigger repeated builds while the servers are idle. Confirm both the
  product site URL and Air proxy URL return HTTP 200.

When the worktree PR merges into `main`, stop any server process started for
that worktree before removing it. Cleanup should include removing the worktree
and pruning stale worktree metadata:

```bash
git worktree remove /tmp/manja-<short-slug>
git worktree prune
```

## Worktree Isolation (required)

Every unit of work - a feature, bugfix, review fix, API slice, UI polish pass,
or coverage pass - MUST happen in its own dedicated git worktree branched from
`origin/main`. Never edit, build, or commit feature work in the shared primary
checkout, and never reuse one worktree for two unrelated tasks. Concurrent
agents sharing a single working tree produce entangled diffs, generated files
collide, and clean per-feature PRs become difficult to assemble.

Start every task with:

```bash
git fetch origin
git worktree add -b <type>/<short-slug> /tmp/manja-<short-slug> origin/main
cd /tmp/manja-<short-slug>
npm ci
```

Rules:

- Branch from `origin/main` only - never from the current branch or another
  feature branch. Run `git fetch origin` first so the base is up to date.
- One worktree per task. If two agents work in parallel, they get two worktrees.
- Do all edits, generation, builds, tests, and commits inside that worktree. The
  primary checkout stays clean.
- If you use a repo-local worktree directory instead of `/tmp`, make sure it is
  gitignored before creating the worktree.
- Remove the worktree after the branch merges and stop any server processes
  started for it.

When you or the harness hand a task to a sub-agent, the worktree boundary is how
that work stays mergeable.

## Generated Files

Never hand-edit generated files:

- `internal/web/templates/*_templ.go` - regenerate with
  `go run github.com/a-h/templ/cmd/templ generate`
- `internal/web/static/request-composer.js` - regenerate with
  `npm run examples:build`
- `internal/web/api.gen.go` - regenerate from `api/dist/openapi.yaml` with
  `oapi-codegen`
- `api/dist/openapi.yaml` - regenerate with `npm run api:bundle`; it is ignored
  and should not be committed

When resolving merge conflicts, resolve source `.templ` files and split
`api/*.yaml` files first, then regenerate. Do not hand-resolve generated templ
or oapi-codegen output unless the generator itself is being changed.

## Architecture

- Keep provider-neutral domain behavior in the public `domain` package.
- Keep reusable orchestration in the public `application` package and its
  context-first infrastructure contracts in `application/port`.
- Keep persistence, source, parser, Markdown, and cache details in
  `internal/adapters`.
- Keep concrete self-hosted wiring in `internal/selfhosted`; `cmd/manja` is the
  executable composition entry point. Raw credentials must terminate at this
  composition boundary or an internal secret/source adapter, while public APIs
  use opaque secret references.
- Keep HTTP routing and server-rendered public docs in `internal/web`.
- Keep the management REST API narrow and API-first; only add API surface that a
  real integration needs.

Prefer ports-first boundaries over wiring concrete adapters into domain or
application code. Consistency-sensitive operational changes cross one coarse
`UnitOfWork`; immutable specs use content-addressed blob keys written before
metadata commits. Use filesystem-backed adapters for the local vertical slice,
and make source sync behavior preserve last-known-good publication state when
parsing or source access fails.

## Public Docs UI

Use Goshtoso primitives before inventing Manja-only UI. The public docs should
feel like a precise documentation workbench: spec title, version, search,
operations, schemas, and source freshness should be easy to scan without a
marketing prelude.

Rules:

- Keep global Ctrl+K/Cmd+K search as a first-class navigation path.
- Keep search result DOM IDs distinct from content anchors, and make every
  search href resolve to a visible section target.
- Render Markdown under `.manja-markdown` and map styles back to Goshtoso
  semantic tokens.
- Use `var(--color-*)` and Goshtoso classes such as `bg-surface`,
  `text-on-surface`, `border-outline`, and `rounded-radius`; do not hard-code
  palette colors when a semantic token exists.
- Do not add decorative dashboards, hero sections, gradients, glassmorphism,
  Tailwind Typography `prose`, or a v1 "try it" proxy surface.

## Quality Gates

Before opening a PR, run the gates relevant to the change:

```bash
npm ci
npm run api:bundle
npm run api:lint
go run github.com/a-h/templ/cmd/templ generate
go test ./...
```

Also run the oapi-codegen command when API YAML changes, and run integration
tests when touching Dex, Forgejo, source adapter, or container-backed flows.
