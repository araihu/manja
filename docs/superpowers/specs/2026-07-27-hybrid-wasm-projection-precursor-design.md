# Hybrid Wasm Projection-Only Precursor Design

**Status:** Review-ready precursor design; no implementation exists

**Date:** 2026-07-27

**Docs authoring base:** `6b5e7a2cbb604d1da0f1cee9da29d6c1f5fbe3c5`

**Later control-plane observation:** `origin/main` reached
`539f6504e2f72e3a7fa470fa600dbfa0e3179966` through a disjoint canonical-SVG
slice while these docs remained on their approved authoring base. That SHA is
not silently incorporated here and is not frozen as the future implementation
base; the implementation plan fetches and records then-current `origin/main`.

**Parent design:**
[Hybrid Wasm Public Docs Design](./2026-07-27-hybrid-wasm-public-docs-design.md)

## Decision

Implement the parent design's explicit projection-only exception as an unused,
provider-neutral application projection plus an internal canonical JSON codec.
The application package consumes only `domain.SpecIndex` as product data. It
does not parse OpenAPI, resolve publications, render HTML, route HTTP, load
assets, access storage, or call the network.

The boundary is:

```text
domain.SpecIndex
        |
        v
application/projection        public deterministic builder + DTO
        |
        v
internal/adapters/projectionjson   strict canonical bytes + SHA-256
        |
        +-- future internal/web projection endpoint
        +-- future in-module Wasm command
```

This precursor ends at deterministic bytes and digest. It exposes no endpoint
and is not referenced by the public docs page. Completion means the contract is
ready for later nodes, not that hybrid public docs exist.

## Why This Package Boundary

`application/projection` is a public subpackage because the approved design
calls the builder a provider-neutral application service, `AGENTS.md` keeps
reusable orchestration in public `application`, and both future server and Wasm
entry points need one implementation. Its proposed API is deliberately small:

```go
type Builder struct{}

func (Builder) Build(context.Context, domain.SpecIndex) (Document, error)
```

The context is the first parameter and permits cancellation between bounded
record builds. `Builder` has no infrastructure dependency and does not replace
the incoming context.

The exported DTO becomes a semantic-versioned Go contract. An unrelated module
must be able to build and inspect it with `GOWORK=off`. That is a conscious cost:
moving the builder under `internal` would conflict with the approved
provider-neutral application-service decision and the repository's extension
boundary. The canonical JSON codec remains under
`internal/adapters/projectionjson`, as required by the parent design's decision
that serialization and browser-runtime adapters stay internal. External Go
consumers do not receive Manja's codec or an implied storage/runtime API.

`internal/adapters/projectionjson` may directly import only the standard
library and `application/projection`; `domain` is allowed only as the latter's
transitive dependency. It must not directly or transitively import the parser,
`internal/web`, Goshtoso, source adapters, stores, or a Wasm command.

## Scope

### Included

- version-1 projection DTOs built from `domain.SpecIndex`;
- deterministic source-order capture, stable sorting, and explicit ordinals;
- canonical anchor, relative href, heading, and landmark metadata;
- bounded detail-scoped schema/example/code-sample inputs;
- strict canonical JSON encoding and decoding;
- exact-byte lowercase SHA-256 digests;
- golden vectors, malformed corpora, property tests, fuzz targets, dependency
  gates, unrelated-module proof, and WebAssembly compilation proof.

### Deferred Without Stubs

- Goshtoso components, CSS, JavaScript, asset manifests, or fallback assets;
- publication visibility, eligibility, release tracks, authenticated previews,
  private-publication behavior, or publication cache keys;
- HTTP handlers, content negotiation, ETag headers, projection routes, sidebar
  fragment routes, or network allowlists;
- server or Wasm HTML rendering, templ, Markdown rendering, lazy UI, search UI,
  fragment size enforcement, or parity screenshots;
- Wasm command/globals, Go runtime packaging, Service Worker, CacheStorage,
  IndexedDB, offline shell, original-spec cache, freshness, rollback, or LRU;
- `MANJA_LOCAL_DOCS`, descriptors, default-on rollout, kill-switch behavior, or
  withdrawal tombstones;
- the parent design's complete performance receipt and release gates.

No field in the version-1 DTO represents deferred state. In particular, there
is no `eligible`, `public`, `private`, `cacheKey`, `releaseTrack`, `assetURL`,
`workerURL`, `wasmURL`, `offlineReady`, or network-route field.

## Source Contract

`Builder.Build` accepts a `domain.SpecIndex` value, copies it, and zeroes
`SpecDownload.JSON` and `ExampleSpecJSON` in that validation copy. Before
building records, it makes these exact identity-validation calls:

```go
domain.ValidateCanonicalIdentity("projection project ID", validationCopy.ProjectID, false)
domain.ValidateCanonicalIdentity("projection revision ID", validationCopy.RevisionID, false)
```

It then calls `domain.ValidateSpecIndex` on the same sanitized copy. This
preserves current domain validation for included branches without letting
excluded bytes affect success, failure, output, or digest. With
`allowEmpty=false`, both identities must be non-empty valid UTF-8, have no
leading/trailing whitespace, and contain no Unicode control character. Internal
whitespace and non-ASCII non-control text accepted by the domain API remain
accepted; the builder adds no normalization or narrower identity grammar.
Failure is never synthesized into another identity.

The builder reads these `SpecIndex` branches:

- `ProjectID`, `RevisionID`, `Title`, `Version`;
- `Branding` and `Overview`;
- `Operations`, `Schemas`, `Search`, and `PublicRoutes`;
- operation/schema detail fields nested below those slices.

The builder does not read:

- `SpecDownload.JSON`;
- `ExampleSpecJSON`;
- a `SpecFile`, parser, source, revision resolver, publication, actor, secret,
  release record, HTTP request, or private adapter value.

`SpecDownload.Filename` is retained as overview display metadata under
`specDownloadFilename`; its bytes are not the original spec and it does not
grant cache or publication eligibility.

### Literal Domain-to-Wire Mapping

