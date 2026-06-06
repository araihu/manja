# Manja Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Manja's first working vertical slice: a Go/Goshtoso app that loads OpenAPI specs from file/git-like sources, indexes them, stores metadata/files locally, renders read-only public docs, and exposes high-quality Ctrl+K search.

**Architecture:** Implement ports-first hexagonal boundaries with filesystem-backed adapters. Keep REST API work YAGNI: scaffold API-first tooling, but only create a health endpoint initially. UI routes are server-rendered HTML with Goshtoso primitives.

**Tech Stack:** Go 1.24+, templ, Goshtoso, Goldmark, kin-openapi, Redocly CLI, oapi-codegen, testcontainers-go, Dex/Forgejo testcontainers later.

---

## File Structure

- `go.mod`: root Go module `github.com/araihu/manja`.
- `tools.go`: pinned Go tools for templ and oapi-codegen.
- `package.json`: Redocly CLI scripts for API spec joining/linting.
- `api/openapi.yaml`: root management API document.
- `api/paths/health.yaml`: first API-first REST endpoint.
- `api/components/schemas.yaml`: shared OpenAPI schemas.
- `cmd/manja/main.go`: executable entry point.
- `internal/app/app.go`: dependency wiring and HTTP server construction.
- `internal/core/project.go`: project/source/publication/theme/SEO domain types.
- `internal/core/spec.go`: spec revision, parsed document, index, search records.
- `internal/core/ports.go`: Store, BlobStore, SecretStore, Cache, Source, Markdown, Parser ports.
- `internal/core/indexer.go`: OpenAPI-to-index transformation.
- `internal/core/indexer_test.go`: indexer tests from fixture specs.
- `internal/adapters/openapi/kin.go`: kin-openapi parser adapter.
- `internal/adapters/openapi/testdata/petstore.yaml`: fixture spec.
- `internal/adapters/store/fs.go`: filesystem Store and BlobStore.
- `internal/adapters/store/fs_test.go`: temp-dir store tests.
- `internal/adapters/markdown/goldmark.go`: Goldmark Markdown renderer.
- `internal/adapters/markdown/goldmark_test.go`: safe Markdown tests.
- `internal/adapters/cache/memory.go`: in-memory cache-aside blob cache.
- `internal/adapters/source/file.go`: local file source adapter for v1 fixtures and non-git singleton sources.
- `internal/web/server.go`: route registration.
- `internal/web/public.go`: public docs handlers.
- `internal/web/management.go`: authenticated/admin placeholder routes.
- `internal/web/api.go`: generated API handler implementation.
- `internal/web/templates/layout.templ`: Goshtoso page shell.
- `internal/web/templates/public.templ`: public docs views.
- `internal/web/templates/search.templ`: search payload wiring around Goshtoso search.
- `internal/web/templates/generated`: generated templ files after `templ generate`.
- `internal/web/static/manja.css`: app-specific token-based Markdown/docs CSS.
- `internal/web/e2e/public_docs_test.go`: public docs and Ctrl+K UI tests.
- `.github/workflows/ci.yml`: build, test, templ drift, API bundle checks.

## Task 1: Scaffold Module, Tooling, And CI

**Files:**
- Create: `go.mod`
- Create: `tools.go`
- Create: `package.json`
- Create: `.gitignore`
- Create: `.github/workflows/ci.yml`
- Create: `cmd/manja/main.go`

- [ ] **Step 1: Write the initial module files**

Create `go.mod`:

```go
module github.com/araihu/manja

go 1.24

require (
	github.com/araihu/goshtoso v0.0.4
	github.com/a-h/templ v0.3.924
)
```

Create `tools.go`:

```go
//go:build tools

package tools

import (
	_ "github.com/a-h/templ/cmd/templ"
	_ "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
)
```

Create `package.json`:

```json
{
  "name": "manja",
  "private": true,
  "scripts": {
    "api:bundle": "redocly bundle api/openapi.yaml -o api/dist/openapi.yaml",
    "api:lint": "redocly lint api/openapi.yaml"
  },
  "devDependencies": {
    "@redocly/cli": "^1.34.5"
  }
}
```

Create `.gitignore`:

```gitignore
/bin/
/tmp/
/api/dist/
/.superpowers/
/node_modules/
*.test
```

Create `cmd/manja/main.go`:

```go
package main

import "fmt"

func main() {
	fmt.Println("manja")
}
```

- [ ] **Step 2: Run dependency resolution**

Run:

```bash
go mod tidy
npm install
```

Expected: `go.sum` and `package-lock.json` are created.

- [ ] **Step 3: Add CI**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: actions/setup-node@v4
        with:
          node-version: 22
          cache: npm
      - run: npm ci
      - run: go mod tidy
      - run: git diff --exit-code go.mod go.sum
      - run: go test ./...
      - run: npm run api:bundle
      - run: npm run api:lint
```

- [ ] **Step 4: Verify scaffold**

Run:

```bash
go test ./...
npm run api:bundle
```

Expected: `go test` passes for the trivial module. `api:bundle` fails because the API spec does not exist yet.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum tools.go package.json package-lock.json .gitignore .github/workflows/ci.yml cmd/manja/main.go
git commit -m "chore: scaffold Manja module"
```

## Task 2: Add Minimal API-First Management Spec

**Files:**
- Create: `api/openapi.yaml`
- Create: `api/paths/health.yaml`
- Create: `api/components/schemas.yaml`
- Create: `internal/web/api.gen.go`
- Create: `internal/web/api.go`
- Test: `internal/web/api_test.go`

