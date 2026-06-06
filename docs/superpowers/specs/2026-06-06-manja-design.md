# Manja Design

Date: 2026-06-06

## Purpose

Manja is a hosted OpenAPI renderer and publisher built with Goshtoso. It points at
a source where an OpenAPI spec file lives, indexes the spec, renders read-only
documentation, and lets users intentionally publish selected versions.

The first iteration is search-first. The most important public experience is fast
navigation across operations, schemas, tags, and versions through a high-quality
Ctrl+K-style command palette.

## Architecture

Manja uses hexagonal architecture. The core domain is provider-neutral and deals
with spec files, metadata, indexes, visibility, and rendering inputs. Adapters
bring bytes into the core or expose core results through UI and management
surfaces.

Core nouns:

- `Project`: owner-facing container for sources, theme settings, SEO metadata,
  publications, and access policy.
- `Source`: configured input for a spec file. Git is the first source adapter.
- `Credential`: reference to secret material used by a source.
- `SpecFile`: raw spec bytes plus format and path metadata.
- `Revision`: a discovered version of a source, such as a branch, tag, commit, or
  singleton file revision for non-git sources.
- `SpecIndex`: searchable and renderable representation derived from an OpenAPI
  document.
- `Publication`: intentional public exposure of a spec revision.
- `ThemeSettings`: Goshtoso theme preferences for a project.
- `ProjectSEO`: public docs metadata such as title template, description,
  canonical base URL, robots/indexing preference, and social image.

The core must not assume that bytes came from git, upload, webhook, polling, or a
future Kubernetes API-server-backed adapter.

## Source And Sync

Git is the first source adapter. It should remain provider-agnostic and support
methods common to GitHub and GitLab:

- public unauthenticated fetch
- HTTPS token credentials
- SSH credentials
- refs from branches, tags, and commits when configured

GitHub, GitLab, and similar webhooks are inbound adapters that mark a source stale
or enqueue sync. They are not core source-of-truth semantics.

Sync is eventually consistent. Polling and webhooks are complementary and both
feed the same pipeline:

```text
source changed -> discover refs -> choose candidate refs -> fetch spec file ->
parse OpenAPI -> build index -> update revision state -> invalidate cache ->
update public routes
```

Published docs are live-ish. They render the latest known good state for the
selected public revision/ref. Public pages may expose freshness metadata such as
last sync time, source ref, and commit SHA when available.

Errors are non-destructive:

- Credential failures keep the last good render if one exists.
- Parse failures are visible to authenticated users and preserve last good public
  docs.
- Public pages avoid leaking private sync details.
- Sync history records time, trigger, ref, commit SHA when known, spec path,
  result, and an error summary.

## Publishing And Visibility

Publishing is intentional. A user marks a spec version as public before anonymous
readers can see it.

Anonymous users can navigate only public versions and public routes.
Authenticated users can see all versions they are allowed to manage.

Public serving should support path-based publishing and unique hostnames. Public
readers should be able to navigate between public versions of the same spec.
Non-git sources do not need a versioning scheme in v1; they can be a single file
source of truth.

## Storage

Manja starts without a server database, but storage is still behind ports so it
can evolve.

Ports:

- `Store`: project, source, revision, publication, theme, SEO, and sync metadata.
- `BlobStore`: raw specs and future blob artifacts.
- `SecretStore`: tokens, SSH keys, and webhook secrets.
- `Cache`: in-memory cache-aside rendered fragments and blob payloads.

Initial implementation:

- filesystem-backed `Store`
- filesystem-backed `BlobStore`
- isolated or encrypted `SecretStore`
- in-memory cache-aside

Future candidates:

- SQLite for transactions and queryability without much operational weight
- Postgres for hosted or multi-instance deployments
- Kubernetes API-server-backed adapter for CRD-style clustered state

Rendered HTML fragments, search payloads, snippet/sample fragments, and
project-theme-derived fragments should be cached at request time. Durable storage
keeps source and revision truth; cache entries are disposable.

## Indexing And Search

Each synced spec revision produces a `SpecIndex` containing:

- API identity: title, version, description, servers
- Operations: method, path, operationId, summary, description, tags, parameters,
  request body, responses, security, and deprecation state
- Schemas: component name, description, fields, and sample/example links
- Navigation groups: tags, paths, schemas, and public versions
- Search documents optimized for Ctrl+K
- Public routes and canonical URLs for published docs

Search should build on Goshtoso's command-palette search behavior:

- global Ctrl+K / Cmd+K
- keyboard navigation
- highlighted matches
- safe href handling
- capped, ranked results