| `domain` source | Version-1 disposition |
| --- | --- |
| `SpecIndex.ProjectID`, `RevisionID`, `Title`, `Version` | Top-level identity/title/API version; project/revision required |
| `DocsBranding.DisplayName`, `Logo.Src`, `Logo.Alt`, `Logo.HomeURL`, `Favicon` | `Branding` scalars |
| `SpecOverview.Description`, `TermsOfService` | `Overview` scalars |
| `SpecContact.Name`, `URL`, `Email` | `Contact` |
| `SpecLicense.Name`, `URL`, `Identifier` | `License` |
| `SpecServer.URL`, `Description`, `Variables` | Ordinal `Server` records |
| `SpecServerVariable.Name`, `Default`, `Description`, `Enum` | Ordinal `ServerVariable`; enum strings become `TextRecord` |
| `SpecDownload.JSON` | Excluded before validation/build/digest |
| `SpecDownload.Filename` | `Overview.specDownloadFilename` only |
| `Operation.ID`, `Anchor`, `Method`, `Path`, `Summary`, `Description`, `Deprecated` | Operation directory/detail and canonical navigation metadata |
| `Operation.Tags` | Ordinal `TextRecord` tags plus operation-tag sidebar membership |
| `OperationParameter.Name`, `In`, `Required`, `Description`, `Schema`, `Example` | Ordinal `Parameter`, recursive `WireSchema`, and an `Example` record whose text is copied exactly when non-empty |
| `OperationRequestBody.Description`, `Required`, `MediaTypes` | Explicit `hasRequestBody` plus concrete `RequestBody` |
| `OperationResponse.Status`, `Description`, `MediaTypes` | Ordinal `Response` |
| `OperationMediaType.ContentType`, `Schema`, `Example`, `ExampleProvided` | Ordinal `MediaType`, recursive `WireSchema`, and one text `Example` when `ExampleProvided` is true |
| `OperationSecurity.Name`, `Scopes` | Ordinal `SecurityRequirement`; scopes become `TextRecord` |
| `RequestSnippet.Label`, `Language`, `Code` | Ordinal `CodeSample` |
| `SchemaSummary.Name`, `Type`, `Format`, `Description`, `Default`, `Example`, `JSON` | `WireSchema` scalar display strings plus canonical embedded `json`; no type is inferred from `Default` or `Example` |
| `SchemaSummary.Properties` | Ordinal recursive `SchemaProperty` |
| `SchemaSummary.Items` | Zero-or-one recursive `WireSchema.items` slice |
| `SchemaProperty.Name`, `Required`, `Schema`, `Description` | `SchemaProperty`; explicit property description retained even when different from nested schema description |
| `Schema.Name`, `Description`, `Summary` | Schema directory/detail and recursive `WireSchema` |
| `Schema.Example.JSON` | Canonical `SchemaDetail.exampleSchemaJSON`; this is schema input, not the explicit example value |
| `Schema.Example.Example`, `Provided` | One text `Example` record when `Provided` is true; preserved independently from `Schema.Example.JSON` |
| `SearchDocument.ID`, `Title`, `Description`, `Href`, `Kind`, `Method`, `Path`, `Section`, `Keywords` | Validated `SearchRecord`; href rebuilt; `Kind` canonicalized to lowercase enum; keywords become `TextRecord` |
| `PublicRoute.Path`, `Title`, `Description` | Sorted `PublicRoute` |
| `ExampleSpecJSON` | Excluded before validation/build/digest; never used as fallback |

`ProjectID` and `RevisionID` are the only identity available from
`SpecIndex`. Publication cache keys, anonymous visibility, resolver evidence,
and release identity cannot be inferred and therefore do not appear.

## Version-1 Wire Model

All wire objects are concrete structs. All collections are slices. There are no
map fields, `any`, `interface{}`, `json.RawMessage`, floating-point fields, or
`omitempty` tags. Empty collections encode as `[]`, never `null`. Optional
objects use an explicit `has...` boolean plus a zero-valued concrete struct.

Every slice element has a non-negative `uint32` `ordinal` and a stable `id`
except `publicRoutes`, whose stable key is `path + "\x00" + title`. Scalar string
lists are represented by `TextRecord {ordinal,id,value}` rather than raw string
slices. This keeps source position explicit and gives the decoder a key to
validate.

Struct declaration order fixes JSON object field order. The required field order
is:

| DTO | JSON fields in declaration order |
| --- | --- |
| `Document` | `formatVersion`, `projectId`, `revisionId`, `title`, `apiVersion`, `branding`, `overview`, `mainLandmark`, `operationGroupHeading`, `schemaGroupHeading`, `sidebarSections`, `operations`, `operationDetails`, `schemas`, `schemaDetails`, `search`, `publicRoutes` |
| `Branding` | `displayName`, `logoSrc`, `logoAlt`, `logoHomeHref`, `faviconHref` |
| `Overview` | `anchor`, `href`, `headingId`, `heading`, `headingLevel`, `description`, `termsOfService`, `specDownloadFilename`, `contact`, `license`, `servers` |
| `Contact` | `name`, `url`, `email` |
| `License` | `name`, `url`, `identifier` |
| `Landmark` | `id`, `role` |
| `Heading` | `id`, `text`, `level` |
| `Server` | `ordinal`, `id`, `url`, `description`, `variables` |
| `ServerVariable` | `ordinal`, `id`, `name`, `default`, `description`, `enum` |
| `TextRecord` | `ordinal`, `id`, `value` |
| `SidebarSection` | `ordinal`, `id`, `kind`, `title`, `href`, `items` |
| `SidebarItem` | `ordinal`, `id`, `anchor`, `href`, `label`, `method` |
| `OperationDirectory` | `ordinal`, `id`, `anchor`, `href`, `method`, `path`, `title`, `deprecated`, `sections` |
| `OperationDetail` | `ordinal`, `id`, `anchor`, `href`, `headingId`, `heading`, `headingLevel`, `method`, `path`, `summary`, `description`, `deprecated`, `tags`, `parameters`, `hasRequestBody`, `requestBody`, `responses`, `security`, `codeSamples` |
| `Parameter` | `ordinal`, `id`, `name`, `in`, `required`, `description`, `schema`, `examples` |
| `RequestBody` | `description`, `required`, `mediaTypes` |
| `Response` | `ordinal`, `id`, `status`, `description`, `mediaTypes` |
| `MediaType` | `ordinal`, `id`, `contentType`, `schema`, `examples` |
| `SecurityRequirement` | `ordinal`, `id`, `name`, `scopes` |
| `CodeSample` | `ordinal`, `id`, `label`, `language`, `code` |
| `WireSchema` | `name`, `type`, `format`, `description`, `defaultValue`, `exampleText`, `json`, `properties`, `items` |
| `SchemaProperty` | `ordinal`, `id`, `name`, `required`, `description`, `schema` |
| `SchemaItem` | `ordinal`, `id`, `schema` |
| `Example` | `ordinal`, `id`, `text`, `provided` |
| `SchemaDirectory` | `ordinal`, `id`, `anchor`, `href`, `name`, `title`, `description` |
| `SchemaDetail` | `ordinal`, `id`, `anchor`, `href`, `headingId`, `heading`, `headingLevel`, `description`, `schema`, `exampleSchemaJSON`, `examples` |
| `SearchRecord` | `ordinal`, `id`, `resultId`, `title`, `description`, `href`, `kind`, `method`, `path`, `section`, `keywords` |
| `PublicRoute` | `ordinal`, `path`, `title`, `description` |