- [ ] **Step 1: Add split OpenAPI files**

Create `api/openapi.yaml`:

```yaml
openapi: 3.1.0
info:
  title: Manja Management API
  version: 0.1.0
servers:
  - url: http://localhost:8080
paths:
  /api/healthz:
    $ref: ./paths/health.yaml
components:
  schemas:
    Health:
      $ref: ./components/schemas.yaml#/Health
```

Create `api/paths/health.yaml`:

```yaml
get:
  operationId: getHealth
  summary: Health check
  responses:
    "200":
      description: Service is healthy
      content:
        application/json:
          schema:
            $ref: ../components/schemas.yaml#/Health
```

Create `api/components/schemas.yaml`:

```yaml
Health:
  type: object
  required: [status]
  properties:
    status:
      type: string
      enum: [ok]
```

- [ ] **Step 2: Verify Redocly**

Run:

```bash
npm run api:bundle
npm run api:lint
```

Expected: both pass and `api/dist/openapi.yaml` is generated but not committed.

- [ ] **Step 3: Generate Go API types**

Run:

```bash
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen \
  -generate types,strict-server \
  -package web \
  -o internal/web/api.gen.go \
  api/dist/openapi.yaml
```

Expected: `internal/web/api.gen.go` exists.

- [ ] **Step 4: Write failing API test**

Create `internal/web/api_test.go`:

```go
package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthAPI(t *testing.T) {
	srv := NewAPIServer()
	req := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != `{"status":"ok"}`+"\n" {
		t.Fatalf("body = %q", got)
	}
}
```

- [ ] **Step 5: Run test and verify failure**

Run:

```bash
go test ./internal/web -run TestHealthAPI -v
```

Expected: fail with `undefined: NewAPIServer`.

- [ ] **Step 6: Implement health API**

Create `internal/web/api.go`:

```go
package web

import (
	"context"
	"encoding/json"
	"net/http"
)

type apiServer struct{}

func NewAPIServer() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		res, err := apiServer{}.GetHealth(r.Context(), GetHealthRequestObject{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		body := res.(GetHealth200JSONResponse)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	return mux
}

func (apiServer) GetHealth(context.Context, GetHealthRequestObject) (GetHealthResponseObject, error) {
	return GetHealth200JSONResponse{Status: HealthStatusOk}, nil
}
```

- [ ] **Step 7: Verify API**

Run:

```bash
go test ./internal/web -run TestHealthAPI -v
go test ./...
```

Expected: tests pass.

- [ ] **Step 8: Commit**

```bash
git add api/openapi.yaml api/paths/health.yaml api/components/schemas.yaml internal/web/api.gen.go internal/web/api.go internal/web/api_test.go
git commit -m "feat: add API-first health endpoint"
```

## Task 3: Define Core Domain And Ports

**Files:**
- Create: `internal/core/project.go`
- Create: `internal/core/spec.go`
- Create: `internal/core/ports.go`
- Test: `internal/core/project_test.go`

- [ ] **Step 1: Write domain invariant tests**

Create `internal/core/project_test.go`:

```go
package core

import "testing"

func TestPublicationVisibility(t *testing.T) {
	pub := Publication{ProjectID: "p1", RevisionID: "r1", Public: true, Path: "/acme/payments/v1"}
	if !pub.VisibleTo(Actor{Anonymous: true}) {
		t.Fatal("public publication should be visible to anonymous readers")
	}
}

func TestPrivateRevisionHiddenFromAnonymous(t *testing.T) {
	pub := Publication{ProjectID: "p1", RevisionID: "r1", Public: false}
	if pub.VisibleTo(Actor{Anonymous: true}) {
		t.Fatal("private publication should be hidden from anonymous readers")
	}
	if !pub.VisibleTo(Actor{UserID: "u1", ProjectIDs: []string{"p1"}}) {
		t.Fatal("project member should see private publication")
	}
}
```

- [ ] **Step 2: Verify failure**

Run:

```bash
go test ./internal/core -run TestPublication -v
```

Expected: fail because core types are undefined.

- [ ] **Step 3: Implement core types**

Create `internal/core/project.go`:

```go
package core

type Project struct {
	ID       string
	Name     string
	Slug     string
	SEO      ProjectSEO
	Theme    ThemeSettings
	SourceIDs []string
}

type ProjectSEO struct {
	TitleTemplate string
	Description   string
	CanonicalBase string
	SocialImage   string
	Robots        string
}

type ThemeSettings struct {
	Theme    string
	DarkMode string
}

type Source struct {
	ID           string
	ProjectID    string
	Kind         string
	SpecPath     string
	CredentialID string
}

type Credential struct {
	ID        string
	SourceID  string
	Kind      string
	SecretRef string
}

type Publication struct {
	ProjectID  string
	RevisionID string
	Public     bool
	Path       string
	Hostname   string
}

type Actor struct {
	Anonymous  bool
	UserID     string
	ProjectIDs []string
}

func (p Publication) VisibleTo(actor Actor) bool {
	if p.Public {
		return true
	}
	if actor.Anonymous {
		return false
	}
	for _, id := range actor.ProjectIDs {
		if id == p.ProjectID {
			return true
		}
	}
	return false
}
```

Create `internal/core/spec.go`:

```go
package core

type SpecFile struct {
	SourceID string
	Path     string
	Format   string
	Bytes    []byte
}

type Revision struct {
	ID        string
	SourceID  string
	Ref       string
	CommitSHA string
	Version   string
}

type SpecIndex struct {
	ProjectID string
	RevisionID string
	Title     string
	Version   string
	Operations []Operation
	Schemas    []Schema
	Search     []SearchDocument
	PublicRoutes []PublicRoute
}

type Operation struct {
	ID          string
	Method      string
	Path        string
	Summary     string
	Description string
	Tags        []string
	Deprecated  bool
}

type Schema struct {
	Name        string
	Description string
}

type SearchDocument struct {
	ID          string
	Title       string
	Description string
	Href        string
	Kind        string
	Section     string
	Keywords    []string
}

type PublicRoute struct {
	Path        string
	Title       string
	Description string
}
```

Create `internal/core/ports.go`:

```go
package core

import "context"

type Store interface {
	SaveProject(context.Context, Project) error
	Project(context.Context, string) (Project, error)
	SaveRevision(context.Context, Revision) error
	Revision(context.Context, string) (Revision, error)
	SavePublication(context.Context, Publication) error
}

type BlobStore interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
}

type SecretStore interface {
	PutSecret(context.Context, string, []byte) error
	GetSecret(context.Context, string) ([]byte, error)
}

type Cache interface {
	Get(string) ([]byte, bool)
	Set(string, []byte)
	Delete(string)
}

type Parser interface {
	Parse(context.Context, SpecFile, Revision) (SpecIndex, error)
}

type MarkdownRenderer interface {
	Render(context.Context, string) (MarkdownResult, error)
}

type MarkdownResult struct {
	HTML  string
	Plain string
}
```

- [ ] **Step 4: Verify**

Run:

```bash
go test ./internal/core -v
go test ./...
```

Expected: tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/core
git commit -m "feat: define core domain ports"
```

## Task 4: Parse OpenAPI Specs Into Search Indexes

**Files:**
- Create: `internal/adapters/openapi/kin.go`
- Create: `internal/adapters/openapi/testdata/petstore.yaml`
- Create: `internal/core/indexer.go`
- Test: `internal/core/indexer_test.go`

- [ ] **Step 1: Add fixture spec**

Create `internal/adapters/openapi/testdata/petstore.yaml`:

```yaml
openapi: 3.1.0
info:
  title: Petstore
  version: 1.0.0
  description: Sample pet API.
paths:
  /pets:
    get:
      operationId: listPets
      summary: List pets
      description: Returns visible pets.
      tags: [Pets]
      responses:
        "200":
          description: Pet list
  /pets/{petId}:
    get:
      operationId: getPet
      summary: Get pet
      tags: [Pets]
      parameters:
        - name: petId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Pet detail
components:
  schemas:
    Pet:
      type: object
      description: A pet record.
      properties:
        id:
          type: string
        name:
          type: string
```

- [ ] **Step 2: Write failing indexer test**

Create `internal/core/indexer_test.go`:

```go
package core

import (
	"context"
	"os"
	"testing"

	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
)

func TestOpenAPIParserBuildsSearchIndex(t *testing.T) {
	data, err := os.ReadFile("../adapters/openapi/testdata/petstore.yaml")
	if err != nil {
		t.Fatal(err)
	}
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(context.Background(), SpecFile{
		SourceID: "src1",
		Path:     "openapi.yaml",
		Format:   "yaml",
		Bytes:    data,
	}, Revision{ID: "rev1", SourceID: "src1", Ref: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if idx.Title != "Petstore" || idx.Version != "1.0.0" {
		t.Fatalf("identity = %q %q", idx.Title, idx.Version)
	}
	if len(idx.Operations) != 2 {
		t.Fatalf("operations = %d, want 2", len(idx.Operations))
	}
	if idx.Search[0].Title != "GET /pets" {
		t.Fatalf("first search title = %q", idx.Search[0].Title)
	}
	if len(idx.Schemas) != 1 || idx.Schemas[0].Name != "Pet" {
		t.Fatalf("schemas = %#v", idx.Schemas)
	}
}
```

- [ ] **Step 3: Verify failure**

Run:

```bash
go test ./internal/core -run TestOpenAPIParserBuildsSearchIndex -v
```

Expected: fail because `internal/adapters/openapi` does not exist.

- [ ] **Step 4: Add parser dependency**

Run:

```bash
go get github.com/getkin/kin-openapi@latest
```

- [ ] **Step 5: Implement parser**

Create `internal/adapters/openapi/kin.go`:

```go
package openapi

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/araihu/manja/internal/core"
)

type Parser struct{}

func (Parser) Parse(ctx context.Context, file core.SpecFile, rev core.Revision) (core.SpecIndex, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromDataWithPath(file.Bytes, file.Path)
	if err != nil {
		return core.SpecIndex{}, err
	}
	if err := doc.Validate(ctx); err != nil {
		return core.SpecIndex{}, err
	}

	idx := core.SpecIndex{
		RevisionID: rev.ID,
		Title:      doc.Info.Title,
		Version:    doc.Info.Version,
	}

	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			operation := core.Operation{
				ID:          op.OperationID,
				Method:      strings.ToUpper(method),
				Path:        path,
				Summary:     op.Summary,
				Description: op.Description,
				Tags:        append([]string(nil), op.Tags...),
				Deprecated:  op.Deprecated,
			}
			idx.Operations = append(idx.Operations, operation)
		}
	}
	sort.Slice(idx.Operations, func(i, j int) bool {
		if idx.Operations[i].Path == idx.Operations[j].Path {
			return idx.Operations[i].Method < idx.Operations[j].Method
		}
		return idx.Operations[i].Path < idx.Operations[j].Path
	})

	for name, schema := range doc.Components.Schemas {
		description := ""
		if schema != nil && schema.Value != nil {
			description = schema.Value.Description
		}
		idx.Schemas = append(idx.Schemas, core.Schema{Name: name, Description: description})
	}
	sort.Slice(idx.Schemas, func(i, j int) bool { return idx.Schemas[i].Name < idx.Schemas[j].Name })

	idx.Search = buildSearch(idx)
	idx.PublicRoutes = buildPublicRoutes(idx)
	return idx, nil
}

