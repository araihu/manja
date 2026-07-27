# Manja Goshtoso Application Structure Snag Ledger

Date: 2026-07-27
Scope: public documentation and management application-structure slice

Record every Goshtoso source dive, missing public API, CSS escape hatch,
generation slowdown, or workaround encountered during this slice. Public
consumer guidance and exported APIs are the authority; this ledger does not
authorize edits to Goshtoso or access to private/generated upstream output.

## 2026-07-27 — dependency-sensitive module gates

- Desired contract: resolve Manja and every nested consumer exactly as CI and a
  fresh external consumer resolve them.
- Public source consulted: `AGENTS.md`, the approved implementation plan, and
  the installed `using-goshtoso` consumer skill.
- Source dive or missing API: the primary checkout has an ignored, user-owned
  `go.work` using Go 1.26.1. It can silently override the tagged Goshtoso
  dependency and the required Go 1.26.5 floor.
- Workaround or no-match decision: do not edit or copy that workspace file.
  Use `GOWORK=off` for dependency-sensitive root, `site/`, and external-module
  gates as appropriate, and verify the resolved module plus Go version
  explicitly.
- Risk: a locally green build could consume the wrong Goshtoso checkout or Go
  toolchain and fail in CI or for external consumers.
- Upstream feedback candidate: none; this is a downstream workspace-isolation
  requirement.

## 2026-07-27 — immutable full dependency fallback manifest

- Desired contract: a public immutable ordered manifest for the full
  `head.Dependencies()` fallback stack, paired with an exact library-version
  API for the approved future hybrid design.
- Public source consulted: Goshtoso v0.0.13 `using-goshtoso` guidance,
  `components-reference.md`, and the frozen head-dependency fallback audit.
- Source dive or missing API: Goshtoso v0.0.13 does not expose the complete
  immutable ordered fallback manifest plus exact library-version API required
  by that later design.
- Workaround or no-match decision: treat this as a downstream blocker for the
  later hybrid design. Preserve the current public `head.Dependencies()`
  contract; do not scrape private or generated output and do not edit upstream
  Goshtoso in this UI slice.
- Risk: attempting the later hybrid design now would couple Manja to private
  implementation details and break versioned fallback integrity.
- Upstream feedback candidate: expose a documented immutable full dependency
  manifest and exact library-version API in a separately reviewed Goshtoso
  release.