Ranking should prioritize exact operationId, title, and path matches, then tags,
summaries, descriptions, schemas, and version labels. Search results should show
method badges, route paths, type labels, and version/source metadata.

The public route index should feed both Ctrl+K and `/sitemap.xml` so sitemap and
search cannot drift.

Small and medium specs may embed the search payload directly in the page. Larger
specs may fetch a search-index endpoint when needed.

## Rendering And Markdown

Public docs are Goshtoso-native, read-only, and search-first. Manja should use
Goshtoso primitives for layout, sidebar navigation, search, badges, code blocks,
tabs, tables, forms, toasts, and other UI elements whenever possible.

If Manja needs a reusable primitive that belongs in a design system, such as a
Markdown renderer component, prefer improving Goshtoso rather than creating a
Manja-only workaround. Manja-specific composition stays in Manja.

Public docs for a published spec version include:

- overview page with project SEO metadata, spec title/version, source freshness,
  servers, tags, and summary Markdown
- operation pages or anchors grouped by tag/path
- method/path badges
- parameters, request body, responses, schemas, examples, and security notes
- schema pages or sections for reusable components
- public-only version switcher for anonymous readers
- canonical URLs and sitemap entries from the public route index

Markdown is first-class. A `MarkdownRenderer` port should accept Markdown plus
context and return safe HTML plus plain text for indexing. Goldmark is the likely
initial implementation because it is pure Go, CommonMark-compatible, and
extensible.

Goshtoso theming is strict, so Markdown must target the Goshtoso token contract.
Do not depend on generic Tailwind Typography `prose` classes or arbitrary author
classes. Render Markdown inside a stable wrapper or custom renderer that maps
elements to Goshtoso tokens such as `surface`, `surface-alt`, `on-surface`,
`on-surface-strong`, `on-surface-muted`, `outline`, `primary`, and
`rounded-radius`, including dark variants.

Raw HTML should be disabled by default. Fenced code should use Goshtoso
`codeblock` behavior where feasible. Markdown tables should either use safe
token-styled table markup or be transformed into Goshtoso-flavored table markup
for richer cases.

Client-side vendored helpers may enhance read-only pages:

- `openapi-sampler` for payload/sample data
- `httpsnippet` or `@readme/httpsnippet` for request snippets

There is no v1 interactive API console. If "try it" returns later, it must be
client-side only. The Manja server must not proxy upstream API requests.

## Management UI And REST API

The HTML UI handles most authenticated workflows:

- create and configure projects
- configure sources
- manage credentials
- inspect sync status
- choose public versions
- edit theme and SEO settings
- view indexed docs as an owner

The REST management API is not a parallel product surface. Follow YAGNI: create
REST endpoints only when integration or automation use cases need them. When a
REST endpoint is needed, use an API-first workflow:

- split OpenAPI files for authoring
- Redocly CLI to join/bundle/check
- `oapi-codegen` for Go types and handler contracts

Likely REST surfaces include webhooks, source sync trigger/status, health and
readiness, and selective project/source/publication automation. UI routes do not
need OpenAPI contracts and can remain server-rendered HTML routes.

## Auth And Authorization

OAuth/OIDC is supported from the start. Dex is the local and test identity
provider.

Authorization separates:

- anonymous public readers
- authenticated project users
- project/source managers

Secret material is handled through `SecretStore`. Management routes and APIs must
not expose token or private-key material after creation.

## Testing

Use a layered test suite:

- Core unit tests for project, source, revision, publication, and index logic
- Golden-ish tests for parsing OpenAPI into search/index records
- Store, BlobStore, SecretStore, and Cache adapter tests
- REST contract tests only for APIs that actually exist
- UI tests for public docs, publishing flow, version visibility, and Ctrl+K
  search
- Auth integration tests around Dex/OIDC behavior
- Git adapter tests against local bare repositories

Use `testcontainers-go` where real services add confidence:

- Forgejo module for provider-style Git forge tests
- Dex module for OIDC tests once its Go module release status is verified

GitHub and GitLab webhook parsing can start as fixture tests unless Manja later
adds provider-specific API behavior.

## Initial Non-Goals

- No server-side proxy for upstream API calls.
- No v1 "try it" console.
- No Postgres requirement.
- No speculative REST API surface.
- No non-git versioning scheme beyond a singleton file source.
- No embedding a full external OpenAPI renderer that fights Goshtoso.

## Open Questions

- Which `httpsnippet` package variant should be vendored.
- Whether Markdown should live first in Manja or be introduced directly as a
  Goshtoso component.
- Exact filesystem layout for Store, BlobStore, and SecretStore.
- Exact public URL model for path-based publishing vs unique hostnames.
- Which OpenAPI parser/model package should back `SpecIndex`.