`WireSchema.items` is a `[]SchemaItem` of length zero or one. Its sole valid
record has `ordinal=0` and `id=items`; a decoded length greater than one is
invalid. This replaces `domain.SchemaSummary.Items`' pointer without introducing
a nullable wire object.

### Stable Record Keys

Readable canonical identities remain readable: operation/schema directory and
detail `id` equals `anchor`; parameter ID is `parameter-` plus the hash helper
below over lowercased `in` and exact `name`; response ID is exact `status`;
media-type ID is exact `contentType`; security ID is exact `name`; property ID is
exact `name`; schema-item ID is `items`; the single available example ID is
`primary`; search ID is exact source `SearchDocument.ID`; sidebar item ID is its
target anchor; public-route key is `path + "\x00" + title`.

Records without a concise collision-safe natural key use:

```text
recordID(kind, parts...) = kind + "-" + lowercaseHex(
    SHA256(kind || 0x00 || uint64be(len(part0)) || part0 || ...)
)
```

Lengths count UTF-8 bytes. This helper keys servers by URL, server variables by
name within their server, `TextRecord` by value within its explicit context
kind, code samples by language plus label, and operation-tag sections by exact tag. The
operation-tag prefix is therefore `operation-tag-` followed by the full
64-character digest. For scalar membership lists, the builder first removes
exact duplicates while preserving the first source occurrence: tags and
keywords also drop values for which `strings.TrimSpace(value)` is empty; server
enums retain an exact empty value. Scopes are deduplicated but never drop an
empty value because `domain.ValidateSpecIndex` rejects blank scope identities
before projection. The retained source order is then compacted to ordinals
`0..n-1`. Repeated tags therefore create one membership and one section. Every
duplicate stable key remaining in a built or decoded parent is rejected;
builders do not append ordinals to conceal duplicates.

Text context kinds are exact: `server-enum`, `tag`, `scope`, `keyword`, and
`operation-section`. `OperationDirectory.sections` is `[]TextRecord` whose
values are operation-tag section IDs. This avoids a recursive sidebar model.

### Detail-Scoped JSON and Display Strings

Only `SchemaSummary.JSON` and `Schema.Example.JSON` are treated as embedded
JSON. They become `WireSchema.json` and `SchemaDetail.exampleSchemaJSON`,
respectively, and remain bounded canonical JSON text encoded as a JSON string;
they are never inserted as objects into the projection wire. Before use, the
builder:

1. requires valid UTF-8;
2. runs duplicate-key, single-value, depth, and number validation;
3. decodes with `UseNumber`;
4. recursively emits objects with lexicographically sorted keys, arrays in
   source order, Go `encoding/json.Marshal` string escaping, lowercase literals,
   and normalized numbers;
5. stores the resulting bytes as a string.

Embedded JSON numbers must first match the JSON number grammar
`-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?`. They are normalized without a
floating-point conversion: expand the exponent to plain decimal, remove
insignificant leading integer and trailing fractional zeroes, and normalize
every zero spelling, including `-0`, to `0`. Exact vectors are `1e+03` to
`1000`, `1.0` to `1`, `-0` to `0`, and `1e-3` to `0.001`. Reject a number when
expansion would exceed its enclosing record bound; never allocate from an
unbounded exponent. `json.Decoder.UseNumber` preserves the source token for this
custom emitter; `encoding/json.Marshal` alone is not the number normalizer.

By contrast, `SchemaSummary.Default`, `SchemaSummary.Example`,
`OperationParameter.Example`, `OperationMediaType.Example`, and
`SchemaExample.Example` are display text. They are copied byte-for-byte after
UTF-8 validation and are never interpreted as JSON, Boolean, or number. Thus
the text `true` remains the four-character string `true`; the precursor does
not invent type information absent from `domain.SpecIndex`. A parameter example
is present only when non-empty. Media-type and schema examples use their
explicit `Provided` boolean, so a provided empty string remains one
`Example{text:"",provided:true}`. `ExampleSpecJSON` is never a fallback.

## Canonical Anchors, Hrefs, Headings, and Landmarks

These values are presentation metadata, not HTML.

- overview anchor: `overview`;
- overview href: `?selected=overview#overview`;
- main landmark: `id=main-content`, `role=main`;
- overview heading: `headingId=overview-heading`, text `SpecIndex.Title`, level `2`;
- operation group heading: `Heading{id=operations-heading,text=Operations,level=2}`;
- schema group heading: `Heading{id=schemas-heading,text=Schemas,level=2}`.

Operation anchor selection is exact:

1. use non-empty `Operation.Anchor` after requiring it already has no padding
   and matches `^[a-z0-9][a-z0-9._~/-]*$`;
2. otherwise slug `Operation.ID`;
3. otherwise slug `strings.ToUpper(Method) + " " + Path`;
4. prefix the non-empty result with `operation-` when a fallback was used;
5. reject an empty result or any duplicate final anchor.

The supplied-anchor alphabet deliberately allows the slash already present in
current canonical fixtures but rejects percent escapes, `#`, spaces, Unicode,
backslashes, and controls. This makes the literal fragment an unambiguous future
DOM target. Exact vectors accept `operation-list/pets` and reject
`operation-list%2Fpets`, `operation#list`, `operation list`, and
`opération-list`.

The slug algorithm lowercases ASCII letters, retains ASCII digits, replaces one
or more other runes with one `-`, and trims leading/trailing `-`. Supplied
anchors are preserved, not re-slugged, because they are current domain output.

Schema anchors are `schema-` plus that same slug of `Schema.Name`. Empty or
duplicate results are rejected. This deliberately replaces the duplicated raw
lowercase fallback found in current server/POC presentation helpers; later
server and Wasm renderers must consume this projection value rather than
recompute it.

