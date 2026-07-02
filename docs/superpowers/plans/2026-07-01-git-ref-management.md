# Git Ref Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Let Manja managers discover Git branches/tags, sync a selected ref, and optionally publish that synced revision.

**Architecture:** Add a core `RevisionCandidate` plus optional discovery port. Implement Git discovery in `internal/adapters/source`, wire sync callbacks from `internal/app` into `internal/web`, and render a server-side management form that posts selected refs through the existing sync pipeline.

**Tech Stack:** Go, templ, Goshtoso components, filesystem store, existing Git CLI adapter.

## Global Constraints

- Work in `/tmp/manja-git-ref-management` on branch `codex/git-ref-management`.
- Keep management UI server-rendered HTML; do not add a REST API.
- Keep publishing explicit; syncing a ref does not publish unless requested in the form.
- Do not hand-edit generated `*_templ.go`; edit `.templ` and run `go run github.com/a-h/templ/cmd/templ generate`.
- Use TDD: write each failing test before production code.

---

### Task 1: Git Ref Discovery

**Files:**
- Modify: `internal/core/spec.go`
- Modify: `internal/core/ports.go`
- Modify: `internal/adapters/source/git.go`
- Test: `internal/adapters/source/git_test.go`

**Interfaces:**
- Produces: `core.RevisionCandidate{SourceID, Ref, Kind, CommitSHA string}`
- Produces: `core.SourceDiscoverer` with `Discover(context.Context) ([]RevisionCandidate, error)`
- Produces: `source.Git.Discover(ctx context.Context) ([]core.RevisionCandidate, error)`

- [x] **Step 1: Write failing discovery test**
- [x] **Step 2: Run `go test ./internal/adapters/source -run TestGitSourceDiscoversBranchAndTagRefs -count=1` and confirm compile/fail**
- [x] **Step 3: Add core model/port and Git discovery implementation**
- [x] **Step 4: Re-run source adapter tests**

### Task 2: Management Sync Action

**Files:**
- Modify: `internal/web/management.go`
- Modify: `internal/web/templates/management.templ`
- Test: `internal/web/management_test.go`

**Interfaces:**
- Consumes: `core.RevisionCandidate`
- Produces: `web.ManagementSyncAction func(context.Context, ManagedSpec, string) (ManagedSpec, error)`
- Adds route: `POST /manage/sync`

- [x] **Step 1: Write failing management test for rendering candidates and syncing a selected ref**
- [x] **Step 2: Run `go test ./internal/web -run TestManagementSyncPostSyncsSelectedGitRefAndCanPublish -count=1` and confirm compile/fail**
- [x] **Step 3: Add candidates to managed spec models, sync route, validation, and template form**
- [x] **Step 4: Run `go run github.com/a-h/templ/cmd/templ generate`**
- [x] **Step 5: Re-run management tests**

### Task 3: App Wiring

**Files:**
- Modify: `internal/app/app.go`
- Test: `internal/app/app_test.go`

**Interfaces:**
- Consumes: `source.Git.Discover`
- Wires management sync action by constructing a per-ref Git fetcher and running `core.Syncer.Sync`.

- [x] **Step 1: Write failing app test showing `/manage` lists Git branch/tag candidates and `/manage/sync` publishes selected ref**
- [x] **Step 2: Run `go test ./internal/app -run TestNewWithOptionsManagesGitRefDiscoveryAndSyncPublication -count=1` and confirm fail**
- [x] **Step 3: Wire Git discoverer and management sync callback in `NewWithOptions`**
- [x] **Step 4: Re-run app tests**

### Task 4: Verification

**Files:**
- Generated: `internal/web/templates/management_templ.go`

- [x] **Step 1: Run `go test ./internal/adapters/source ./internal/app ./internal/web`**
- [x] **Step 2: Run `go test ./...`**
- [x] **Step 3: Run `git diff --check`**
- [x] **Step 4: Review `git status --short` and summarize changed files**
