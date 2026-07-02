# Git Ref Management Design

## Goal

Add a narrow management workflow for Git-backed specs that discovers branch and tag candidates, lets a manager sync one candidate, and keeps publication explicit.

## Scope

This slice covers provider-agnostic Git refs only. Manja should list local or remote Git branches and tags for an already configured Git source, show those candidates in `/manage`, and allow a selected ref to be synced through the same OpenAPI parse/index/store path used at startup.

Publishing remains intentional. Syncing a ref updates the managed spec state but does not automatically make it public unless the form explicitly asks to publish that synced revision with a public path.

## Architecture

Core gets a small discovery model and optional port instead of expanding `SourceFetcher`. Git implements the discovery port with `git for-each-ref`, reusing the existing clone/auth/SSH behavior. Management gets an optional sync action callback so `internal/web` stays HTTP/form oriented while `internal/app` owns source/parser/store orchestration.

The management page renders discovered candidates as selectable refs for Git sources. A POST to `/manage/sync` validates the managed spec and ref, calls the sync action, updates in-memory management state, and optionally saves a public publication for the new revision.

## Non-Goals

- No source creation UI.
- No credential creation UI.
- No webhooks or polling.
- No new REST API contract.
- No server-side upstream API proxy or Try It surface.

## Error Handling

Discovery errors are shown to managers as management sync/source state, not to anonymous public docs. Sync errors return HTTP 400 from the management action and preserve the current managed spec state. Publication path validation continues to use the existing store validation.

## Tests

Add source adapter tests for branch/tag discovery ordering and commit SHAs. Add management tests for rendering candidates and syncing/publishing a chosen ref. Add app tests for wiring Git discovery and sync callbacks into management.