Every selected-item href is:

```text
?selected=<url.QueryEscape(anchor)>#<anchor>
```

Anchors produced by the fallback slug are ASCII URL-safe. A supplied operation
anchor may contain URL-significant characters; its query value is escaped while
its fragment remains the literal validated anchor, matching the current
canonical selected-route contract. The href is publication-base relative. A
later HTTP route joiner owns the publication base and root-path special case.

Operation heading text is first non-empty of `Summary`, `ID`, and
`strings.TrimSpace(Method + " " + Path)`. Its detail section landmark is the
operation anchor, heading ID is the same anchor, and heading level is `3`.
Schema heading and landmark use the schema name/anchor with level `3`.

Search `resultId` is `recordID("search-result", sourceID)`. It must differ from
every visible content anchor. Search hrefs are rebuilt from the referenced
anchor, not copied blindly. Every search record must resolve to overview, one
operation, or one schema; unresolved or duplicate search/result IDs fail the
build.

The builder maintains one global DOM-target namespace seeded with
`main-content`, `overview`, `overview-heading`, `operations-heading`,
`schemas-heading`, and the schema sidebar ID `schemas`. It then admits every
operation/schema anchor, operation-tag section ID, and search `resultId` only
once. Record IDs that are aliases rather than rendered targets—such as a
sidebar item's target-anchor ID—may repeat their target by design. Exact tests
reject an operation anchor colliding with `overview`, any fixed heading or
landmark, a schema anchor, a section ID, or a generated `search-result-...` ID.

Search target extraction accepts either exact `#<source-target>` or a
same-document relative reference whose fragment is non-empty and whose optional
`selected` query value equals that fragment. Absolute URLs, scheme/host values,
missing fragments, conflicting `selected` values, and unknown targets fail.
The emitted href always uses the canonical relative selected-item form above.

For compatibility with frozen `domain.SpecIndex` production, the builder
registers exactly one legacy alias per schema:

```text
legacySchemaAnchor(name) = "schema-" + strings.ToLower(name)
```

This mirrors the frozen parser's search/public-route helper only at the input
boundary. An alias must resolve uniquely and must not collide with another
canonical anchor/alias. Both canonical and exact legacy targets map to the new
ASCII-slug schema anchor before output. No other fuzzy, decoded, or case-folded
alias is accepted. Exact coverage uses schema `author_association`: source
`#schema-author_association` and
`/?selected=schema-author_association#schema-author_association` both emit
`?selected=schema-author-association#schema-author-association` (with leading
`/` retained for the public route).

Search kind is `strings.ToLower(strings.TrimSpace(source.Kind))` and must be
exactly `overview`, `operation`, or `schema`. It must agree with the resolved
anchor type: only `overview` targets `overview`, `operation` targets an
operation anchor, and `schema` targets a schema anchor. Current parser values
`Overview`, `Operation`, and `Schema` therefore have one stable wire spelling;
unknown or mismatched kinds fail.

Each copied `PublicRoute.Path` is parsed with `url.Parse`, because current
canonical paths include a fragment and `url.ParseRequestURI` does not split that
form correctly. It must have an absolute, `path.Clean` path beginning `/`, no
backslash, scheme, host, userinfo, control, or padding. A non-root route must
carry both a `selected` query and fragment; both must agree with one known
canonical anchor or exact legacy schema alias. Extra query keys are rejected.
The overview route `/` remains valid. A selected route is rebuilt from its
validated clean path plus the canonical resolved anchor; legacy target spelling
is never copied to output.
Exact negative vectors cover conflicting query/fragment, absolute and
scheme-relative URLs, traversal/unclean paths, backslashes, extra query keys,
and missing halves. The precursor preserves the validated path because
publication-base joining belongs to the later HTTP node.

Operation-tag sidebar section IDs use `recordID("operation-tag", exactTag)`.
The title preserves the tag. `Untagged` is used for missing/blank tags. Their
href is `?section=<url.QueryEscape(sectionID)>`; items are the canonically
ordered operations carrying that tag. When schemas exist, one section uses
`id=schemas`, `kind=schemas`, `title=Schemas`, `href=?section=schemas`, and
canonically ordered schema items. Overview is already a top-level projection
record and is not duplicated as a sidebar section. Hash-based tag IDs avoid
collisions between different labels whose human-readable slugs would be
identical.

## Ordinals and Stable Ordering

Ordinals are zero-based `uint32`; count overflow fails before conversion.
Top-level directories/details/search/routes and sidebar sections derive ordinal
from their canonical sorted position, so equivalent top-level permutations are
byte-identical. The builder first canonicalizes operations by anchor and schemas
by anchor. Operation-tag sidebar items derive their ordinals from that canonical
operation order, and schema sidebar items derive theirs from canonical schema
order. Each `OperationDirectory.sections` list instead follows that operation's
deduplicated retained tag order with compact ordinals `0..n-1`; a single
operation can belong to several sections. Other nested presentation records
capture their input ordinal before sorting.

Canonical encoded order is:

- sidebar sections by kind rank (`operation-tag`, `schemas`), then
  section ID;
- operation directories and details by anchor;
- schema directories and details by anchor;
- search records by stable source ID;
- public routes by path, then title;
- parameters, responses, media types, properties, examples, code samples,
  servers, variables, enum values, tags, scopes, keywords, and sidebar items by
  ordinal, then ID.

Nested original source position remains in `ordinal` even where stable encoding
uses ordinal then ID. Sorting never mutates the caller's `SpecIndex`; all
strings, slices, and recursive schema values are copied. Equivalent inputs that
differ only in top-level slice order produce the same bytes. Changing nested
presentation order is a semantic change and therefore changes ordinals/digest.

## Canonical Encoding and Digest

`projectionjson.Marshal` performs projection validation and then calls
`encoding/json.Marshal` exactly once for the top-level DTO.

- input strings must be valid UTF-8; Go's replacement behavior is forbidden;
- standard Go JSON escaping is required, including HTML-sensitive characters
  and U+2028/U+2029;
- integer tokens are unsigned base-10 with no leading zero;
- output is compact, contains no insignificant whitespace, and has no trailing
  newline;
- source CR/LF characters inside strings encode as `\r`/`\n`; canonical output
  contains no literal CR or LF byte anywhere;
- empty slices are `[]`;
- bytes are not content-coded before hashing.

`projectionjson.Unmarshal` validates bytes before returning a DTO:

