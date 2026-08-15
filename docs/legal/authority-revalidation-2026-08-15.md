# OC-01 authority and provenance revalidation receipt

Date: 2026-08-15

Result: **BLOCKED**

This receipt records mechanical identity and public-source observations. It is
not a copyright opinion, an assignment, trademark permission, or permission to
redistribute Manja. The external URLs below are the auditable references; fixed
upstream bytes also carry their observed size, Git blob, and SHA-256.

## Exact source freeze

The revalidation began from the clean isolated checkout below. Adding this
receipt changes the candidate tree, so the final commit and tree must be frozen
again before review or handoff.

- repository: `/Users/guilhermecastro/repos/araihu/manja`;
- ref: `refs/heads/codex/opencore-reconciler-next`;
- commit: `4fda300572506d12439a9427a4feb6976b0c4204`;
- tree: `15d215844b92e0c883a14a516a643832c5cac627`;
- parent: `8035a9faefa9b8ed45032375dd63d7cfd635cd1f`;
- `origin/main`: `507c5ea9fcdc8cee670a023dbb82f348ba2ed763`;
- merge-base with `origin/main`: `507c5ea9fcdc8cee670a023dbb82f348ba2ed763`;
- source worktree status: clean, with no staged, unstaged, or untracked files.

The source freeze has no root `LICENSE`, `NOTICE`,
`THIRD_PARTY_NOTICES.md`, or project SBOM. The current distribution gate must
remain blocked until first-party authority and final-artifact clearance are
separately established.

## First-party identity observations

