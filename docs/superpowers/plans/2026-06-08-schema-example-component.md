# Schema Example Component Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a reusable placement-agnostic example component that renders fallback JSON server-side and hydrates generated examples client-side with `openapi-sampler`.

**Architecture:** Add raw schema JSON to Manja's indexed schema summaries, because the existing shallow `SchemaSummary` is useful for tables but not enough for browser-side sampling. Render one generic templ component around Goshtoso's codeblock; endpoint and schema callers pass IDs, labels, fallback examples, schema JSON, and options. Generate a static browser asset by combining the vendored `openapi-sampler` browser dist with a small Manja hydrator.

**Tech Stack:** Go, templ, Goshtoso codeblock, OpenAPI kin-openapi, Node.js, `openapi-sampler`, small browser JavaScript, Go tests, Node built-in test runner.

---

### Task 1: Preserve Raw Schema JSON

**Files:**
- Modify: `internal/core/spec.go`
- Modify: `internal/adapters/openapi/kin.go`
- Test: `internal/core/indexer_test.go`

- [ ] **Step 1: Write the failing parser test**

Add assertions in `TestOpenAPIParserIndexesOperationRequestAndResponses`:

```go
if !strings.Contains(requestMedia.Schema.JSON, `"required":["name"]`) {
	t.Fatalf("request schema JSON = %q", requestMedia.Schema.JSON)
}
if !strings.Contains(responseMedia.Schema.JSON, `"required":["id","name"]`) {
	t.Fatalf("response schema JSON = %q", responseMedia.Schema.JSON)
}
if idx.Schemas[0].Example.JSON == "" || !strings.Contains(idx.Schemas[0].Example.JSON, `"properties"`) {
	t.Fatalf("schema example payload = %#v", idx.Schemas[0].Example)
}
```

- [ ] **Step 2: Run the parser test to verify RED**

Run:

```bash
go test ./internal/core -run TestOpenAPIParserIndexesOperationRequestAndResponses -count=1
```

Expected: fail because `SchemaSummary.JSON` and `Schema.Example.JSON` do not exist.

- [ ] **Step 3: Add minimal core fields**

Extend the core structs:

```go
type SchemaExample struct {
	JSON    string
	Example string
}

type SchemaSummary struct {
	Name        string
	Type        string
	Format      string
	Description string
	Properties  []SchemaProperty
	Items       *SchemaSummary
	JSON        string
}

type Schema struct {
	Name        string
	Description string
	Example     SchemaExample
}
```

- [ ] **Step 4: Populate raw schema JSON**

In `internal/adapters/openapi/kin.go`, add a helper:

```go
func schemaJSON(schema *openapi3.Schema) string {
	if schema == nil {
		return ""
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return ""
	}
	return string(data)
}
```

Set `summary.JSON = schemaJSON(schema)` in `schemaSummaryDepth` for depth `0`, and set `core.Schema.Example.JSON` from component schemas when indexing `idx.Schemas`.

- [ ] **Step 5: Run parser test to verify GREEN**

Run:

```bash
go test ./internal/core -run TestOpenAPIParserIndexesOperationRequestAndResponses -count=1
```

Expected: pass.

### Task 2: Render Generic Example Component

**Files:**
- Modify: `internal/web/templates/public.templ`
- Test: `internal/web/public_test.go`

- [ ] **Step 1: Write failing render tests**

Add a web test proving one generic component serves endpoint and schema examples:

```go
func TestPublicDocsRenderGenericSchemaExamples(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Todos",
		Operations: []core.Operation{{
			ID: "updateTodo", Anchor: "operation-updatetodo", Method: "PUT", Path: "/todos/{todoId}", Summary: "Update Todo",
			RequestBody: &core.OperationRequestBody{MediaTypes: []core.OperationMediaType{{
				ContentType: "application/json",
				Schema: core.SchemaSummary{Name: "TodoInput", Type: "object", JSON: `{"type":"object","required":["name"],"properties":{"name":{"type":"string"},"done":{"type":"boolean"}}}`},
				Example: "{\n  \"name\": \"fallback\"\n}",
			}}},
			Responses: []core.OperationResponse{{Status: "200", MediaTypes: []core.OperationMediaType{{
				ContentType: "application/json",
				Schema: core.SchemaSummary{Name: "Todo", Type: "object", JSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`},
				Example: "{\n  \"id\": \"fallback\"\n}",
			}}}},
		}},
		Schemas: []core.Schema{{Name: "Todo", Description: "A todo.", Example: core.SchemaExample{
			JSON: `{"type":"object","required":["id"],"properties":{"id":{"type":"string"},"name":{"type":"string"}}}`,
			Example: "{\n  \"id\": \"fallback\"\n}",
		}}},
	}

	body := renderPublicDocs(t, NewPublicServer(idx), "/")
	for _, want := range []string{
		`data-manja-example`,
		`id="operation-updatetodo-request-body-application-json-example"`,
		`Request Example: application/json`,
		`Response Example: 200 application/json`,
		`type="application/json"`,
		`"skipNonRequired":false`,
		`/manja-assets/schema-example.js`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("generic endpoint example missing %q:\n%s", want, body)
		}
	}

	body = renderPublicDocs(t, NewPublicServer(idx), "/?selected=schema-todo")
	for _, want := range []string{
		`id="schema-todo-example"`,
		`Example: Todo`,
		`data-manja-example`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("generic schema example missing %q:\n%s", want, body)
		}
	}
	for _, reject := range []string{"Try It", "Send API Request", "<form", "Execute request"} {
		if strings.Contains(body, reject) {
			t.Fatalf("example component should stay read-only, got %q:\n%s", reject, body)
		}
	}
}
```

- [ ] **Step 2: Run web test to verify RED**

Run:

```bash
go test ./internal/web -run TestPublicDocsRenderGenericSchemaExamples -count=1
```

Expected: fail because the generic component and helper fields do not exist.

- [ ] **Step 3: Add component config and template**

Add `ExampleOptions`, `ExampleConfig`, `schemaExample`, and `schemaExamplePayloadJSON` helpers in `public.templ`. `schemaExample` wraps `codeblock.CodeBlock` with:

```templ
<div data-manja-example data-manja-example-id={ cfg.ID }>
	@codeblock.CodeBlock(codeblock.Config{ID: cfg.ID, Label: cfg.Label, Language: cfg.Language, Code: exampleFallback(cfg), MaxHeight: "24rem"})
	<script id={ cfg.ID + "-payload" } type="application/json">{ schemaExamplePayloadJSON(cfg) }</script>
	<p data-manja-example-status class="sr-only" aria-live="polite"></p>