1. reject more than 16 MiB before allocation-heavy decode;
2. reject invalid UTF-8;
3. use a `json.Decoder` token pass with `UseNumber` and an object stack to reject
   duplicate keys at every depth, non-integer wire numbers, invalid integer
   spellings, out-of-range integers, non-object top level, and trailing values;
4. decode into the concrete DTO with `DisallowUnknownFields`;
5. validate version, identities, non-nil slices, ordinals, record-key
   uniqueness, href/anchor/heading relationships, embedded JSON strings, and
   per-record/total bounds; validate every decoded `WireSchema` iteratively with
   depth at most 64 and aggregate nodes at most 100,000 before recursive use;
6. marshal the decoded DTO and require byte equality with the input.

Step 6 rejects reordered fields, alternate escaping, whitespace, newlines,
`null` collections, and other non-canonical but semantically equivalent JSON.

`projectionjson.Digest` computes SHA-256 over these exact uncompressed bytes and
returns 64 lowercase hexadecimal characters. It never hashes a struct, raw
OpenAPI, compressed response, ETag spelling, or decoded/re-encoded browser
value. HTTP `Content-Type` and ETag formatting belong to the later endpoint
node.

## Bounds

All limits are inclusive:

| Subject | Maximum canonical encoded bytes |
| --- | ---: |
| one `OperationDetail` | 262,144 (256 KiB) |
| one `SchemaDetail` | 524,288 (512 KiB) |
| complete `Document` | 16,777,216 (16 MiB) |

The internal codec checks record bounds before returning canonical bytes and
again after decode. The public builder does not import the internal codec or
promise that every in-memory DTO is encodable; callers must use the codec at the
server/Wasm composition boundary. One byte over fails. A record that fits its
own limit may still fail the total limit. The parent design's 2 MiB
rendered-fragment and 64 MiB original-spec limits are deferred because this
precursor renders no HTML and caches no original spec.

`domain.ValidateSpecIndex` remains the source depth/node gate: maximum schema
summary depth 64 and aggregate node budget 100,000. The decoder independently
applies the same 64/100,000 limits to wire schemas because decoded bytes did not
pass through domain validation; exact cases accept depth 64/node 100,000 and
reject depth 65/node 100,001 without panic. Embedded JSON validation uses depth
64 and the enclosing detail byte limit. No unbounded recursion is introduced.

## Frozen Vectors

### Vector A: Canonical Escaping and Empty Slices

The exact 872 bytes below have no trailing newline:

```json
{"formatVersion":1,"projectId":"payments","revisionId":"rev-0001","title":"Payments \u003cAPI\u003e\u2028","apiVersion":"1.2.3","branding":{"displayName":"","logoSrc":"","logoAlt":"","logoHomeHref":"","faviconHref":""},"overview":{"anchor":"overview","href":"?selected=overview#overview","headingId":"overview-heading","heading":"Payments \u003cAPI\u003e\u2028","headingLevel":2,"description":"","termsOfService":"","specDownloadFilename":"","contact":{"name":"","url":"","email":""},"license":{"name":"","url":"","identifier":""},"servers":[]},"mainLandmark":{"id":"main-content","role":"main"},"operationGroupHeading":{"id":"operations-heading","text":"Operations","level":2},"schemaGroupHeading":{"id":"schemas-heading","text":"Schemas","level":2},"sidebarSections":[],"operations":[],"operationDetails":[],"schemas":[],"schemaDetails":[],"search":[],"publicRoutes":[]}
```

Because the Markdown code block itself has a line terminator, the fixture file
must be written without that final byte. Its SHA-256 is
`8267e1a8a597a6561409e81492b06c24b44b6cbd12875fc90985295c5765889d` and must
be checked in as `application/projection/testdata/v1-empty.sha256`; the
implementation plan forbids accepting a generated digest without reviewing
`wc -c` and a hex dump.

### Vector B: One-Operation Independent Golden

The exact 2,780 bytes below have no trailing newline. This is a second
independently reviewable oracle rather than output that a test accepts from the
implementation under test:

```json
{"formatVersion":1,"projectId":"pets","revisionId":"rev-0002","title":"Petstore","apiVersion":"1.0.0","branding":{"displayName":"","logoSrc":"","logoAlt":"","logoHomeHref":"","faviconHref":""},"overview":{"anchor":"overview","href":"?selected=overview#overview","headingId":"overview-heading","heading":"Petstore","headingLevel":2,"description":"","termsOfService":"","specDownloadFilename":"","contact":{"name":"","url":"","email":""},"license":{"name":"","url":"","identifier":""},"servers":[]},"mainLandmark":{"id":"main-content","role":"main"},"operationGroupHeading":{"id":"operations-heading","text":"Operations","level":2},"schemaGroupHeading":{"id":"schemas-heading","text":"Schemas","level":2},"sidebarSections":[{"ordinal":0,"id":"operation-tag-a6bcc99598729134578da5115a847a2a039d17052571625d3b51bc59a1dadc50","kind":"operation-tag","title":"Pets","href":"?section=operation-tag-a6bcc99598729134578da5115a847a2a039d17052571625d3b51bc59a1dadc50","items":[{"ordinal":0,"id":"operation-list/pets","anchor":"operation-list/pets","href":"?selected=operation-list%2Fpets#operation-list/pets","label":"List pets","method":"GET"}]}],"operations":[{"ordinal":0,"id":"operation-list/pets","anchor":"operation-list/pets","href":"?selected=operation-list%2Fpets#operation-list/pets","method":"GET","path":"/pets","title":"List pets","deprecated":false,"sections":[{"ordinal":0,"id":"operation-section-8ea50d30e07543f5c6bbf699f0e76f617efab63774e9c7de76deca0633cd14a0","value":"operation-tag-a6bcc99598729134578da5115a847a2a039d17052571625d3b51bc59a1dadc50"}]}],"operationDetails":[{"ordinal":0,"id":"operation-list/pets","anchor":"operation-list/pets","href":"?selected=operation-list%2Fpets#operation-list/pets","headingId":"operation-list/pets","heading":"List pets","headingLevel":3,"method":"GET","path":"/pets","summary":"List pets","description":"","deprecated":false,"tags":[{"ordinal":0,"id":"tag-fe22122df5f8647a0015238eb1093ba567eb2d9c001ff63a1b82f9285db90752","value":"Pets"}],"parameters":[],"hasRequestBody":false,"requestBody":{"description":"","required":false,"mediaTypes":[]},"responses":[],"security":[],"codeSamples":[]}],"schemas":[],"schemaDetails":[],"search":[{"ordinal":0,"id":"op-list","resultId":"search-result-1e46984659c11bcb062b8421268996f857f71a88283c62ea0a79d7f47ec4c4b1","title":"List pets","description":"","href":"?selected=operation-list%2Fpets#operation-list/pets","kind":"operation","method":"GET","path":"/pets","section":"Pets","keywords":[{"ordinal":0,"id":"keyword-2a1efd480526a59f37e96ad6fc2af6f79e277f5b41f2eb6c5b676fd1d5e4f3ad","value":"pets"}]}],"publicRoutes":[{"ordinal":0,"path":"/","title":"Petstore","description":""},{"ordinal":1,"path":"/?selected=operation-list%2Fpets#operation-list/pets","title":"List pets","description":""}]}
```

