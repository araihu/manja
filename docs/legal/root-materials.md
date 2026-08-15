# Root legal materials

Date: 2026-08-15

Status: **BLOCKED** for redistribution. The repository now carries the
project's user-authorized MIT license label, a project notice, and an explicit
third-party notice record. These files are evidence inputs, not a clearance
decision. No first-party Manja copyright holder or year is asserted because
the local evidence does not establish either fact.

## Root files

`LICENSE`, `NOTICE`, and `THIRD_PARTY_NOTICES.md` are regular, non-empty,
tracked root files. `internal/distribution.LoadRepositoryLegalEvidence` reads
those exact paths, binds their bytes to SHA-256 file evidence, rejects missing,
symlinked, non-regular, or empty files, and intentionally leaves `Holder` and
`YearRange` empty. `Evaluate` therefore remains fail-closed until an
independent authority receipt supplies the missing first-party attribution
and any other required clearance evidence.

The root `LICENSE` records the MIT project-license fact provisionally without
manufacturing a holder or year. It contains no operative permission grant while
authority and redistribution remain blocked. `NOTICE` identifies the same
unresolved first-party boundary and grants no project-name, logo, mark, or
trade-dress permission. Neither file changes the distribution gate's
requirement for independent authority and attribution evidence.

## Upstream source records

`THIRD_PARTY_NOTICES.md` records only material verified from the following
immutable local sources:

- Arai Hû Assets release `v0.2.1`, revision
  `fdfb1c2aad8fa61779e7b8c6f208e52a6cf825ce`, release SHA-256
  `818a32246c040871c8f28bb085269b6b9f21c579b18dc4c3c1f20d70716eaf70`,
  source tree `e89dce3f6bbb129bf3dfd3f04d874cdb220bb4a0`, with upstream
  `LICENSE`/`NOTICE` blob and SHA-256 receipts recorded in that file. The same
  blobs were independently checked at upstream `origin/main` commit
  `9a1fce17ad1a99892e81bf3b3b36e7ed48448b63`, tree
  `2be5242f515b452052e514c8dd495a95791e5925`;
- Goshtoso `v0.1.13`, commit
  `19c86f1dbbcf5a85c55f2d9b3bfaac4fd5febea6`, source tree
  `49c9f5557ae5bb74a8f333af8958bbf495377af0`, with its upstream MIT
  `LICENSE` blob and SHA-256 receipt recorded in that file. The same blob was
  independently checked at upstream `origin/main` commit
  `78921015c2f3b46379ac30d3d9f80755bb860307`, tree
  `87d6cac8b9aa732b27976c32851d28fd430475af`.

Those upstream holder/year statements apply only to their respective upstream
projects. They are not Manja attribution, and their license or notice bytes
do not resolve the separate Kubernetes, GitHub, Stripe, Simple Icons, Muamba,
generated-asset, container/base, or trademark blockers listed in
`THIRD_PARTY_NOTICES.md`.

## Packaging boundary

The existing deterministic `Pack` and `GenerateSBOM` APIs are the only package
integration claimed here. They accept caller-supplied, digest-bound legal and
dependency evidence and refuse to package before authority and inventory
checks pass. This checkpoint does not add a root SBOM: the dependency graph,
final artifact inventories, and unresolved third-party/trademark evidence are
not complete enough to make one truthful. A passing gate still requires the
final artifact to contain the required legal paths and independently reviewed
authority receipts.
