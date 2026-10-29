# Root legal materials

> Historical checkpoint: [OCI publication](oci-publication.md) records the
> maintainer-confirmed MIT attribution and current packaging policy from
> 2026-10-25. The provisional-file and blanket-block statements below describe
> the August checkpoint. The generic evidence API retains its own checks.

Date: 2026-08-15

Asset release evidence updated: 2026-10-25. The original checkpoint and
remaining clearance decisions below retain their original scope.

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

- Arai Hû Assets release `v0.2.6`, revision
  `96c81116e6610a882f89cca2b40b92b20d59fbb0`, release SHA-256
  `2d86ebd0c0724e4293175186e80e7108fd8e813ed885f66bb192bc871b64d16b`,
  source tree `25fcd034e116990f439eecab04bdcc94bcbceea4`, with upstream
  `LICENSE`/`NOTICE` blob and SHA-256 receipts recorded in that file. The same
  blobs were independently checked at upstream `origin/main` commit
  `96c81116e6610a882f89cca2b40b92b20d59fbb0`, tree
  `25fcd034e116990f439eecab04bdcc94bcbceea4`;
- Goshtoso `v0.4.4`, commit
  `234eaaf97a7058d137dfed942423dcfb77f2ace6`, source tree
  `0242bf344b601a019a10b3bc08fc11364f8c4907`, with its upstream MIT
  `LICENSE` blob and SHA-256 receipt recorded in that file. The same blob was
  independently checked at upstream `origin/main` commit
  `234eaaf97a7058d137dfed942423dcfb77f2ace6`, tree
  `0242bf344b601a019a10b3bc08fc11364f8c4907`.

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