</div>
```

- [ ] **Step 4: Wire endpoint and schema callers**

Replace request/response `@codeExample(...)` calls with `@schemaExample(...)` when a media item has `Schema.JSON` or `Example`. Add a schema page example when `schema.Example.JSON` or `schema.Example.Example` is present.

- [ ] **Step 5: Run web test to verify GREEN**

Run:

```bash
go test ./internal/web -run TestPublicDocsRenderGenericSchemaExamples -count=1
```

Expected: pass.

### Task 3: Add Client-Side Sampler Asset

**Files:**
- Create: `internal/web/static/schema-example-hydrator.js`
- Create: `scripts/build-schema-example-asset.mjs`
- Create generated: `internal/web/static/schema-example.js`
- Modify: `package.json`
- Test: `internal/web/static/schema-example-hydrator.test.mjs`

- [ ] **Step 1: Write failing client tests**

Create tests using Node's built-in test runner and a minimal fake DOM:

```js
import test from 'node:test';
import assert from 'node:assert/strict';
import { hydrateSchemaExamples } from './schema-example-hydrator.js';

test('hydrates multiple examples and honors skipNonRequired', () => {
  const roots = [
    fakeRoot('a', { schema: objectSchema(), options: { skipNonRequired: true } }, 'fallback'),
    fakeRoot('b', { schema: objectSchema(), options: { skipNonRequired: false } }, 'fallback'),
  ];
  hydrateSchemaExamples({ roots, sampler: { sample: sampleSchema } });
  assert.equal(roots[0].code.textContent, '{\n  "name": "string"\n}');
  assert.equal(roots[1].code.textContent, '{\n  "name": "string",\n  "done": true\n}');
});

test('preserves fallback when sampling fails and is idempotent', () => {
  const root = fakeRoot('broken', { schema: { type: 'object' }, options: {} }, 'fallback');
  hydrateSchemaExamples({ roots: [root], sampler: { sample() { throw new Error('boom'); } } });
  hydrateSchemaExamples({ roots: [root], sampler: { sample() { throw new Error('boom'); } } });
  assert.equal(root.code.textContent, 'fallback');
  assert.equal(root.dataset.manjaExampleHydrated, 'true');
});
```

- [ ] **Step 2: Run client tests to verify RED**

Run:

```bash
node --test internal/web/static/schema-example-hydrator.test.mjs
```

Expected: fail because `schema-example-hydrator.js` does not exist.

- [ ] **Step 3: Implement hydrator and build script**

Export `hydrateSchemaExamples({ roots, sampler })`, and on browser load call it with `window.OpenAPISampler`. Generate `schema-example.js` by concatenating `node_modules/openapi-sampler/dist/openapi-sampler.js` and `internal/web/static/schema-example-hydrator.js`.

- [ ] **Step 4: Add npm script**

Add:

```json
"examples:build": "node scripts/build-schema-example-asset.mjs",
"examples:test": "node --test internal/web/static/schema-example-hydrator.test.mjs"
```

- [ ] **Step 5: Run client tests and build to verify GREEN**

Run:

```bash
npm run examples:test
npm run examples:build
```

Expected: tests pass and `internal/web/static/schema-example.js` is generated.

### Task 4: Regenerate And Verify

**Files:**
- Modify generated: `internal/web/templates/public_templ.go`
- Modify generated: `internal/web/templates/layout_templ.go`

- [ ] **Step 1: Regenerate templ output**

Run:

```bash
go run github.com/a-h/templ/cmd/templ generate
```

Expected: `*_templ.go` files update from `.templ` sources only.

- [ ] **Step 2: Run focused tests**

Run:

```bash
go test ./internal/core ./internal/web -count=1
npm run examples:test
npm run examples:build
```

Expected: all pass.

- [ ] **Step 3: Run full verification**

Run:

```bash
go test ./...
npm run api:bundle
npm run api:lint
git diff --check
```

Expected: all commands succeed. `npm run api:lint` may print the existing three Redocly warnings while exiting successfully.

- [ ] **Step 4: Commit**

Stage only the implementation files and commit:

```bash
git add internal/core/spec.go internal/adapters/openapi/kin.go internal/core/indexer_test.go internal/web/public_test.go internal/web/templates/public.templ internal/web/templates/public_templ.go internal/web/templates/layout.templ internal/web/templates/layout_templ.go internal/web/static/schema-example-hydrator.js internal/web/static/schema-example-hydrator.test.mjs internal/web/static/schema-example.js scripts/build-schema-example-asset.mjs package.json package-lock.json docs/superpowers/plans/2026-06-08-schema-example-component.md
git commit -m "feat: add schema example component"
```
