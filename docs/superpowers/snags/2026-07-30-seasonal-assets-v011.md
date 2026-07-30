# Manja seasonal Assets v0.1.1 adoption

## Boundary

- Task: downstream Assets adoption plan, Task 2 only.
- Base: `5d5005ec8b68a2f0783e624c123d1e69fa12e82d`.
- Branch: `codex/seasonal-assets-v011`.
- Worktree: `/tmp/manja-seasonal-assets-v011`.
- Assets source: released tag `github.com/araihu/assets@v0.1.1`, checked out
  detached at `74c36ed` and verified with the released CLI.
- Release-tracks worktree `/Users/guilhermecastro/.codex/worktrees/53ab/manja`
  remains paused and untouched.
- Dirty primary checkout and external `codex/v11-manja-assets` worktree remain
  untouched.

## Frozen contract

- Runtime: `https://araihu.com/assets/campaign/v1.js`.
- Channel: `https://araihu.com/assets/releases/current`.
- SRI:
  `sha384-oPH7l1vK9vKP1Dn+18sO3yEXlz4ts6KzPEQl0SW4Y/+im05gOaamNNaQAf6bGH/n`.
- Explicit `theme` preference wins.
- Only application-owned default Manja logo and favicon are managed.
- Spec/caller logo and favicon remain authoritative and unmarked.
- Runtime failure preserves the local Assets v0.1.1 fallback.
- Candidate may be committed and reviewed, but not merged or deployed until
  the Ahairu runtime and channel routes stop returning 404.

## Snags and debt

- The released `araihu-assets export` command requires a source checkout with
  `dist/`; invoking the tagged module from the Manja directory failed before
  writing. Resolution: clean detached v0.1.1 source worktree, `verify`, then
  generator-driven export to a temporary directory.
- Pre-change `GOWORK=off go test ./... -count=1` passed package tests through
  `internal/web` but its Chromium tail emitted no output for about 120 seconds.
  It was interrupted once under the user's test-less cadence. Receipt:
  <https://github.com/araihu/manja/issues/59>.
- Existing local `codex/v11-manja-assets@1e75163` is unmerged and overlaps
  asset paths. It is an external preserved worktree, not an input to this task.
- The Impeccable design hook reported existing typography/color literals while
  touched CSS files were inspected. None was introduced or changed by the
  seasonal slice, so no unrelated design token or suppression was added.

## Evidence

- `npm ci`: pass; existing 22 dependency findings unchanged.
- `go version`: `go1.26.5 darwin/arm64`.
- `site/`: `GOWORK=off go test ./... -count=1` pass.
- Assets v0.1.1: `go run ./cmd/araihu-assets verify` pass.
- Assets v0.1.1 export: 714 release files generated.
- Literal renderer RED before production edits:
  `GOWORK=off go test ./internal/web -run
  'TestPublicDocs(SeasonalAssetsManageOnlyApplicationDefaults|UseAssetsV011FallbackHashes)$'
  -count=1` failed because the root lacked `data-theme-source="default"` and
  the local favicon still had the pre-v0.1.1 digest.
- Literal product-site RED before production edits:
  `GOWORK=off go test ./internal/server -run
  '^TestSeasonalAssetsContractAndFallbacks$' -count=1` failed because the root
  lacked `data-theme-source="default"`.
- Focused renderer branding/seasonal tests: pass.
- Focused product-site route/static/seasonal tests: pass.
- Chromium forced-runtime-failure smoke at 390x844: pass; explicit `minimal`
  preference and local Manja logo/favicon survived HTTP 503, the campaign
  toggle remained hidden, and no page error occurred.
- Changed-package test, race, vet, and root/site `go mod tidy -diff`: pass.
- Second `templ generate`: zero updates and identical generated hashes.
- Live runtime, channel, and immutable v0.1.1 asset: GET and HEAD HTTP 200.
- Live runtime SHA-256:
  `a936193b4fed8120e6cb3423f19d3e2ddb0ba32266dc4e5f02a98f5261853709`;
  recomputed SHA-384 matches the frozen SRI.