Its SHA-256 is
`6609c4e78e6556c8a178e500aeff8da85801ce30aaa784129c85b2c4e63cdc41`.
The future fixture names are `v1-operation.json` and
`v1-operation.sha256`. The literal IDs, escaped selected route, byte count, and
digest are reviewed requirements; regeneration may only propose a diff.

Its direct `domain.SpecIndex` input is exactly identity `pets`/`rev-0002`, title
`Petstore`, version `1.0.0`; one operation with ID `listPets`, supplied anchor
`operation-list/pets`, `GET /pets`, summary `List pets`, and tag `Pets`; one
search record with ID `op-list`, kind `Operation`, href
`#operation-list/pets`, section `Pets`, and keyword `pets`; route `/` titled
`Petstore`; and route
`/?selected=operation-list%2Fpets#operation-list/pets` titled `List pets`, both
with empty descriptions. Every other source field/slice is its explicit
zero/empty value.

### Vector C: Full Contract Fixture

`v1-full` is built directly as `domain.SpecIndex`, not by an OpenAPI parser. It
contains:

- identity `payments` / `rev-0001` and title `Payments <API>\u2028`;
- two operations supplied in reverse anchor order: `operation-create-pet`
  (`POST /pets`) and `operation-list-pets` (`GET /pets`);
- tags `Pets`, `Admin`, and a repeated `Pets` membership;
- the create-operation directory sections retain `Pets` ordinal 0 and `Admin`
  ordinal 1 after the repeated `Pets` is collapsed, while each tag section's
  items follow canonical operation order;
- deliberately unsorted parameters, responses, media types, security scopes,
  and cURL/JavaScript code samples;
- schemas `Pet` and `Error`, recursive properties/items without cycles;
- embedded schema JSON containing `<script>`, U+2028/U+2029, a newline,
  integer boundaries, and numeric tokens `1e+03`, `1.0`, `-0`, and `1e-3`;
- paired display examples `true`, `1e+03`, and `{"looks":"json"}` that remain
  exact text, plus `Schema.Example.JSON` and `Schema.Example.Example` set to
  distinct sentinels to prove neither payload overwrites the other;
- search records in reverse order and public routes with equal paths/different
  titles to exercise tie-breaking;
- non-empty branding, overview contact/license, two servers, and variables.

Its reviewed output files are:

```text
application/projection/testdata/v1-full.json
application/projection/testdata/v1-full.sha256
```

Tests compare bytes, byte count, and digest literally. A guarded
`MANJA_UPDATE_PROJECTION_GOLDEN=1` path may create candidate files, but ordinary
tests never rewrite them.

### Vector D: Excluded Sentinels

Two otherwise identical `SpecIndex` inputs differ only in:

```text
SpecDownload.JSON = __MANJA_SPEC_DOWNLOAD_SENTINEL_7d67d7e4__
ExampleSpecJSON   = __MANJA_EXAMPLE_SPEC_SENTINEL_12eb9dc1__
```

Neither sentinel may occur in `v1-full.json`. Mutating either excluded field,
including to invalid UTF-8, must leave build success, complete bytes, and digest
unchanged. A filename-only mutation must change bytes/digest, proving the test
does not accidentally ignore the whole `SpecDownload` struct.

The builder signature itself proves parser/source/private values are absent:
the package has no type parameter or import through which they can enter.

### Scale Vector

The existing GitHub REST fixture remains a non-golden scale input at
`internal/adapters/openapi/testdata/github-v3-rest.json`, SHA-256
`dedfee9ad6a676c2f7186b8e2137d887d6449cad8b7af8253aecdaae24b27977`.
An adapter may parse it before calling the builder in tests. The builder never
imports that adapter. Because the current parser fills `RevisionID` but not
`ProjectID`, the test fixture composition sets `ProjectID="github"` after parse
and supplies fixed revision `github-v3-rest-fixture`; production code must not
infer either identity. Every operation/schema detail must meet its 256/512 KiB
record bound and the resulting projection must be at most 16 MiB; its
bytes/digest are measurement evidence, not a stable golden across parser
versions.

## Required Tests

### Unit and Property Tests

- exact vectors A-D, repeat builds, caller-input immutability, and 1,000 seeded
  top-level permutations;
- nested ordinals retained while canonical record sorting remains stable;
- anchor/href/heading/landmark fixtures for overview; supplied anchor
  `operation-list/pets` with href
  `?selected=operation-list%2Fpets#operation-list/pets`; method/path fallback
  `operation-post-pets-pet-id`; schema input `Pet <Profile>` with authoritative
  fallback `schema-pet-profile`; supplied `%`, `#`, space, and Unicode anchors
  rejected; and search-result ID separation;
- duplicate final anchors, record keys, routes, search IDs, and nested IDs fail;
- invalid UTF-8 in every included source branch fails before `json.Marshal`;
  invalid UTF-8 in the two excluded byte fields does not affect output;
- display examples/defaults remain exact text; embedded JSON numbers normalize
  to the four exact decimal vectors without any float wire field;
- operation/schema record and total sizes just below, exactly at, and one byte
  above their inclusive limits;
- decoded wire-schema depth 64/node 100,000 accepted and depth 65/node 100,001
  rejected without recursive overflow;
- cancellation before build and between records returns `context.Canceled`
  without partial output.

### Strict Decode Corpus