func buildSearch(idx core.SpecIndex) []core.SearchDocument {
	docs := make([]core.SearchDocument, 0, len(idx.Operations)+len(idx.Schemas)+1)
	docs = append(docs, core.SearchDocument{
		ID:          "overview",
		Title:       idx.Title,
		Description: "API overview",
		Href:        "/",
		Kind:        "Overview",
	})
	for _, op := range idx.Operations {
		title := fmt.Sprintf("%s %s", op.Method, op.Path)
		docs = append(docs, core.SearchDocument{
			ID:          "operation-" + op.ID,
			Title:       title,
			Description: firstNonEmpty(op.Summary, op.Description),
			Href:        "#operation-" + op.ID,
			Kind:        "Operation",
			Section:     strings.Join(op.Tags, ", "),
			Keywords:    []string{op.ID, op.Method, op.Path, strings.Join(op.Tags, " ")},
		})
	}
	for _, schema := range idx.Schemas {
		docs = append(docs, core.SearchDocument{
			ID:          "schema-" + schema.Name,
			Title:       schema.Name,
			Description: schema.Description,
			Href:        "#schema-" + strings.ToLower(schema.Name),
			Kind:        "Schema",
			Section:     "Schemas",
			Keywords:    []string{"schema", schema.Name},
		})
	}
	return docs
}

func buildPublicRoutes(idx core.SpecIndex) []core.PublicRoute {
	routes := []core.PublicRoute{{Path: "/", Title: idx.Title, Description: "API overview"}}
	for _, op := range idx.Operations {
		routes = append(routes, core.PublicRoute{
			Path:        "#operation-" + op.ID,
			Title:       op.Method + " " + op.Path,
			Description: firstNonEmpty(op.Summary, op.Description),
		})
	}
	return routes
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
```

- [ ] **Step 6: Verify**

Run:

```bash
go test ./internal/core -run TestOpenAPIParserBuildsSearchIndex -v
go test ./...
```

Expected: tests pass.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/core/indexer_test.go internal/adapters/openapi
git commit -m "feat: index OpenAPI specs"
```

## Task 5: Add Filesystem Store, Blob Store, And In-Memory Cache

**Files:**
- Create: `internal/adapters/store/fs.go`
- Test: `internal/adapters/store/fs_test.go`
- Create: `internal/adapters/cache/memory.go`
- Test: `internal/adapters/cache/memory_test.go`

- [ ] **Step 1: Write filesystem store test**

Create `internal/adapters/store/fs_test.go`:

```go
package store

import (
	"context"
	"testing"

	"github.com/araihu/manja/internal/core"
)

func TestFileStorePersistsProjectRevisionPublicationAndBlob(t *testing.T) {
	ctx := context.Background()
	fs := NewFileStore(t.TempDir())

	project := core.Project{ID: "p1", Name: "Payments", Slug: "payments"}
	if err := fs.SaveProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	gotProject, err := fs.Project(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if gotProject.Name != "Payments" {
		t.Fatalf("project name = %q", gotProject.Name)
	}

	rev := core.Revision{ID: "r1", SourceID: "s1", Ref: "main"}
	if err := fs.SaveRevision(ctx, rev); err != nil {
		t.Fatal(err)
	}
	gotRev, err := fs.Revision(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if gotRev.Ref != "main" {
		t.Fatalf("revision ref = %q", gotRev.Ref)
	}

	if err := fs.Put(ctx, "specs/r1.yaml", []byte("openapi: 3.1.0")); err != nil {
		t.Fatal(err)
	}
	blob, err := fs.Get(ctx, "specs/r1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(blob) != "openapi: 3.1.0" {
		t.Fatalf("blob = %q", blob)
	}
}
```

- [ ] **Step 2: Verify failure**

Run:

```bash
go test ./internal/adapters/store -v
```

Expected: fail with `undefined: NewFileStore`.

- [ ] **Step 3: Implement filesystem store**

Create `internal/adapters/store/fs.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/araihu/manja/internal/core"
)

type FileStore struct {
	root string
}

func NewFileStore(root string) *FileStore {
	return &FileStore{root: root}
}

func (s *FileStore) SaveProject(ctx context.Context, p core.Project) error {
	return s.writeJSON(ctx, filepath.Join("projects", p.ID+".json"), p)
}

func (s *FileStore) Project(ctx context.Context, id string) (core.Project, error) {
	var p core.Project
	err := s.readJSON(ctx, filepath.Join("projects", id+".json"), &p)
	return p, err
}

func (s *FileStore) SaveRevision(ctx context.Context, r core.Revision) error {
	return s.writeJSON(ctx, filepath.Join("revisions", r.ID+".json"), r)
}

func (s *FileStore) Revision(ctx context.Context, id string) (core.Revision, error) {
	var r core.Revision
	err := s.readJSON(ctx, filepath.Join("revisions", id+".json"), &r)
	return r, err
}

func (s *FileStore) SavePublication(ctx context.Context, p core.Publication) error {
	name := p.ProjectID + "-" + p.RevisionID + ".json"
	return s.writeJSON(ctx, filepath.Join("publications", name), p)
}

func (s *FileStore) Put(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.safePath(filepath.Join("blobs", key))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (s *FileStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := s.safePath(filepath.Join("blobs", key))
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (s *FileStore) writeJSON(ctx context.Context, name string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.safePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func (s *FileStore) readJSON(ctx context.Context, name string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.safePath(name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func (s *FileStore) safePath(name string) (string, error) {
	clean := filepath.Clean(name)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", errors.New("unsafe store path")
	}
	return filepath.Join(s.root, clean), nil
}
```

- [ ] **Step 4: Add cache test and implementation**

Create `internal/adapters/cache/memory_test.go`:

```go
package cache

import "testing"

func TestMemoryCacheCopiesValues(t *testing.T) {
	c := NewMemory()
	in := []byte("abc")
	c.Set("k", in)
	in[0] = 'z'
	got, ok := c.Get("k")
	if !ok || string(got) != "abc" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	got[0] = 'x'
	got, _ = c.Get("k")
	if string(got) != "abc" {
		t.Fatalf("cache leaked mutable slice: %q", got)
	}
}
```

Create `internal/adapters/cache/memory.go`:

```go
package cache

import "sync"

type Memory struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func NewMemory() *Memory {
	return &Memory{data: map[string][]byte{}}
}

func (c *Memory) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok := c.data[key]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), value...), true
}

func (c *Memory) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = append([]byte(nil), value...)
}

func (c *Memory) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}
```

- [ ] **Step 5: Verify**

Run:

```bash
go test ./internal/adapters/store ./internal/adapters/cache -v
go test ./...
```

Expected: tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/store internal/adapters/cache
git commit -m "feat: add local storage adapters"
```

## Task 6: Add Goldmark Markdown Renderer

**Files:**
- Create: `internal/adapters/markdown/goldmark.go`
- Test: `internal/adapters/markdown/goldmark_test.go`

- [ ] **Step 1: Write Markdown safety test**

Create `internal/adapters/markdown/goldmark_test.go`:

```go
package markdown

import (
	"context"
	"strings"
	"testing"
)

func TestRendererDisablesRawHTMLAndExtractsPlainText(t *testing.T) {
	r := NewRenderer()
	out, err := r.Render(context.Background(), "# Hello\n\n<script>alert(1)</script>\n\nUse `pets`.")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.HTML, "<script>") {
		t.Fatalf("raw HTML was not escaped: %s", out.HTML)
	}
	if !strings.Contains(out.HTML, `class="manja-markdown"`) {
		t.Fatalf("missing wrapper: %s", out.HTML)
	}
	if !strings.Contains(out.Plain, "Hello") || !strings.Contains(out.Plain, "Use pets") {
		t.Fatalf("plain text = %q", out.Plain)
	}
}
```

- [ ] **Step 2: Verify failure**

Run:

```bash
go test ./internal/adapters/markdown -v
```

Expected: fail because package implementation does not exist.

- [ ] **Step 3: Add Goldmark dependency**

Run:

```bash
go get github.com/yuin/goldmark@latest
go get github.com/yuin/goldmark/extension@latest
```

- [ ] **Step 4: Implement renderer**

Create `internal/adapters/markdown/goldmark.go`:

```go
package markdown

