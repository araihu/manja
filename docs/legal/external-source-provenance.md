# External source provenance receipt

Date: 2026-08-15

Status: **MECHANICAL EVIDENCE RECORDED**

This bounded receipt records immutable source identities and byte-level
observations for the Arai Hû Assets release consumed by Manja and the pinned
Goshtoso Go module. The machine-readable values are in
[`external-source-provenance.json`](external-source-provenance.json), and
`architecture/external_source_provenance_test.go` verifies the receipt against
the pinned asset manifest, installed asset bytes, and downloaded module license
bytes.

## Frozen external identities

| Source | Consumer identity | Legal bytes recorded |
| --- | --- | --- |
| [`araihu/assets`](https://github.com/araihu/assets) | `origin/main` commit `9a1fce17ad1a99892e81bf3b3b36e7ed48448b63`, tree `2be5242f515b452052e514c8dd495a95791e5925`; consumed release `v0.2.1` commit `fdfb1c2aad8fa61779e7b8c6f208e52a6cf825ce`, tree `e89dce3f6bbb129bf3dfd3f04d874cdb220bb4a0` | `LICENSE`: Apache-2.0, 11,358 bytes, Git blob `d645695673349e3947e8e5ae42332d0ac3164cd7`, SHA-256 `cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30`; `NOTICE`: 633 bytes, Git blob `61949028cb73348c69d488a844a36fb6253b46b7`, SHA-256 `8ab9628587c91891e424abdc1c16fdd8d9a89191e56fe496dadc979994bd6366`. |
| [`araihu/goshtoso`](https://github.com/araihu/goshtoso) | `origin/main` commit `d3eaf0d19be3dcae3fe0fab688c6fe915bc3abd6`, tree `fa6b7ed44ab6be7ec02c36783f80a08b72dd1769`; Manja dependency `v0.2.6` commit `ed9750d2443ab5e961aec50b546dca4f3b033d62`, tree `7b5a8fc3a03196f08db0b1209dcdd43b521d942c` | `LICENSE`: MIT, 1,078 bytes, Git blob `0a7743398ecbeacc05ed822e1f74023ee9b36842`, SHA-256 `cacf68ff9920c026f5de2ebf992333c1a243e45d81aaa5b4577e05b52c5a9584`. |

The Assets release archive is bound by the existing
[`araihu-assets.json`](../../araihu-assets.json) identity:
`https://github.com/araihu/assets/releases/download/v0.2.1/araihu-assets-v0.2.1.tar.gz`,
archive SHA-256
`818a32246c040871c8f28bb085269b6b9f21c579b18dc4c3c1f20d70716eaf70`, and
`release.json` SHA-256
`1e071ba6d88efa862b6166820bdc759c7edb917c8566ce7111358c5c3dc2714e`.

The four source paths selected by the manifest are recorded with their Git
path, Git blob, size, SHA-256, and every installed Manja destination in the
JSON receipt. The tests compare all installed copies byte-for-byte by size,
Git blob SHA-1, and SHA-256. No mutable branch, local dirty checkout, or
unverified path is a source authority.

## Mechanical evidence versus legal clearance

The receipt proves which immutable upstream bytes were observed and which
local bytes match them. It does not itself grant a license, establish a rights
holder, or decide trademark permission.

Task authority states that Manja's first-party project license is MIT. This
checkpoint records that statement as scope context only; it does not create or
edit the root `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, SBOM, or package
layout. Those artifacts remain owned by OC-02.

The Assets `NOTICE` text identifies Arai Hû project use and redistribution, and
the Assets `LICENSE` identifies Apache-2.0 for that repository. The Goshtoso
module `LICENSE` identifies MIT for Goshtoso. These are upstream observations,
not a conclusion that every Manja artifact is cleared for redistribution or
that any Arai Hû, Manja, Goshtoso, or third-party mark may imply endorsement.
Final attribution placement, first-party authority, and trademark review stay
separate gates.

No Goshtoso source, generated output, legal file, or asset is copied by this
checkpoint. Runtime and browser redistribution scope still requires final
artifact inspection and the OC-02 notice/SBOM gate.