Checked-in or table-generated cases cover duplicate keys at top level and every
nested object, unknown fields at every DTO layer, trailing JSON, invalid UTF-8,
negative/float/exponent/leading-zero/out-of-range ordinals, wrong format
version, `null` slices, missing required identity, unsorted records, duplicate
keys, wrong href, wrong heading/landmark, malformed embedded JSON, non-canonical
whitespace, alternate escapes, a trailing newline, and bounded error
non-disclosure for unknown-key, identity, example, and malformed-byte sentinels.

### Fuzz Targets

- `FuzzUnmarshalCanonicalProjection`: seed all goldens and malformed cases; no
  panic; every accepted input re-marshals byte-for-byte identically;
- `FuzzBuildDeterminism`: derive bounded permutations from seed bytes and require
  stable output/digest for equivalent top-level records;
- `FuzzEmbeddedJSON`: no panic, duplicate keys rejected, accepted strings
  canonical and within depth/record bounds.

### Architecture and Consumer Tests

- `application/projection` directly imports only standard library and `domain`;
- `internal/adapters/projectionjson` directly imports only standard library and
  `application/projection`; its dependency graph may contain transitive
  `domain`, but no forbidden package;
- neither package imports `kin-openapi`, `internal/web`, Goshtoso, storage,
  source, HTTP, `syscall/js`, or generated API code;
- unrelated `example.com/manja-extension` imports `application/projection` with
  `GOWORK=off`, builds Vector A, and observes the documented DTO values;
- an independent Node test reads literal vectors A and B, verifies exact byte
  counts/digests and sentinel absence, and parses the documented
  identity/landmarks/operation metadata without importing Go code;
- `GOOS=js GOARCH=wasm GOWORK=off go build` compiles the pure projection
  package. No Wasm runtime, JS global, test-only parser, or asset is added.

## Security and Failure Behavior

Projection input and bytes are untrusted. Every failure is fail-closed and
returns no usable partial DTO/bytes. Error messages are at most 256 UTF-8 bytes
and name only a bounded, enumerated field path plus class (`invalid_utf8`,
`duplicate_key`, `unknown_field`, `invalid_identity`, `invalid_source`,
`duplicate_record`, `record_too_large`, `projection_too_large`, or
`non_canonical`), without
echoing spec/example bodies, unknown key text, identity values, or malformed
bytes. Named tests inject a sentinel into each of those sources and require both
sentinel absence and the 256-byte maximum.

`Builder.Build` must not return `domain.ValidateSpecIndex` text directly: deep
schema paths can exceed the cap even when they do not disclose values. Except
for exact `context.Canceled`/`context.DeadlineExceeded`, it converts source
validation failures to a coarse enumerated path/class such as
`projection[source]: invalid_source` without wrapping the original text. Codec
errors similarly expose only their own enumerated DTO path/class. Failures from
either exact `domain.ValidateCanonicalIdentity` call become bounded
`invalid_identity` errors naming only `projectId` or `revisionId`; they never
wrap or echo the domain error or rejected value. Tests cover empty, invalid
UTF-8, padded, and control-bearing identities, a depth-64 domain path, and
identity/example sentinels at the builder boundary, plus
unknown-key/malformed-byte sentinels at the codec boundary.

The DTO contains display strings, not trusted HTML. This precursor has no
`templ.Raw` and performs no HTML rendering. Later renderers must escape all
projection data through canonical components.

SHA-256 provides exact-byte integrity, not authenticity. Authenticity,
publication visibility, transport headers, cache state, and revocation remain
later-node responsibilities.

## POC Disposition

The preserved POC is branch `codex/wasm-pwa-poc` at
`ec4b4eda5667947370a52e4a1fa9d2aff89a7aa9`. It remains read-only. No commit or
file is an exact cherry-pick candidate.

Its 26-commit topology after merge-base
`6216bcb2d9a50083b130daa37cf381c6e2b4b601` is grouped as follows:

- raw engine/router/Wasm: `e6ad5c6`, `30b0167`, `f795997`;
- standalone shell/assets/offline proof: `1b996dc`, `f4c19dc`, `5018ef5`;
- shared-workspace and canonical-parity iteration: `e1f418a`, `2d12647`,
  `4fdeeb8`, `a588c96`, `9a30fa5`, `95d1caa`, `ee567a5`, `3421397`;
- freshness design/plan: `9d54dc8`, `a024127`;
- prepare/storage/revalidate/update/reload/corruption implementation:
  `ab5f171`, `a4a6894`, `2cb2c4a`, `e691562`, `34f0a43`, `39d71fc`;
- concurrency/evidence/handoff/reload fixes: `b5134b8`, `4fb6637`, `d2215f6`,
  `ec4b4ed`.