import (
	"bytes"
	"context"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/araihu/manja/internal/core"
)

type Renderer struct {
	md goldmark.Markdown
}

func NewRenderer() Renderer {
	return Renderer{md: goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)}
}

func (r Renderer) Render(ctx context.Context, input string) (core.MarkdownResult, error) {
	if err := ctx.Err(); err != nil {
		return core.MarkdownResult{}, err
	}
	sanitized := stripRawHTML(input)
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(sanitized), &buf); err != nil {
		return core.MarkdownResult{}, err
	}
	html := `<div class="manja-markdown">` + buf.String() + `</div>`
	return core.MarkdownResult{HTML: html, Plain: plainText(sanitized)}, nil
}

var rawHTML = regexp.MustCompile(`(?is)<[^>]+>`)

func stripRawHTML(input string) string {
	return rawHTML.ReplaceAllString(input, "")
}

func plainText(input string) string {
	text := stripRawHTML(input)
	text = strings.ReplaceAll(text, "`", "")
	text = strings.ReplaceAll(text, "#", "")
	text = strings.ReplaceAll(text, "*", "")
	return strings.Join(strings.Fields(text), " ")
}
```

- [ ] **Step 5: Verify**

Run:

```bash
go test ./internal/adapters/markdown -v
go test ./...
```

Expected: tests pass.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/adapters/markdown
git commit -m "feat: add markdown renderer"
```

## Task 7: Render Public Docs With Goshtoso And Search

**Files:**
- Create: `internal/app/app.go`
- Create: `internal/web/server.go`
- Create: `internal/web/public.go`
- Create: `internal/web/templates/layout.templ`
- Create: `internal/web/templates/public.templ`
- Create: `internal/web/static/manja.css`
- Modify: `cmd/manja/main.go`
- Test: `internal/web/public_test.go`

- [ ] **Step 1: Write public docs handler test**

Create `internal/web/public_test.go`:

```go
package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/araihu/manja/internal/core"
)

func TestPublicDocsRenderSearchAndOperations(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Petstore",
		Version: "1.0.0",
		Operations: []core.Operation{{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}}},
		Search: []core.SearchDocument{{ID: "operation-listPets", Title: "GET /pets", Description: "List pets", Href: "#operation-listPets", Kind: "Operation", Section: "Pets"}},
	}
	srv := NewPublicServer(idx)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Petstore", "GET", "/pets", "Search docs", "operation-listPets"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}
```