| Reference | Observed bytes | Mechanical observation | Authority result |
| --- | ---: | --- | --- |
| [GitHub AraiHu organization API](https://api.github.com/orgs/araihu) | 1,176 bytes; response SHA-256 `255ffdc972b5c9ba5b3ef2995a9c372a43be2c6ecbafe63f7b2f84b3be5698b0` | `login=araihu`, `blog=araihu.com`, `location=Brazil`; the public organization page also identifies a verified domain and lists `manja` as public. | Identifies a public organization and domain only. It does not prove copyright ownership, employment assignment, or authority to license the complete repository. |
| [GitHub Manja repository API](https://api.github.com/repos/araihu/manja) | 6,322 bytes; response SHA-256 `0c2a9fd6552862864ec69f1f6beed082c5a92d1ca931c5a6b02d81302f91ef46` | `full_name=araihu/manja`, `visibility=public`, `default_branch=main`, `license=null` at observation time. | Public hosting and a null repository license do not establish a project license or redistribution authority. |

These observations do not clear first-party Go, JavaScript, templ, YAML, CSS,
documentation, generated inputs, product copy, social compositions, or
container material. The dated text formerly in
`rights-holder-confirmation.md` is not an executed assignment or an
independent rights-holder receipt.

## Reproducible upstream inputs

The local receipts remain the byte-level authority for the copied or generated
inputs. Each row binds repository, commit, tree, path, size, Git blob, and
SHA-256 where the input is reproducible. An upstream license label identifies
that upstream input only; it is not a Manja license claim.

| Input | Repository / commit / tree | Path and byte receipt | Upstream license observation | Clearance |
| --- | --- | --- | --- | --- |
| Kubernetes OpenAPI catalog | `https://github.com/kubernetes/kubernetes` / `a818af18fe29d999d6741234c8cd72709ef2f424` / `f2ff46de6b8cd5e93d196fa1f16a1b3737566dd0` | All 65 paths, sizes, Git blobs, and SHA-256 values are in [`receipt.json`](../../internal/renderer/testdata/kubernetes/receipt.json). The retained `LICENSE` is 11,358 bytes, blob `d645695673349e3947e8e5ae42332d0ac3164cd7`, SHA-256 `cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30`. | The fixed upstream [`LICENSE`](https://raw.githubusercontent.com/kubernetes/kubernetes/a818af18fe29d999d6741234c8cd72709ef2f424/LICENSE) is Apache License 2.0 for Kubernetes material. | Mechanical source/license identity is recorded. Final notice placement, first-party authority, and Kubernetes mark/social clearance remain **BLOCKED**. |
| GitHub REST fixture | `https://github.com/github/rest-api-description` / `6948cb04f5304188569c4bb4ae2190c08e7cbdba` / `6270ed1bd31a741adf3c7143c39d9bdc57d2fbc1` | `descriptions/ghes-3.0/ghes-3.0.json`, local path `internal/adapters/openapi/testdata/github-v3-rest.json`, 3,319,366 bytes, blob `f0ddf34ad4398c319db0643e45a0908ca026b382`, SHA-256 `dedfee9ad6a676c2f7186b8e2137d887d6449cad8b7af8253aecdaae24b27977`; receipt: [`github-v3-rest.provenance.json`](../../internal/adapters/openapi/testdata/github-v3-rest.provenance.json). | Same-revision `LICENSE.md`: 1,063 bytes, blob `b50625eb63949013cae604b1cadd42cfa1eaf825`, SHA-256 `3243761cbac07e6d169a5a2f4e7c25cc544da85248e735df74c3672e055cc87b`, SPDX `MIT`. | Mechanical identity is recorded. GitHub attribution/notice and trademark clearance remain **BLOCKED**. |
| Stripe OpenAPI input | `https://github.com/stripe/openapi` / `d70de345383dd818a0ce831f4e20d375c5a90cec` / `a7e155600c10dcfab91a94070b0e954419255862` | `openapi/spec3.json`, 3,840,021 bytes, blob `058edc82a247c71f05b94dfa6b9cef0a794a1358`, SHA-256 `8b608cba7129d121f12358a7092574e176833fe8cb4c9fcead178c71c545f870`; receipt: [`stripe-openapi.provenance.json`](../../internal/renderer/testdata/kubernetes/stripe-openapi.provenance.json). | Same-revision `LICENSE`: 1,095 bytes, blob `edf2d132d8bb95146e05585c3a782d059298b46b`, SHA-256 `8c1ce883f4eee7b531e0b7872dbfc72d410ced87dfff9501305de05ca8d203e5`, SPDX `MIT`. | Mechanical identity is recorded. Stripe attribution/notice and trademark clearance remain **BLOCKED**. |
| Simple Icons GitHub and Stripe marks | `https://github.com/simple-icons/simple-icons` / `fc91ef03ec113d06627b2d47c1f9644ca202b6f9` / `4c01339d8cafffdd7a6a59837b2fc0bbc5ad6e92` | Upstream `icons/github.svg`: 822 bytes, blob `538ec5bf2a9a5724899daf728577cd0b8beaae90`, SHA-256 `3bf8cceead820aec50d4ee825a3fd02c5a1cd6665cc9cf4cbf3d9c8861a204bb`; upstream `icons/stripe.svg`: 588 bytes, blob `8ebadf74d367a5a9bd7deb45a53f1844fc08a095`, SHA-256 `130c6d957b8977f5eda2928267b9df531ca038a400a801765d263801bb1bd870`. Local adapted paths and hashes are recorded in [`simple-icons.provenance.json`](../../internal/web/static/simple-icons.provenance.json). | Fixed upstream `LICENSE.md`: 6,569 bytes, blob `70d4a7b6740c5d9b594ff2fc27d3ea7e89413185`, SHA-256 `9046848b63a5c92bff14e4accca80bd987e0623b74adf9226ce5198d312b79d5`, SPDX `CC0-1.0`. The fixed [`DISCLAIMER.md`](https://raw.githubusercontent.com/simple-icons/simple-icons/fc91ef03ec113d06627b2d47c1f9644ca202b6f9/DISCLAIMER.md) is 4,604 bytes, blob `49f239e6400ca3d10932e34f5bf52a8f883f88e5`, SHA-256 `5757a1f28eff735a8b5e7425478f367812d71e974530f5bedd01219480965f4a`. | CC0 source terms do not waive or grant trademark rights. The official disclaimer directs users to seek the applicable brand permissions. GitHub/Stripe trademark permission and final NOTICE/attribution remain **BLOCKED**. |

## Local visual and social artifacts

| Local path | Source freeze byte receipt | Authority / redistribution result |
| --- | --- | --- |
| `internal/web/static/manja-social.svg` | 2,198 bytes, Git blob `5cb2fda632e511e4eeccec6412858f6f630bc6c9`, SHA-256 `002b05823a870f28ff28d12fe0b793cee979418435bb4ff4c4a634affc7b2fe2` | Source/output relationship is mechanically recorded, but complete first-party composition authority and portable lawful regeneration remain **BLOCKED**. |
| `internal/web/static/manja-social.png` | 21,500 bytes, Git blob `9260d190361cceeef611f3a2178f14c613b0f533`, SHA-256 `7234c9a20fc3a4a44364b8f9d544ddae5aba8c2b6a418b26ad5a930d2d0ab0bd` | Exact bytes are identified; font/generator rights and complete composition authority remain **BLOCKED**. |
| `internal/web/static/kubernetes-social.png` | 48,705 bytes, Git blob `b6f7d948c54b780dee14b1b4c80734d4859a73e4`, SHA-256 `a7cf0baba81cf79fdbe8a0487bd30ed1b6a34dc816ec8345a50591a99a2db423` | No editable source, generator receipt, or Kubernetes visual/trademark clearance is recorded; **BLOCKED**. |

The three unresolved logo concepts under
`docs/brand/logo-concepts/2026-06-08/` have no verified generator, account,
model, generation terms, or redistribution decision. They remain **BLOCKED**
for source distribution.

## Decision by claim type

| Claim type | Current result | Evidence needed to unblock |
| --- | --- | --- |
| Mechanical identification | Partially resolved for the pinned upstream and local receipts above. | Complete final-artifact inventory, including derived renderer data and copied/generated bytes. |
| First-party authority | **BLOCKED**. Git authorship, a public organization, and a maintainer address are not assignment evidence. | Dated written authority naming the rights holder for the complete first-party work and resolving employment/assignment rights. |
| Trademark permission | **BLOCKED**. Simple Icons source terms do not grant brand permission. | Permission or documented policy for each used mark and social composition, including Kubernetes, GitHub, and Stripe. |
| Attribution / NOTICE | **BLOCKED**. Upstream license bytes are identified, but no reviewed final placement exists. | Byte-bound attribution and notice mapping for every shipped artifact. |
| Redistribution clearance | **BLOCKED**. No complete, digest-bound source/binary/site/OCI release inspection is recorded. | Independent authority plus final archive/site/OCI inspection and matching SBOM/notice evidence. |

No root `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`, Apache metadata, or
production SBOM is created by this receipt.