| POC evidence | Decision | Precursor use |
| --- | --- | --- |
| `e6ad5c6`, `internal/pwa/engine.go` | rewrite | Retain exact-byte digest and mismatch test intentions. Reject raw OpenAPI/parser import and full `SpecIndex` JSON snapshot, which includes excluded fields and accepts weak JSON. |
| `internal/pwa/engine_test.go` | selective test rewrite | Convert matching/malformed/checksum intent into golden, strict decode, sentinel, and boundary tests. No hydration/parser mock. |
| `30b0167`, `internal/pwa/router.go` | rewrite | Reuse anchor/href fixture values only. Reject Markdown, templates, asset defaults, `/openapi.json`, and routing. |
| `internal/pwa/router_test.go` | selective fixture port | Preserve overview, operation, schema, search ID, query-escape, and landmark expectations as DTO tests after new RED. |
| `internal/pwa/templates.go` | reject | Presentation-only `templ.Raw` placeholder and HTML rendering. |
| `f795997`, `cmd/manja-pwa/main.go`, `main_nonwasm.go`, `main_wasm.go` | reject now | Raw parser/hydration, active/prepared router, `syscall/js`, and JS globals belong to later Wasm/runtime nodes. |
| `cmd/manja-pwa/main_test.go` | defer | JS-global/runtime lifecycle proof belongs to a later reduced-Wasm command; none of its parser/hydration harness is ported. |
| `1b996dc`, `pwa/static/index.html`, `manifest.webmanifest` | reject | Standalone shell and global loading state are expressly discarded. |
| `pwa/static/app.js` | reject | UI, Service Worker control, update action, reload, and HTMX lifecycle are deferred. |
| `pwa/static/app.test.mjs` | selective fixture rewrite | Canonical selected-URL expectations inform new pure DTO REDs; every DOM, worker, update, and reload assertion stays deferred. |
| `a4a6894`, `pwa/static/spec-storage.js` | reject | IndexedDB, CacheStorage, generations, locks, repair, rollback, and pointers are outside precursor. |
| `pwa/static/spec-storage.test.mjs` | defer | Storage generation, corruption, concurrency, rollback, and quota cases belong to the later storage node; no helper is ported. |
| `2cb2c4a`, `pwa/static/sw.js` | reject now | Fetch interception, cache activation, and 64 MiB raw-spec behavior belong to later worker/original-spec nodes. |
| `pwa/static/sw.test.mjs` | selective test rewrite | Digest mismatch, malformed bytes, and byte-bound intentions become new pure codec REDs; fetch/cache/routing cases are deferred. |
| `f4c19dc`, `cmd/manja-pwa-assets/main.go`, `scripts/build-pwa.mjs` | reject | Goshtoso/Manja assets, raw fixture copy, `wasm_exec.js`, standalone build, and `pwa/dist` cleanup are forbidden here. |
| `cmd/manja-pwa-assets/main_test.go` | reject | Asset-path/build assertions test the rejected standalone package, not projection behavior. |
| `.gitignore`, `package.json` | reject | PWA build commands/output ignores are standalone build wiring and are not needed by a pure Go package/codec. |
| `internal/web/templates/public.templ`, `public_templ.go` | reject | POC is 22 commits behind frozen main and generated/UI code is stale. Never hand-port generated templates. |
| `internal/web/public_test.go` | defer/selective rewrite | Server-page parity expectations belong to the later server consumer; canonical anchor fixtures already enter DTO REDs. |
| `internal/web/templates/public_external_test.go` | defer | External HTML/Goshtoso parity belongs to the later renderer node and cannot validate a DTO-only precursor. |
| `internal/pwa/e2e/pwa_test.go` | defer | Browser shell, offline reload, worker routing, and server/Wasm parity belong to later DAG nodes. |
| `internal/pwa/e2e/freshness_test.go` | defer | Freshness, update, corruption recovery, and withdrawal behavior require storage/worker/runtime nodes. |
| `internal/pwa/e2e/harness_test.go` | reject now | POC server/browser/process harness support is not projection behavior; later E2E work rebuilds only needed support from its frozen base. |
| `README.md`; `docs/superpowers/plans/2026-07-26-wasm-pwa-proof-of-concept.md`; `docs/superpowers/plans/2026-07-26-wasm-pwa-public-docs-parity.md`; `docs/superpowers/plans/2026-07-27-wasm-pwa-spec-freshness.md`; `docs/superpowers/specs/2026-07-27-wasm-pwa-spec-freshness-design.md`; `docs/superpowers/snags/2026-07-26-wasm-pwa-poc.md` | evidence only | Preserve topology, measurements, freshness/corruption findings, and reversal lessons; they authorize no source port or cherry-pick. |
| POC performance evidence | receipt only | Raw snapshot 25,844,624 bytes and Wasm 23,763,920 bytes justify projection-first reduction; neither is precursor acceptance. |

Selective reuse always begins with a new failing test on the future frozen
implementation base. There are zero approved exact cherry-picks.

## Traceability to Parent Design

| Parent requirement | Precursor contract | Deferred owner |
| --- | --- | --- |
| Integration DAG exception | Separate spec/plan/branch; only `SpecIndex` plus deterministic bytes | Every full hybrid node waits for prerequisite integration base |
| Projection is not raw OpenAPI | Explicit DTO; excluded sentinels; no parser input | Original spec endpoint/cache |
| Slice-only, stable order, ordinals | Concrete DTO table, non-nil slices, pre-sort ordinals, canonical sorting | None |
| UTF-8 and `json.Marshal` rules | Pre-validation, exact struct order/escaping/integer/newline rules | None |
| Decimal OpenAPI values | Exact decimal normalization for explicitly typed embedded-JSON numbers; ambiguous domain display strings remain text | Typed-example domain extension if later interaction requires it |
| Strict duplicate/unknown/trailing rejection | Token pass, `DisallowUnknownFields`, canonical byte equality | Browser adapter later reuses codec |
| Exact-byte SHA-256 | Internal codec digest over uncompressed entity bytes | HTTP ETag formatting and Fetch verification |
| Per-record/total bounds | 256 KiB operation, 512 KiB schema, 16 MiB total | 2 MiB fragment and 64 MiB original-spec bounds |
| Canonical parity metadata | Anchors, relative hrefs, headings, landmarks in DTO/goldens | Server/Wasm renderers and browser parity |
| Provider-neutral application service | `application/projection` imports only `domain`/standard library | Endpoint/Wasm composition |
| Serialization/browser adapters internal | `internal/adapters/projectionjson`; no public codec | HTTP/Wasm/storage adapters |
| Security | Strict validation, bounded errors, no raw HTML | Same-origin routing, CSP, templ escaping, cache identity |
| Testing strategy | Unit/property/fuzz/architecture/consumer/Wasm compile | UI, worker, browser, performance tests |
| Rollout slice 1 | Unused package and codec only; independently revertible | Endpoint activation remains later |
| POC reuse policy | File/test/commit disposition; new RED; zero cherry-picks | Later nodes reassess deferred concepts |

## Rollout and Reversal

The precursor ships no route, descriptor, asset, page reference, storage schema,
or default. Its code can land unused after its focused and repository gates pass.
Reversal is one commit-series revert of `application/projection`,
`internal/adapters/projectionjson`, their tests/fixtures, and architecture
fixtures. No publication or browser data migration exists.

Format version 1 is immutable after another component consumes checked-in golden
bytes. Any incompatible field/order/anchor/bound change requires a new format
version and parallel decoder, not mutation of v1 goldens.

## Acceptance Criteria

- Builder product input is only validated `domain.SpecIndex`.
- Projection bytes contain neither excluded sentinel and do not change when
  excluded fields change.
- DTO uses concrete structs and non-nil slices only; no maps/generic objects or
  floats appear on wire.
- Golden bytes/digests, deterministic ordering, ordinals, canonical metadata,
  strict rejection, inclusive bounds, properties, and fuzz invariants pass.
- Public projection package and internal codec obey dependency tests; unrelated
  Go and Node consumers verify the intended public/wire implications.
- Package compiles for `js/wasm` without a runtime command or browser assets.
- No endpoint, UI, Goshtoso, publication, Wasm runtime, Service Worker, storage,
  offline, routing, rollout, or release behavior is implemented or claimed.