- [ ] **Step 2: Verify failure**

Run:

```bash
go test ./internal/web -run TestPublicDocsRenderSearchAndOperations -v
```

Expected: fail with `undefined: NewPublicServer`.

- [ ] **Step 3: Create templ views**

Create `internal/web/templates/layout.templ`:

```go
package templates

import "github.com/araihu/goshtoso/components/head"

templ Layout(title string) {
	<!DOCTYPE html>
	<html lang="en" data-theme="goshtoso">
		<head>
			<meta charset="UTF-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
			<title>{ title }</title>
			@head.Dependencies()
			<link rel="stylesheet" href="/manja-assets/manja.css"/>
			<style>[x-cloak] { display: none !important; }</style>
		</head>
		<body class="min-h-screen bg-surface text-on-surface dark:bg-surface-dark dark:text-on-surface-dark">
			{ children... }
		</body>
	</html>
}
```

Create `internal/web/templates/public.templ`:

```go
package templates

import (
	"strings"

	"github.com/araihu/goshtoso/components/badge"
	searchfield "github.com/araihu/goshtoso/components/search"

	"github.com/araihu/manja/internal/core"
)

templ PublicDocs(idx core.SpecIndex) {
	@Layout(idx.Title) {
		<div class="mx-auto flex min-h-screen max-w-6xl flex-col gap-8 px-6 py-8">
			<header class="flex flex-col gap-4 border-b border-outline pb-6 dark:border-outline-dark md:flex-row md:items-center md:justify-between">
				<div>
					<p class="text-sm text-on-surface-muted dark:text-on-surface-dark-muted">OpenAPI docs</p>
					<h1 class="font-title text-3xl font-bold text-on-surface-strong dark:text-on-surface-dark-strong">{ idx.Title }</h1>
					<p class="mt-1 text-on-surface-muted dark:text-on-surface-dark-muted">Version { idx.Version }</p>
				</div>
				<div class="w-full md:w-80">
					@searchfield.Search(searchfield.Config{
						ID: "docs-search",
						Label: "Search docs",
						Placeholder: "Search operations and schemas",
						GlobalShortcut: true,
						Items: searchItems(idx.Search),
						MaxResults: 8,
					})
				</div>
			</header>
			<main class="grid gap-4">
				for _, op := range idx.Operations {
					<section id={ "operation-" + op.ID } class="rounded-radius border border-outline bg-surface p-4 dark:border-outline-dark dark:bg-surface-dark">
						<div class="flex flex-wrap items-center gap-3">
							@badge.Badge(badge.Config{Text: op.Method, Variant: badge.Primary})
							<h2 class="font-mono text-lg font-semibold text-on-surface-strong dark:text-on-surface-dark-strong">{ op.Path }</h2>
						</div>
						if op.Summary != "" {
							<p class="mt-3 text-on-surface dark:text-on-surface-dark">{ op.Summary }</p>
						}
						if len(op.Tags) > 0 {
							<p class="mt-2 text-sm text-on-surface-muted dark:text-on-surface-dark-muted">Tags: { strings.Join(op.Tags, ", ") }</p>
						}
					</section>
				}
			</main>
		</div>
	}
}

func searchItems(docs []core.SearchDocument) []searchfield.Item {
	items := make([]searchfield.Item, 0, len(docs))
	for _, doc := range docs {
		items = append(items, searchfield.Item{
			ID: doc.ID,
			Title: doc.Title,
			Description: doc.Description,
			Href: doc.Href,
			Section: doc.Section,
			Keywords: doc.Keywords,
		})
	}
	return items
}
```

Create `internal/web/static/manja.css`:

```css
.manja-markdown {
  color: var(--color-on-surface);
  line-height: 1.65;
}

.dark .manja-markdown {
  color: var(--color-on-surface-dark);
}

.manja-markdown h1,
.manja-markdown h2,
.manja-markdown h3 {
  color: var(--color-on-surface-strong);
  font-family: var(--font-title);
  font-weight: 700;
}

.dark .manja-markdown h1,
.dark .manja-markdown h2,
.dark .manja-markdown h3 {
  color: var(--color-on-surface-dark-strong);
}

.manja-markdown a {
  color: var(--color-primary);
  text-decoration: underline;
}

.dark .manja-markdown a {
  color: var(--color-primary-dark);
}
```

- [ ] **Step 4: Generate templ**

Run:

```bash
go run github.com/a-h/templ/cmd/templ generate
```

Expected: generated `_templ.go` files exist next to templates.

- [ ] **Step 5: Implement public server**

Create `internal/web/public.go`:

```go
package web

import (
	"net/http"

	"github.com/araihu/goshtoso/assets"
	"github.com/araihu/manja/internal/core"
	"github.com/araihu/manja/internal/web/templates"
)

func NewPublicServer(idx core.SpecIndex) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/assets/", assets.Handler())
	mux.Handle("/manja-assets/", http.StripPrefix("/manja-assets/", http.FileServer(http.Dir("internal/web/static"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_ = templates.PublicDocs(idx).Render(r.Context(), w)
	})
	return mux
}
```

Create `internal/web/server.go`:

```go
package web

import (
	"net/http"

	"github.com/araihu/manja/internal/core"
)

func NewServer(idx core.SpecIndex) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", NewPublicServer(idx))
	mux.Handle("/api/", NewAPIServer())
	return mux
}
```

- [ ] **Step 6: Wire executable with fixture data**

Create `internal/app/app.go`:

```go
package app

import (
	"context"
	"net/http"

	openapiadapter "github.com/araihu/manja/internal/adapters/openapi"
	sourceadapter "github.com/araihu/manja/internal/adapters/source"
	"github.com/araihu/manja/internal/web"
)

func New(ctx context.Context, specPath string) (http.Handler, error) {
	src := sourceadapter.File{Path: specPath}
	spec, rev, err := src.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	parser := openapiadapter.Parser{}
	idx, err := parser.Parse(ctx, spec, rev)
	if err != nil {
		return nil, err
	}
	return web.NewServer(idx), nil
}
```

Modify `cmd/manja/main.go`:

```go
package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/araihu/manja/internal/app"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	spec := flag.String("spec", "internal/adapters/openapi/testdata/petstore.yaml", "OpenAPI spec path")
	flag.Parse()

	handler, err := app.New(context.Background(), *spec)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("manja listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
```

- [ ] **Step 7: Verify**

Run:

```bash
go test ./internal/web -run TestPublicDocsRenderSearchAndOperations -v
go test ./...
go run ./cmd/manja -addr :8080
```

Expected: tests pass. Manual server starts and `/` renders Petstore docs.

- [ ] **Step 8: Commit**

```bash
git add internal/app internal/web cmd/manja/main.go
git commit -m "feat: render public docs"
```

## Task 8: Add File Source Adapter And End-To-End Local Sync

**Files:**
- Create: `internal/adapters/source/file.go`
- Test: `internal/adapters/source/file_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Write file source test**

Create `internal/adapters/source/file_test.go`:

```go
package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSourceFetchesSpec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "openapi.yaml")
	if err := os.WriteFile(path, []byte("openapi: 3.1.0\ninfo:\n  title: T\n  version: v1\npaths: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	src := File{Path: path}
	spec, rev, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != path || string(spec.Bytes) == "" {
		t.Fatalf("spec = %#v", spec)
	}
	if rev.Ref != "file" || rev.ID == "" {
		t.Fatalf("revision = %#v", rev)
	}
}
```

- [ ] **Step 2: Verify failure**

Run:

```bash
go test ./internal/adapters/source -v
```

Expected: fail because `File` does not exist.

- [ ] **Step 3: Implement file source**

Create `internal/adapters/source/file.go`:

```go
package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"

	"github.com/araihu/manja/internal/core"
)

type File struct {
	Path string
}

func (f File) Fetch(ctx context.Context) (core.SpecFile, core.Revision, error) {
	if err := ctx.Err(); err != nil {
		return core.SpecFile{}, core.Revision{}, err
	}
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return core.SpecFile{}, core.Revision{}, err
	}
	sum := sha256.Sum256(data)
	id := "file-" + hex.EncodeToString(sum[:])[:16]
	return core.SpecFile{Path: f.Path, Format: "yaml", Bytes: data}, core.Revision{ID: id, Ref: "file"}, nil
}
```

- [ ] **Step 4: Use file source in app wiring**

Modify `internal/app/app.go` so it uses `sourceadapter.File`:

```go
src := sourceadapter.File{Path: specPath}
spec, rev, err := src.Fetch(ctx)
if err != nil {
	return nil, err
}
idx, err := parser.Parse(ctx, spec, rev)
```

Add import:

```go
sourceadapter "github.com/araihu/manja/internal/adapters/source"
```

- [ ] **Step 5: Verify**

Run:

```bash
go test ./internal/adapters/source -v
go test ./...
```

Expected: tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/source internal/app/app.go
git commit -m "feat: add file source adapter"
```

## Task 9: Add Sitemap And SEO Metadata

**Files:**
- Modify: `internal/core/project.go`
- Modify: `internal/web/public.go`
- Test: `internal/web/seo_test.go`

- [ ] **Step 1: Write sitemap test**

Create `internal/web/seo_test.go`:

```go
package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/araihu/manja/internal/core"
)

func TestSitemapUsesPublicRoutes(t *testing.T) {
	idx := core.SpecIndex{
		Title: "Petstore",
		PublicRoutes: []core.PublicRoute{
			{Path: "/", Title: "Petstore"},
			{Path: "/operations/listPets", Title: "GET /pets"},
		},
	}
	srv := NewPublicServer(idx)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<urlset") || !strings.Contains(body, "/operations/listPets") {
		t.Fatalf("sitemap missing routes:\n%s", body)
	}
}
```

- [ ] **Step 2: Verify failure**

Run:

```bash
go test ./internal/web -run TestSitemapUsesPublicRoutes -v
```

Expected: fail because `/sitemap.xml` returns 404.

- [ ] **Step 3: Implement sitemap**

Modify `internal/web/public.go`:

```go
import (
	"encoding/xml"
	"net/http"
)

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc string `xml:"loc"`
}
```

Add route before `/`:

```go
mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/sitemap.xml" {
		http.NotFound(w, r)
		return
	}
	urls := make([]sitemapURL, 0, len(idx.PublicRoutes))
	for _, route := range idx.PublicRoutes {
		urls = append(urls, sitemapURL{Loc: route.Path})
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(sitemapURLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs: urls,
	})
})
```

- [ ] **Step 4: Verify**

Run:

```bash
go test ./internal/web -run TestSitemapUsesPublicRoutes -v
go test ./...
```

Expected: tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/web/public.go internal/web/seo_test.go
git commit -m "feat: expose public sitemap"
```

## Task 10: Add First Browser-Level Public Docs Test

**Files:**
- Create: `internal/web/e2e/public_docs_test.go`
- Modify: `go.mod`

- [ ] **Step 1: Add Playwright dependency**

Run:

```bash
go get github.com/playwright-community/playwright-go@latest
go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps chromium
```

- [ ] **Step 2: Write e2e test**

Create `internal/web/e2e/public_docs_test.go`:

```go
package e2e

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/playwright-community/playwright-go"

	"github.com/araihu/manja/internal/core"
	"github.com/araihu/manja/internal/web"
)

func TestPublicDocsSearchKeyboard(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	idx := core.SpecIndex{
		Title: "Petstore",
		Version: "1.0.0",
		Operations: []core.Operation{{ID: "listPets", Method: "GET", Path: "/pets", Summary: "List pets", Tags: []string{"Pets"}}},
		Search: []core.SearchDocument{{ID: "operation-listPets", Title: "GET /pets", Description: "List pets", Href: "#operation-listPets", Kind: "Operation", Section: "Pets"}},
	}
	server := httptestServer(t, web.NewPublicServer(idx))

	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()
	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(server); err != nil {
		t.Fatal(err)
	}
	if err := page.Keyboard().Press("Control+K"); err != nil {
		t.Fatal(err)
	}
	input := page.Locator("#docs-search-input")
	if err := input.Fill("pets"); err != nil {
		t.Fatal(err)
	}
	if err := page.Locator("#operation-listPets:visible").WaitFor(); err != nil {
		t.Fatal(err)
	}
}

func httptestServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	return "http://" + listener.Addr().String()
}
```

- [ ] **Step 3: Verify**

Run:

```bash
go test ./internal/web/e2e -run TestPublicDocsSearchKeyboard -v
go test ./...
```

Expected: e2e test passes locally when browser dependencies are available. In CI, keep this test enabled only if the runner can install Chromium.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum internal/web/e2e/public_docs_test.go
git commit -m "test: cover public docs search"
```

## Task 11: Add Testcontainers Integration Test Skeleton

**Files:**
- Create: `internal/integration/forgejo_test.go`
- Create: `internal/integration/dex_test.go`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add testcontainers dependency**

Run:

```bash
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/testcontainers/testcontainers-go/modules/forgejo@latest
```

Do not add the Dex module until its Go module release status is verified.

- [ ] **Step 2: Add Forgejo smoke test**

Create `internal/integration/forgejo_test.go`:

```go
//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/forgejo"
)

func TestForgejoContainerStarts(t *testing.T) {
	ctx := context.Background()
	c, err := forgejo.Run(ctx, "codeberg.org/forgejo/forgejo:11")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Terminate(ctx)
}
```

- [ ] **Step 3: Add Dex release guard**

Create `internal/integration/dex_test.go`:

```go
//go:build integration

package integration

import "testing"

func TestDexContainerPlanned(t *testing.T) {
	t.Skip("enable once testcontainers-go Dex module release status is verified")
}
```

- [ ] **Step 4: Add optional CI integration job**

Modify `.github/workflows/ci.yml` to add a separate job:

```yaml
  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go test -tags=integration ./internal/integration -v
```

- [ ] **Step 5: Verify**

Run:

```bash
go test ./...
go test -tags=integration ./internal/integration -run TestForgejoContainerStarts -v
```

Expected: unit tests pass. Integration test passes when Docker is available; otherwise report Docker availability as an environment blocker.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/integration .github/workflows/ci.yml
git commit -m "test: add integration test skeleton"
```

## Task 12: Final Verification And README

**Files:**
- Create: `README.md`
- Modify: `docs/superpowers/specs/2026-06-06-manja-design.md` only if implementation discoveries require a spec clarification.

- [ ] **Step 1: Add README**

Create `README.md`:

```markdown
# Manja

Manja is a hosted OpenAPI renderer and publisher built with
[Goshtoso](https://github.com/araihu/goshtoso).

The first vertical slice renders read-only OpenAPI docs from a spec file and
provides Ctrl+K search across indexed operations and schemas.

## Development

```bash
go test ./...
npm ci
npm run api:bundle
npm run api:lint
go run ./cmd/manja -spec internal/adapters/openapi/testdata/petstore.yaml
```

Open <http://localhost:8080>.
```

- [ ] **Step 2: Run full verification**

Run:

```bash
go mod tidy
npm run api:bundle
npm run api:lint
go test ./...
git diff --check
git status --short
```

Expected:

- API commands pass.
- Go tests pass.
- `git diff --check` is clean.
- Only intended files are modified.

- [ ] **Step 3: Commit**

```bash
git add README.md go.mod go.sum package-lock.json
git commit -m "docs: add Manja development guide"
```

- [ ] **Step 4: Push**

```bash
git push
```

Expected: branch pushes to `origin/main`.

## Self-Review Notes

Spec coverage:

- Hosted OpenAPI rendering: Tasks 4, 7, 10.
- Search-first indexing: Tasks 4, 7, 10.
- Hexagonal ports: Tasks 3, 5, 6, 8.
- Git/source abstraction: Task 8 starts with file source; Task 11 prepares provider-style Forgejo tests. Real git adapter is intentionally next-plan work.
- Publishing/visibility: Task 3 models publication visibility; UI workflows are next-plan work after the public read path exists.
- Storage without DB: Task 5.
- Markdown/Goldmark: Task 6.
- API-first only when needed: Task 2 creates only health.
- Testcontainers: Task 11.

Known follow-up plans:

- Git source adapter with local bare repo tests and Forgejo integration.
- Auth with Dex and project membership.
- Publishing management UI.
- SecretStore implementation.
- Goshtoso Markdown component evaluation.
