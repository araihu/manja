# Manja Open Core Product Goal

Updated: 2026-08-12

## Objective

Deliver Manja as a complete Open Core, self-hosted OpenAPI renderer and
publisher. Coordinate three independent Codex tasks: this product-management
task, one implementation task, and one read-only review task.

This file is the repository-visible recovery summary. External control-plane
YAML remains the lifecycle and ownership authority. Machine-specific worktree
paths stay in that external ledger rather than this committed record.

## Product Boundary

Open Core includes:

- self-hosted renderer and catalog;
- offline `manja check` workflows;
- public `domain`, `application`, `application/port`, and `contracttest` APIs;
- self-hosted source, storage, rendering, and composition adapters;
- provenance, licensing, notices, SBOMs, reproducible packages, and operator
  documentation;
- hybrid SSR/Wasm public documentation, Service Worker, offline storage,
  recovery, rollback, performance gates, and kill switch.

Deferred hosted SaaS scope includes:

- hosted accounts, tenants, billing, entitlements, or marketplaces;
- hosted authentication and multi-user management;
- connected GitHub review as a hosted service;
- hosted release promotion, authenticated preview, and publication control.

Provider-neutral seams may support later hosted products. Open Core work must
not implement hosted product behavior speculatively.

## Lanes

### Product manager and integration coordinator

- Task: `019fef00-0495-7841-b442-031451ebb185`
- Branch: `coord/opencore-product`
- Worktree label: product-manager integration worktree
- Base: `39d65ade21c080ee2102f53da5ed741f000d6dd7`
- Owns scope, priorities, checkpoint acceptance, integration order, PR state,
  CodeQL/CodeRabbit triage, and durable recovery updates.

### Developer

- Task: `019fef17-2fad-73d2-b004-d3706d36ea82`
- Worktree label: dedicated Open Core developer worktree
- Initial lane base: `39d65ade21c080ee2102f53da5ed741f000d6dd7`
- OC-04H accepted direct parent:
  `fb9381f4b04821f322d128da3c7a50ce827af7d6`
- Current checkpoint base tree:
  `35fa43dfc56375e8af37b52438614ea4b3d3260e`
- Fresh-main merge-base:
  `43f96dfbf9d18eee2364f14778e6b94312c8abac`, tree
  `959f199e145c316b4b76e40a561413c0e6d57134`
- Branch: `codex/oc04h-request-body-media-summary`
- Worktree label: OC-04H request-body media-summary worktree
- Owns only bounded Open Core implementation checkpoints assigned here.
- Must commit each coherent checkpoint with a meaningful message.
- Must not implement deferred SaaS behavior or edit the active Arai Hû theme
  rollout.

### Independent reviewer

- Task: `019fef17-2faa-7620-b95c-ba6dc0343094`
- Worktree label: independent read-only review worktree
- Base: `39d65ade21c080ee2102f53da5ed741f000d6dd7`
- Read-only unless the product manager explicitly assigns a separate correction
  checkpoint.
- Reviews exact base/head identities, behavior, tests, scope, provenance, and
  Open Core boundary independently from developer conclusions.

## Operating Rules

1. Branch every unit from fetched, verified `origin/main` in an isolated
   worktree. Preserve the shared primary checkout and unrelated worktrees.
2. Developer commits coherent checkpoints often. Product manager integrates
   accepted checkpoints often; do not accumulate one massive merge block.
3. Freeze base, head, tree, status, and test evidence for every review.
4. Reviewer verdict binds one exact candidate identity. Any correction requires
   a fresh review of the new identity.
5. Push, PR, merge, release, publication, deployment, and cleanup remain
   separate lifecycle actions. Perform only actions explicitly in this goal or
   later authorized by the user.
6. PR must be checked for CodeQL and CodeRabbit presence. Investigate findings
   against current behavior. Fix findings that materially apply; record why
   false positives or irrelevant suggestions do not justify code changes.
7. Record Goshtoso source lookup, generated templ friction, component gaps, or
   dependency friction in the applicable snag ledger.
8. Keep this file current after every accepted checkpoint, lane replacement,
   PR transition, or important blocker.

## Current Evidence

- Audited OC-01 and PR #94 base:
  `39d65ade21c080ee2102f53da5ed741f000d6dd7`.
- Main CI and CodeQL passed for that audited-base identity.
- At the `2026-08-11T18:54:35Z` observation, PR #94 is `MERGED` via squash at
  `9733949b8dde9eb0fe1ef285ceb3ecbeb88f5d06`; fetched `origin/main` is that exact
  commit with tree `10c75d6c0fa3e945093bc47e46df0d33e0ad40de`.
- Local PM ref and worktree `coord/opencore-product` remain preserved at exact
  `d520edd5b6115904fbed247729dcf997716e2d03`, tree
  `eb9c05086b0f6d0380fc5da593ef96c8e3febd2d`. At the
  `2026-08-11T19:02:25Z` observation, remote
  `refs/heads/coord/opencore-product` is absent after the authorized squash
  merge. It is not recreated or pushed by this checkpoint.
- The fresh-main Stripe captured-byte checkpoint uses exact base and merge-base
  `9733949b8dde9eb0fe1ef285ceb3ecbeb88f5d06`, tree
  `10c75d6c0fa3e945093bc47e46df0d33e0ad40de`. Accepted local implementation head
  `b4d7562e7a97903726234a3d8d8a8d2130bf4481`, tree
  `ef6f22e9842d233ccde7acfd93c1563941d1290a`, received independent `ACCEPT`
  with no findings. Accepted ledger head
  `2ba674e4b7b8000a32ea47ce8896b9d741681b7f`, tree
  `b86fcf32d882d5baefd8ced024da2cd261ade191`, accurately distinguishes preserved
  local PM state from the absent remote PM ref. Authorized push opened [PR
  #96](https://github.com/araihu/manja/pull/96) at that exact ledger head against
  base `main` `9733949b8dde9eb0fe1ef285ceb3ecbeb88f5d06`.
- PR #96 was squash-merged with authorization at `2026-08-12T02:12:14Z`.
  Exact source head was `ad9d114ede2fe8b5337331eee8268e830a0e4577`;
  merge commit is `43f96dfbf9d18eee2364f14778e6b94312c8abac`, tree
  `959f199e145c316b4b76e40a561413c0e6d57134`, with sole parent/base
  `9733949b8dde9eb0fe1ef285ceb3ecbeb88f5d06`.
- Exact-source-head CI run `31552796911` succeeded on authorized retry attempt
  2: `test` and `integration` passed; image publication and deployment were
  skipped. CodeQL run `31552795037` succeeded for actions, Go,
  JavaScript/TypeScript, and summary. Exact-head CodeRabbit run
  `b7c5b517-c405-4711-8edd-b9be54a0b92d` generated no actionable comments, and
  independent final review returned `ACCEPT` with no findings.
- Accepted post-merge goal-only checkpoint
  `3d308ae3fc17a8ebc10cfe953c57d0651df9887c`, tree
  `66487fb7eb2bf98c0fc6b4dd1a0aa3831ba59bb1`, remains clean and preserved on
  its local branch. It is not integrated, pushed, or rewritten by this
  checkpoint.
- The Stripe integrity activation checkpoint starts from exact fresh
  `origin/main`
  `43f96dfbf9d18eee2364f14778e6b94312c8abac`, tree
  `959f199e145c316b4b76e40a561413c0e6d57134`. It activates the existing
  provider-neutral Git integrity receipt seam for the real Stripe renderer
  build source. Exact local candidate
  `dab2b8f24552f89cc2b1d705181a1ac4a2b97847`, tree
  `1abee072de1fce813731bfc3e02706c31cd0eb6d`, received independent `ACCEPT`
  with no findings from both developer and designer reviewers and remains
  clean, preserved, unpushed, and unintegrated.
- OC-01M7 starts from that accepted Stripe candidate as its direct parent; its
  merge-base remains fresh `origin/main`
  `43f96dfbf9d18eee2364f14778e6b94312c8abac`. First coherent implementation
  commit `e342c68fff9abc9be1d744da8fb7c3c73f6e6c81`, tree
  `f1aba174a207b6db02afbcee6a3f9892d18f14fa`, adds initial-HTML metadata for
  the product homepage and `/docs`, plus an exact product-site route to the
  existing embedded Manja preview bytes. Exact final head
  `fd2f16f36a423a8fea2f4273695aaad39525614f`, tree
  `dc411a51a9e6674c21f5bfc187d455ecc26c9f26`, received independent `ACCEPT`
  from technical and design reviewers and remains clean, preserved, unpushed,
  and unintegrated.
- OC-01M8 starts from that accepted OC-01M7 head as its exact direct parent.
  Implementation commit `5e0b3cfd3f2e4f79a84fb81c072f0b016db06b16`,
  tree `61e5620116aacdd3c19f4694fa079bd3e57fc7b3`, adds the complete initial-HTML
  social metadata contract to the standalone root public-docs overview,
  operation, and schema routes. Technical review rejected exact head
  `8f0dda961aac098f4eda2395f9648b8ba67eab85`, tree
  `7ee2c647e3a9e8fa0f7c593359b097b8c1b5ff8c`, for three material findings:
  request-Host authority, loss of non-root publication paths, and divergent
  raw-selection canonicals. Design review accepted the bounded design scope.
  Corrective commits `63f6edf2f66fbf31d016c07dc611c31146eae1f6`
  and `a5e88520c153673f34b58963b3051773817e4711` now require an explicit validated
  public origin for remote production metadata, keep loopback HTTP development
  explicit, preserve the resolver-approved external publication path, and
  resolve selection once before metadata and rendering. The existing approved
  Manja preview asset and rendered plain-text descriptions remain unchanged.
  Root tests, E2E, affected race/vet, Muamba, webassets, templ drift, preview
  bytes, and adversarial HTTP receipts passed locally. Exact correction head
  `cb3c2ac9df78482472a3929476ec77e0a885a2f1`, tree
  `9011e5def2caf30c40f18c9737f43bb2b2d8976c`, received independent `ACCEPT`
  from technical and design reviewers and remains clean, preserved, unpushed,
  and unintegrated.
- OC-01M9 starts from that accepted OC-01M8 head as its exact direct parent.
  Implementation commit `d1b464b23bd47b45dfe537b4c7333562f35a0684`
  restores a keyboard-only focus indicator on the catalog-search combobox with
  the existing `primary` and `primary-dark` semantic tokens. A rendered-template
  regression rejects missing focus-visible or layout-shifting focus classes;
  the focused browser test proves `Ctrl+K` focus, a solid 2 px outline with 2 px
  offset, token-color binding, and at least 3:1 contrast in both palettes. Full
  root tests passed, including E2E in 195.961s; focused template, catalog web,
  race, vet, Muamba, templ-drift, and catalog E2E gates also passed. Exact head
  `57643af0bae0727bf2b9e78622130fb1aa6e26ec`, tree
  `ec973970d9545d7a7e3b20ea0bfc9d4d22938d19`, received independent `ACCEPT`
  from technical and design reviewers and remains the immutable base for
  OC-04A.
- OC-04A starts from accepted OC-01M9 exact head
  `57643af0bae0727bf2b9e78622130fb1aa6e26ec`, tree
  `ec973970d9545d7a7e3b20ea0bfc9d4d22938d19`. Local implementation commit
  `4b21788de6ecdcadfab2e9643b7a5f0adae090ab`, tree
  `7056f2c8cdbab86e2209e0362b86e7510ae7a5aa`, exposes only declared immutable
  `detail` and `schema-node` children under snapshot-scoped
  `projection-data` routes. GET, HEAD, conditional 304, exact size/SHA-256/ETag,
  immutable cache headers, root/nested mounts, prefix-plus-kind allowlisting,
  path rejection, and changed/unreadable child failure are covered offline.
  Initial SSR HTML remains byte-identical to the accepted OC-01M9 fixture and
  contains no local-runtime activation marker. No descriptor, public-eligibility
  decision, Wasm renderer, Service Worker, offline storage, rollback, tombstone,
  kill switch, UI, Docker, SaaS, theme, legal, or Task 8 behavior is added.
  Goal-only child `05fcf118d9eb7fca94ae9a587f2776771f4678ef`, tree
  `9e806e77adbf3fb1b007925eeb6a2fd497fe90dd`, records the frozen OC-04A
  receipts and received independent `ACCEPT`; it is the immutable direct parent
  for OC-04B and remains unpushed and unintegrated.
- OC-04B implementation commit
  `66f259b90a424f6934ce539e88e39c6e91698162`, tree
  `c213e73eec2192cd53230cdc6939d71e5c44c2d5`, admits an inert enhancement
  descriptor only when composition explicitly declares the catalog public and
  anonymous with a stable publication key. Before emission, the server requires
  exact manifest/snapshot identity, recomputed lowercase SHA-256, canonical
  revision, `projection-v2`, source digest, catalog identity, and identical
  transport/identity child inventories. `MANJA_LOCAL_DOCS=off` is parsed
  strictly by self-hosted runtime composition and removes the descriptor while
  preserving byte-identical SSR/no-JS HTML and the OC-04A transport. Root tests
  passed, including self-hosted and E2E in 187.067s and 186.957s; affected full,
  race, vet, strict Muamba, templ regeneration, and diff checks also passed.
  No Wasm renderer, Service Worker, offline storage, rollback, tombstone, UI,
  Docker, SaaS, theme, legal, packaging, or Task 8 behavior is added. The final
  moving checkpoint identity is supplied by the external review packet to avoid
  recursive self-reference.
- Goal-only child `f02469c39e09576963b9de4d98807abfc542b792`, tree
  `f6400357ebfefcc4571b4bd3246a3176df797e99`, was independently rejected only
  for one technical P1: `publicationKey` used an over-broad canonical string
  validator and eligible catalogs could share a cache namespace. The rejected
  parent and its branch remain unchanged.
- Correction implementation commit
  `366b95530b10f6d79346b5485dc27be13bd56b96`, tree
  `ca7b646b48fb7cef6e2a3226865dca336c944082`, binds every eligible
  `publicationKey` to 1-64 lowercase ASCII bytes with alphanumeric endpoints
  and `[a-z0-9._-]` interior characters. Renderer configuration rejects
  duplicate eligible keys before server creation. Direct web policy admission
  disables enhancement globally on invalid or duplicate keys, preserving SSR
  HTTP 200 with no descriptor and no HTTP 500. Distinct valid keys remain
  independently addressable. Focused RED reproduced all four accepted-invalid
  cases; GREEN, affected full packages, race, vet, strict Muamba, diff check,
  and root `go test ./... -count=1` passed. Root receipts include self-hosted
  202.962s and E2E 189.132s. No other OC-04B/OC-04A behavior or scope changed;
  final moving identity remains external for fresh review.
- OC-04B correction child
  `c7a3e89d82c7ad47e7e436ed4a0ddb065365a20a`, tree
  `6020165a3bfbab00bae1910a197b2d4dc9bf57b3`, received independent technical
  and design `ACCEPT` and is the immutable direct parent for OC-04C.
- OC-04C starts from that accepted parent. Commit
  `97f89034d8c2d3f9c77e12e162d77ae0b542af0f` adds a pure, network-free and
  filesystem-free activation admission core. It strictly decodes canonical
  manifest bytes, binds the snapshot/revision/projection digest and exact
  same-origin snapshot paths, admits only declared detail and schema-node
  entries, sorts them, and returns defensive copies only after every check
  succeeds. Commit `13a919b1a91559fa6b36269b77b50c5e847e9c71`
  exposes canonical `manifest.json` bytes under root and nested immutable
  snapshot routes with GET, HEAD, conditional 304, fixed size/SHA-256 ETag,
  JSON MIME, and immutable cache headers, and adds its exact URL to the existing
  eligible descriptor. Commit `efd3f7c66bf93af0b4194fac29825e98195dcc9b`
  closes the defensive-copy contract and expands mutation coverage for strict
  JSON, missing/extra inventory, encoded traversal, and coordinated path/kind
  drift. Failure returns zero activation state, while SSR/no-JS and OC-04A/B
  behavior remain authoritative.
- OC-04C RED receipts captured missing admission types, empty descriptor URL and
  manifest-route 404s, accepted encoded-traversal/query-like bases, and exposed
  mutable inventory. GREEN receipts include localdocs race 1.334s, web race
  3.298s, architecture 9.066s, vet, strict Muamba, stable templ regeneration,
  and focused projection/enhancement tests. The first root run failed only the
  pre-existing catalog-search focus E2E once; its exact rerun passed in 3.06s.
  A complete fresh root rerun then passed with self-hosted 178.553s and E2E
  186.989s. The final moving goal-child identity remains external for review.
- OC-04C goal child `b98f00d63d78a66e27d31fa19e9f8024fc6d2e00`,
  tree `d8221d003eecbc2a360bf74cb1ea50aabcd8e188`, received independent
  `ACCEPT` and is the immutable direct parent for OC-04D.
- OC-04D implementation commit
  `a7ebe1578cba30211c9810301304565d5b43d6e9`, tree
  `3e449e3d9b84e62599afafeeb68ebdde5878951c`, adds parser-free shard
  preparation after OC-04C admission. `SelectDetail` and `SelectSchemaNode`
  require the exact admitted path and kind, verify size and SHA-256 before the
  strict canonical codec, bind the exact document key, and select only the
  requested detail ID or schema-node ordinal. Every failure returns a zero
  value. Successful results retain no input or activation-state aliases. The
  package performs no network, filesystem, parser, template, or HTML work.
- OC-04D RED captured both absent selection methods. GREEN and mutation proofs
  cover operation/schema details, schema nodes, unknown and cross-kind paths,
  changed size, same-length substitution, duplicate/unknown/trailing/
  noncanonical JSON, wrong document, missing ID/ordinal, and input/result
  mutation. Focused web OC-04A/B/C tests passed in 0.457s, localdocs race in
  1.353s, architecture/Wasm in 6.991s, plus vet and strict Muamba. Full root
  tests passed with self-hosted 164.053s and E2E 189.850s. The moving final
  goal-child identity remains external for review.
- OC-04D goal child `fa248fdc5adb45e72953f5d9ac9db9f318b70ff0`,
  tree `4187a705f13a34de834852f8ff82a13d205c093d`, received independent
  `ACCEPT` and is the immutable direct parent for OC-04E.
- OC-04E implementation commit
  `00632ce4fd83ad495c2d6d3fd0854420c66d277f`, tree
  `18ed93f485adfa955a12608ae05ff2aeef83680f`, adds a pure prepared
  schema-node HTML fragment renderer under `internal/localdocs/render`.
  Admission requires the exact schema detail identity, canonical document href,
  schema-node IDs and ordinals, and exact unique direct-reference coverage.
  Inputs are copied; projection text is escaped through one templ component;
  inconsistent or oversized inputs return no bytes. The package performs no
  parser, network, filesystem, HTTP, or template-raw work and compiles for
  `js/wasm`.
- OC-04E delegation commit `89eb2d36c37da90cf2b5bee63cfb3fce5dc9f2b3`,
  tree `3dd362b452a789a7a6e9c6915ab9448ea24aae5f`, removes the duplicate
  server-only schema-node component and makes existing SSR and the future Wasm
  boundary call the same renderer. A focused test captured byte-identical local
  and untouched SSR fragments before delegation and continues to prove exact
  parity after it. Existing DOM IDs, classes, landmarks, Goshtoso tooltip,
  escaping, HTMX hrefs, 100-edge display bound, and theme tokens remain in the
  shared component.
- OC-04E RED receipts captured the absent preparation API, unvalidated schema
  identities and hrefs, incomplete/extra references, encoded traversal, and a
  retained second renderer. GREEN receipts include focused fragment and SSR
  parity tests, localdocs/web race, architecture and `js/wasm` dependency/build
  gates, vet, strict Muamba, stable templ generation, and diff check. Full root
  tests passed with self-hosted 193.387s and E2E 189.120s. The moving final
  goal-child identity remains external for fresh review.
- OC-04E goal child `acb83447b847eacc2ba12207d2a93d092cb1be75`,
  tree `9407fd41174dc14eb29a81fc2b657684767b1704`, received independent
  `ACCEPT` and is the immutable direct parent for OC-04F.
- OC-04F implementation commit
  `1bcbce91c4069f359e29536a357858c26e3e5720`, tree
  `99ef2ceb0e2f265d9c02e4200b549e9779ec494d`, adds a parser-free prepared
  operation-header HTML fragment under `internal/localdocs/render`. It binds
  the immutable operation detail ID, anchor, heading, compiler-produced href,
  method, path, summary, description, and deprecation state to the prepared
  operation. Inputs are copied; templ escapes projection text; a 2 MiB bound
  prevents partial oversized output. The package performs no parser, network,
  filesystem, HTTP, or template-raw work and compiles for `js/wasm`.
- OC-04F delegation commit `d75fa9a98799acfdcc8aec90db6026cd73e4efd5`,
  tree `601554d4bcfc3a8d2f78768af1c977f199a177c2`, removes the duplicate
  catalog operation-header renderer. Existing SSR delegates to the same pure
  component with current Copy Page and provenance components supplied as
  explicit slots. The endpoint body, request composer, response/schema trees,
  operation navigation, canonical metadata, and no-JS routes remain on their
  existing server path.
- Immutable-base reconciliation proved the accepted compiler emits
  `documents/<key>/?selected=<detail>#<detail>` and partitioning copies that
  exact href into operation details. Two accepted web fixtures still used the
  older `?selected=<detail>` shape; only those fixture identities were aligned.
  Production href semantics were not widened. Lowercase HTTP methods remain
  accepted and rendered uppercase, matching the existing compiler and badge
  contract.
- OC-04F RED receipts captured the absent preparation API, inconsistent
  identities/prepared fields, invalid document and endpoint paths, and retained
  duplicate renderer. A pre-delegation test proved the prepared header matched
  untouched SSR bytes; the durable test proves the same exact SSR/component
  bytes after delegation. GREEN receipts include focused fragment/web/template
  and architecture tests, localdocs/web/template race, vet, strict Muamba,
  `js/wasm` build, stable templ generation, and diff check. Full root tests
  passed with self-hosted 194.240s and E2E 187.018s. The moving final goal-child
  identity remains external for fresh review.
- OC-04F goal child `a901b008164227d8b480124bafb59ae1d9903df0`,
  tree `3287ec7c5dbd81da23dacb66ffd496c13d3a8861`, received independent
  `ACCEPT` from technical and design reviewers and is the immutable direct
  parent for OC-04G.
- OC-04G renderer commit `dbce706d11e9a7d09410fd042ae2ef83046d80a4`,
  tree `12740225f4797496d64eace492abcf264ffde2bc`, adds one parser-free
  operation-parameter fragment under `internal/localdocs/render`. It verifies
  the complete ordered projection parameter inventory against the prepared
  operation and the exact recursively used schema-node inventory, copies all
  inputs, preserves compiler ordinals/IDs and the established path/query/header
  grouping, and emits no cookie or other unsupported parameter group. Invalid,
  missing, duplicate, reordered, extra, inconsistent, or oversized input fails
  without partial bytes. It imports no parser, network, filesystem, HTTP,
  template-raw, or browser API and compiles for `js/wasm`.
- OC-04G correction commit `1302019c60304add6d70c242721fef505eecd363`,
  tree `ba481c161fa525b6a7927396ed844563f77ec774`, preserves the exact existing
  empty-schema location fallback and named primitive-enum label contract. The
  bounded schema inventory allows the existing 256-node recursive expansion
  plus one root per declared parameter rather than imposing a smaller
  parameter-count limit.
- OC-04G delegation commit `69a35397ce228f07908bc94e8eab20f378af10fd`,
  tree `d323a93a55f7cd8d15986513bd8df3af86101da7`, makes catalog SSR use the same
  component for Path, Query, and Header Parameters only. It retains exact
  sibling whitespace, IDs, `<dl>` semantics, required/optional states,
  templ escaping, Alpine disclosure and keyboard-focus attributes, responsive
  classes, theme tokens, request composer, navigation, canonical metadata, and
  no-JS server behavior. Standalone public docs with a Markdown renderer keep
  their existing server-only parameter path.
- OC-04G RED receipts captured the absent preparation API, then real-handler
  rejection of a stale noncanonical parameter fixture, whole-endpoint sibling
  whitespace drift, and an operation-only node-capture panic on the untouched
  schema route. GREEN receipts include adversarial fragment mutations, exact
  legacy group and complete endpoint byte parity, root/nested catalog routes,
  preserved schema parity, full localdocs/web/template/architecture tests,
  affected race and vet, strict Muamba, stable templ generation, `js/wasm`
  dependency/build gates, and diff check. Full root tests passed with
  self-hosted 194.923s and E2E 188.570s. The moving final goal-child identity
  remains external for fresh review.
- OC-04G goal child `52f99958a9a326e653b771f3d5624a15a06113ee`,
  tree `748d84df049c1a6a2eb54be42e14c71be751710c`, is preserved unchanged as the
  rejected parent for this correction. Independent technical review found one
  P1: the prepared renderer retained separators only between nonempty groups,
  while legacy SSR retains the fixed Path/Query/Header slot boundaries even
  when a slot is empty.
- The bounded correction retains all three ordered slots, renders HTML only for
  populated groups, and preserves the two literal slot separators. A RED parity
  matrix failed seven of eight Path/Query/Header presence combinations; GREEN
  proves all eight complete endpoint byte streams, including a request kept
  renderable only by an unsupported cookie parameter. Focused, affected, race,
  vet, strict Muamba, stable templ generation, architecture and `js/wasm` gates
  pass. The first root run exposed one transient pre-existing catalog-focus E2E
  failure; its exact isolated rerun passed in 2.90s, and a complete root rerun
  passed with self-hosted 190.049s and E2E 189.365s. The moving correction-child
  identity remains external for fresh review.
- OC-04G correction head `fb9381f4b04821f322d128da3c7a50ce827af7d6`,
  tree `35fa43dfc56375e8af37b52438614ea4b3d3260e`, received independent
  `ACCEPT` and is the immutable direct parent for OC-04H.
- OC-04H renderer commit `b9e8e6e3f985e37430f9a0f06eb9af4f99ac7dbf`,
  tree `ee6af5c4336f02a531c4cc9115b7ef3e674fbf09`, adds one bounded,
  parser-free request-body media-summary fragment. It validates exact
  request-body declaration, description, required state, ordered media
  ordinals/IDs/content types/examples, unique referenced root schema-node
  inventory, root name/type/format/enum identity, canonical same-document
  schema hrefs, defensive copies, media index, and the 2 MiB output limit.
  Invalid, missing, duplicate, extra, reordered, inconsistent, malformed, or
  oversized input fails without partial bytes. Rendering uses templ escaping
  and public Goshtoso badge/icon APIs, with no parser, network, filesystem,
  HTTP, browser API, or `templ.Raw` dependency.
- OC-04H delegation commit `5a64e3814a1889d952de4a9ddfb1ef6ba6062494`,
  tree `df62c0ee3f6f2c7008e10284827aaf3bda23f66a`, captures the already-read
  unique request-body root nodes without a second child read and makes catalog
  SSR share the media badge, shallow schema label, and optional same-document
  schema link. Recursive schema properties, request-body description, examples,
  request composer, responses, and standalone fallback remain server-owned and
  unchanged. Full endpoint parity covers zero, one, and multiple media types;
  root and nested mounts preserve href, HTMX, ARIA, focus, responsive, theme,
  canonical, and no-JS behavior. Operations without a request body retain a nil
  fragment rather than failing catalog rendering.
- OC-04H RED receipts captured the absent preparation API; permissive example,
  media whitespace, enum, and href validation; absent `PublicDocsOptions` seam;
  absent catalog preparation; and the real no-request-body failure. GREEN
  receipts include focused and full localdocs/web/template packages, root and
  nested real-handler proof, exact zero/one/multiple complete-endpoint bytes,
  affected race and vet, architecture and `js/wasm`, strict Muamba verification
  and generation, webassets, stable templ generation, and diff check. Full root
  tests passed with self-hosted 206.767s and E2E 189.169s. No new Goshtoso
  source-lookup snag occurred; existing public badge/icon APIs were sufficient.
  The moving final goal-child identity remains external for fresh review.
- At `2026-08-12T03:34:23Z`, the confirmed public preview URL
  `https://manja.araihu.com/manja-assets/manja-social.png` returned HTTP 200,
  `image/png`, 21,500 bytes, 1280x640, and SHA-256
  `7234c9a20fc3a4a44364b8f9d544ddae5aba8c2b6a418b26ad5a930d2d0ab0bd`.
  The external central deployment configuration confirms
  `manja.araihu.com` as the Manja production host, but currently routes it to
  the renderer runtime rather than the product-site binary. OC-01M7 therefore
  proves local server-rendered product-site HTML and the already-live preview
  asset; it does not claim that the new homepage or `/docs` metadata has been
  deployed.
- Goshtoso snag: the existing product site is an `html/template` shell, not a
  templ/Goshtoso App Shell. Replatforming it solely to call
  `head.Metadata` would mix shell/theme work into this narrow metadata patch;
  OC-01M7 keeps the existing server-rendered shell and records that migration
  as a separate future reconciliation.
- Its direct ancestry after the fresh-main base is `bd4a96fbaa898142845d8eeab051a45b504cff3a`,
  `87fe9e8d1bb3b3824b1aa3ecbbc2cb01e2fb1ff9`, rejected candidate
  `2da4be9420def8c24356ebc3e08b4e662a4b4244` / tree
  `bee9343d70a13ce4163b1d819f390e54cc804065`, rejected child
  `39a622bc25d94434608f903792e2e9f7f249904a` / tree
  `73188114ef9c3b3f62b59a614a41b26609bd054a`, rejected child
  `f10e16b05e7ff69fc13c70cabaff5f17c57eafd0` / tree
  `c3079d2025188e442ba8cee9aaee27c371966d97`, then accepted child
  `b4d7562e7a97903726234a3d8d8a8d2130bf4481` / tree
  `ef6f22e9842d233ccde7acfd93c1563941d1290a`. The accepted child's immediate
  parent is `f10e16b05e7ff69fc13c70cabaff5f17c57eafd0`, not `origin/main`; its merge-base
  remains exact `9733949b8dde9eb0fe1ef285ceb3ecbeb88f5d06`.
- Accepted scope is mechanical captured-Git-byte integrity only: strict bounded
  receipt JSON, exact key allowlists, root-confined no-symlink regular-file
  receipt admission, SHA-1/SHA-256 Git commit/tree/inventory/blob and raw-byte
  verification, recursive captured-file coverage, same-read substitution
  rejection, and preserved operational error classification. Legal provenance,
  attribution, notice, and authority stay separate; overall provenance remains
  `BLOCKED`.
- Public domain/application/port packages, adapter contract tests, external
  module proof, architecture gates, and self-hosted composition already exist.
- `docs/legal/provenance.md` remains `BLOCKED`; root `LICENSE`, `NOTICE`,
  `THIRD_PARTY_NOTICES.md`, deterministic SBOM generation, and verified release
  packaging remain absent.
- OC-01 refreshed the stale July legal inventory with current Muamba/browser,
  Kubernetes renderer, social preview, Go/Alpine, source-archive, site, and OCI
  evidence. Provenance remains `BLOCKED`, and licensing/package-generation Task
  8 remains stopped.
- Simple Icons provenance checkpoint `7eb7d58c8bb936d2ca3813b90f91884a2f9fdb29`
  / tree `f234bf311c2a6a03e3cde4bf126a07c6d1e30182` received independent
  `ACCEPT`, then was fast-forward integrated and pushed by the product manager.
- CodeRabbit review `4904235223`, run
  `be132353-6da0-4b99-a5ab-5b9785ed2126`, completed substantively at exact
  `7eb7d58c8bb936d2ca3813b90f91884a2f9fdb29`. Discussion `3756311030` is
  independently `VALID` and material. The first correction candidate
  `051ef67bfb7dd49c7052fe7ee743fd1a88fad1ab`, tree
  `abf63a77c74228e019a1e39dc4016b156ab05389`, was rejected because same-`RUN`
  tails and Go flag aliases could bypass its exact-flag scan. Child
  `d5ede512ebb784c7c695948616fb209ac182db1e`, tree
  `c5e43f4336690bea451088cb023f654bd572aeb6`, replaced flag scanning with a
  strict canonical full-command token comparison and is integrated and pushed
  on the product-manager branch.
- OC-01M5's exact observed Manja social-rendering checkpoint and goal correction
  remain in that history at `dd1a9e5d41422cb400d99a407500562c70ab21a0`, tree
  `7ac46daea42ab9215231475f215becbf539de9fc`.
- Final-head CodeRabbit review `4904696726`, run
  `05b3efe6-5170-4f07-bb67-59f5121f6772`, reviewed exact
  `d5ede512ebb784c7c695948616fb209ac182db1e` and produced a material rejection.
  Independent classification confirms the complete browser-test-source artifact
  scan as material. Strict receipt decoding, corrected module prose, stable
  worktree labels, effective-build-command failure wording, and retained
  Dockerfile parse diagnostics are valid minor corrections. Candidate
  `2bd9114ec7c2a6d034c66a56692b3141da2a769a`, tree
  `b9b1fae0b0875938de84f55668a205478c947410`, implemented those findings and
  was integrated and pushed.
- CodeRabbit review `4905151932`, run
  `fde28eb6-45b5-4def-945e-4efe262617dd`, thread
  `PRRT_kwDOSzGXLc6YMCp7`, comment `PRRC_kwDOSzGXLc7f8HHp`, reviewed exact
  `2bd9114ec7c2a6d034c66a56692b3141da2a769a`. Independent verdict:
  `VALID` and material. A caller-selected clean subdirectory could pass while
  prohibited source remained elsewhere, so that exact parent is rejected for
  merge. This bounded docs correction removes the false host-archive success
  claim and leaves the host archive gate blocked under Task 8. Exact correction
  `e3d6bb977c096dc13933068369f46d3cf8decd3c`, tree
  `d867bfd01430196aad29e97b67636564958c1b17`, received independent `ACCEPT`
  with no findings, then was fast-forward integrated and pushed unchanged.
- Exact-head CodeRabbit invocation
  `84e96e2d-bec2-45ef-9ef6-925155c9f783` at `e3d6bb977c096dc13933068369f46d3cf8decd3c`
  was rate-limited and produced no substantive review object. A successful
  status context is not review evidence, so the CodeRabbit merge gate remains
  open. Goal-ledger correction `52c7598c4ea1f0b5f0f5e27e363320866b49f789`,
  tree `cef5aa1ce775095d1e7cd8d156c9f9249f8b2901`, received acceptance, then was
  fast-forward integrated and pushed unchanged.
- CodeRabbit review `4905658559`, run
  `d5161f3c-d4aa-41e6-af99-3f76ba07eeb0`, invocation
  `f2f1fbc0-de97-4c29-9fe0-365880fb74cc`, reviewed exact
  `52c7598c4ea1f0b5f0f5e27e363320866b49f789` substantively. Independent verdict:
  `VALID` and material. The inline digest/publication facet and outside-diff
  no-execution facet are one OCI inspection trust-boundary finding, not two.
  Candidate `59f6df7de70f7b41d3d53c911307192c5c2fe7ef`, tree
  `6ea9a541e6ddfbf9b8b6cf11a72e54a1c312c171`, marked OCI distribution
  inspection blocked and recorded prospective fail-closed Task-8 invariants,
  but was rejected because it falsely said no published release artifact
  existed. Child `a0d1c4e0622b91a070ff96abbeda0ac5d874e82a`, tree
  `d580a55a59972032e578a9f38e5287341517ed71`, corrected that claim, received
  acceptance, then was fast-forward integrated and pushed unchanged.
- CI runs `31485230010` at `52c7598c4ea1f0b5f0f5e27e363320866b49f789`
  and `31487572347` at `a0d1c4e0622b91a070ff96abbeda0ac5d874e82a`
  reproduce one narrow failure. `TestKubernetesCatalog` exhaustively checks all
  3,028 published detail IDs through global exact search; changing late IDs
  returned the bounded 31-byte temporary-unavailability 503 after both the
  initial request and its one documented retry. Activation, public and private
  Forgejo paths, the retry-helper test, root tests, and CodeQL passed in both
  runs. The test is not racing renderer readiness: its catalog, directory, and
  detail checks have already succeeded before the exhaustive search loop.
- Candidate `1866412c67650a413f6e2340f728a278ef8b685c`, tree
  `b1622cdf413945ab98c2b03a4ad313d2f217faa0`, resolved the production ordering
  defect by looking up published detail IDs in the admitted immutable catalog
  directory before deadline-bound search-child loading, but independent review
  rejected it. Its early path bypassed canonical `SearchService` validation:
  over-limit and control-wrapped exact IDs returned 200, while an
  NFKC-equivalent exact ID remained deadline-bound and could return 503.
- Correction `f185838ab5fcb1eb6ca9c2a75e0f54c56e164c9a`, tree
  `7c6a60757fbad01f8fc5d62db65952af06dda98d`, canonicalizes and validates the
  caller query exactly once, then passes the same opaque canonical value to
  both directory lookup and subsequent search. Controlled RED reproduced all
  three rejected cases;
  GREEN returns 400 without child reads for invalid queries, resolves the NFKC
  exact query directory-only, and preserves non-exact persistent 503 plus
  `Retry-After: 1`. It received independent acceptance, then was fast-forward
  integrated and pushed unchanged.
- CodeRabbit review `4906397511`, run
  `03263440-726a-4661-bd9e-b2d52654deb9`, reviewed exact
  `f185838ab5fcb1eb6ca9c2a75e0f54c56e164c9a` and was submitted at
  `2026-08-11T12:51:50Z`. Independent classification marks its final-head-loop
  wording `DUPLICATE`: existing external identity, review, CI, CodeQL, and
  substantive CodeRabbit gates already fail closed, so no recursive candidate
  identity is added. Its unconditional exact-directory preflight finding is
  `VALID` and material: ordinary global queries compare against all 4,872
  current demo details before the existing child-search deadline.
- Exact-query traversal correction
  `4578206bec2d28d2ca51e9b25f6823078c56e333`, tree
  `6c5077ed6e570fa92c2d9e5149a9a2eaf363e7bb`, keeps canonical validation first,
  then permits exact directory traversal only for canonical
  `detail-sha256-` plus 64-lowercase-hex queries. Controlled decoy receipts
  prove wrong-prefix, wrong-length, non-hex, suffixed, and ordinary queries
  enter bounded `SearchService` instead. Exact lowercase,
  uppercase-normalized, and NFKC-equivalent IDs remain directory-only and
  collect across catalogs. It received independent `ACCEPT` with no findings,
  then was fast-forward integrated and pushed unchanged.
- The first automatic CodeRabbit status at `2026-08-11T13:20:54Z` for exact
  `4578206bec2d28d2ca51e9b25f6823078c56e333` was rate-limited and insufficient.
  After exact-head revalidation, the product manager retriggered review at
  `2026-08-11T13:52:18Z` with [request comment
  `5254090894`](https://github.com/araihu/manja/pull/94#issuecomment-5254090894),
  invocation `aaedd82e-3e68-47f6-b28c-2fd7da1f4ad7`. The [bot reply
  `5254092288`](https://github.com/araihu/manja/pull/94#issuecomment-5254092288)
  was updated to `Review finished` at `2026-08-11T13:55:39Z`; the exact-head
  CodeRabbit context moved from `Review in progress` to successful
  `Review completed`.
- The [CodeRabbit standing
  summary](https://github.com/araihu/manja/pull/94#issuecomment-5249207465) was
  updated at `2026-08-11T13:55:36Z`. Run
  `048fe19f-1f5f-466e-844c-9f1f6b23cdce` reviewed exact range
  `f185838ab5fcb1eb6ca9c2a75e0f54c56e164c9a...4578206bec2d28d2ca51e9b25f6823078c56e333`,
  selected exactly `goal.md`, `internal/web/catalog_search.go`, and
  `internal/web/catalog_test.go`, and generated no actionable comments. It
  processed `internal/web/catalog_test.go` and disclosed that
  `internal/web/catalog_search.go` and `goal.md` were skipped as similar to
  earlier changes; independent exact-byte review covers those skipped files.
- The independent final-gate audit chose `ACCEPT` option A: this successful
  exact-head incremental review/check is substantive and has zero findings.
  The absence of a new pull-review object does not block this immutable
  identity because the retrigger, invocation, status transition, run, exact
  range, selected/skipped files, and standing-summary result form the stronger
  evidence chain. The generic docstring warning is non-material: the change
  adds only an unexported helper and conventional Go `Test*` functions, with no
  undocumented exported production API.
- At that historical checkpoint, the PR gate was satisfied only for immutable
  `4578206bec2d28d2ca51e9b25f6823078c56e333`, tree
  `6c5077ed6e570fa92c2d9e5149a9a2eaf363e7bb`. Any child or other head movement
  restarts exact-head independent review, CI, CodeQL, and substantive
  CodeRabbit gates. This goal-only ledger child adds no product decision; its
  moving commit and tree identity remain external to avoid recursive
  self-reference.
- Goal-only ledger checkpoint
  `811b34b311c29dd60a70d6c88b0c0ec155ffbf12`, tree
  `d947f03358248aeb51c851ac0d9da93045f7a047`, was the pushed PR head for this
  pre-merge review snapshot. CodeRabbit review
  [`4907614755`](https://github.com/araihu/manja/pull/94#pullrequestreview-4907614755),
  run `8fda1015-1268-4df3-af8e-afc302d9b7d2`, reviewed exact range
  `4578206bec2d28d2ca51e9b25f6823078c56e333...811b34b311c29dd60a70d6c88b0c0ec155ffbf12`
  after [request
  `5254855031`](https://github.com/araihu/manja/pull/94#issuecomment-5254855031),
  invocation `a837d3db-c790-4712-9e36-804a1d2af553`, and [finished reply
  `5254857274`](https://github.com/araihu/manja/pull/94#issuecomment-5254857274).
  It selected and processed the only changed file, `goal.md`. Independent
  classification is `REJECT` for merge only because the durable merge gate did
  not explicitly require a clean worktree.
- Discussion
  [`3759102440`](https://github.com/araihu/manja/pull/94#discussion_r3759102440)
  is a false positive and non-material: the referenced lane-base entries are
  historical lineage. Operating Rule 1 still requires every new unit from
  current `origin/main`; this correction does not rebase, absorb active-theme
  bytes, or change that scope rule. Discussion
  [`3759102445`](https://github.com/araihu/manja/pull/94#discussion_r3759102445)
  is duplicate, superseded, and non-material: the current exact-811 review
  selected and processed `goal.md`, while the prior exact-457 skip remains
  truthfully disclosed. Discussion
  [`3759102451`](https://github.com/araihu/manja/pull/94#discussion_r3759102451)
  is duplicate and non-material: the prior immutable-identity `ACCEPT` remains
  applicable after unchanged fast-forward integration and push; fresh review is
  already required only when bytes or identity move.
- The outside-diff clean-worktree finding is `VALID` and material. That
  correction made an empty staged, unstaged, and untracked state an explicit
  merge condition. The generic 0% docstring warning is a false positive and
  non-material because the reviewed delta is Markdown-only. That correction's
  moving head and tree stay external to avoid recursive self-reference.
- Full root tests, strict Muamba verification, architecture, unrelated external
  module, generation, browser, API, and templ gates passed for the OC-01
  evidence. Local OCI inventory observations supplied baseline evidence only;
  they are not a digest-bound distribution gate, which remains blocked under
  Task 8. On the OC-01M7 parent, direct standalone site setup and tests pass
  with `GOWORK=off`; this supersedes the earlier historical dependency-graph
  failure without changing site module files.
- Renderer/catalog initial HTML and preview-image checks pass the social-ready
  metadata gate. OC-01M7's equivalent product-site contract is independently
  accepted. OC-01M8's route-specific canonical/Open Graph/explicit X Card
  metadata and preview response contract for standalone public docs is also
  independently accepted. OC-01M9's separate catalog-search keyboard-focus
  correction is independently accepted. OC-04A is independently accepted at
  exact `05fcf118d9eb7fca94ae9a587f2776771f4678ef`, tree
  `9e806e77adbf3fb1b007925eeb6a2fd497fe90dd`. OC-04B correction starts from
  rejected exact `f02469c39e09576963b9de4d98807abfc542b792`, tree
  `f6400357ebfefcc4571b4bd3246a3176df797e99`, and awaits fresh exact-identity
  independent review.
- Active Arai Hû Modern-theme rollout belongs to task
  `019fef01-b65f-7980-a360-83e48f8a6345`; this control plane must avoid its
  files and refs.
- Historical Manja worktrees and branches remain preserved. Existence does not
  authorize reuse or cleanup.

## Checkpoint Queue

### OC-01: Current Open Core provenance and artifact baseline

Status: PR #94's accepted OC-01 baseline and mechanical corrections were squash
merged at exact `9733949b8dde9eb0fe1ef285ceb3ecbeb88f5d06`, tree
`10c75d6c0fa3e945093bc47e46df0d33e0ad40de`. The subsequent fresh-main Stripe
captured-byte integrity line was squash-merged through PR #96. Exact
implementation head `b4d7562e7a97903726234a3d8d8a8d2130bf4481`, tree
`ef6f22e9842d233ccde7acfd93c1563941d1290a`, is independently accepted with no
findings. Accepted ledger head `2ba674e4b7b8000a32ea47ce8896b9d741681b7f`,
tree `b86fcf32d882d5baefd8ced024da2cd261ade191`, remained in the accepted PR
lineage. Exact source head `ad9d114ede2fe8b5337331eee8268e830a0e4577`
passed final gates and was squash-merged as
`43f96dfbf9d18eee2364f14778e6b94312c8abac`, tree
`959f199e145c316b4b76e40a561413c0e6d57134`. The next narrow OC-01 checkpoint
activates that accepted integrity seam for the configured Stripe build input;
it does not claim overall provenance clearance.

Accepted source identity:

- base: `39d65ade21c080ee2102f53da5ed741f000d6dd7`;
- rejected candidate preserved: `bcb8bcf455d380f4c35787fc3671f512c917ca7d`;
- accepted corrected head: `f4a9f48080902e0f6e390589e9f8662b525131f1`;
- accepted corrected tree: `9324262e34a2db5596300aa3f74cf77eaabada28`;
- independent verdict: `ACCEPT`, no findings;
- PM integration commits: `f5b2f6d` and `17f7d2e`.

Reviewed PR-transition identity:

- parent commit: `98b6218e4f78236e68708ef4332975ee5292badc`;
- parent tree: `e9ec53bd10e5255db449e7eb1bed9a14075ab760`;
- independent verdict: `ACCEPT`, no findings.

Preserved local product-manager ref and worktree after PR #94 merge:

- head: `d520edd5b6115904fbed247729dcf997716e2d03`;
- tree: `eb9c05086b0f6d0380fc5da593ef96c8e3febd2d`;
- disposition: local ref and worktree preserved unchanged; remote
  `refs/heads/coord/opencore-product` is absent after the authorized squash
  merge and is not recreated or pushed.

Fresh-main Stripe captured-byte checkpoint:

- base and merge-base: `9733949b8dde9eb0fe1ef285ceb3ecbeb88f5d06`;
- base tree: `10c75d6c0fa3e945093bc47e46df0d33e0ad40de`;
- accepted local head: `b4d7562e7a97903726234a3d8d8a8d2130bf4481`;
- accepted local tree: `ef6f22e9842d233ccde7acfd93c1563941d1290a`;
- immediate parent: `f10e16b05e7ff69fc13c70cabaff5f17c57eafd0`;
- accepted ledger head: `2ba674e4b7b8000a32ea47ce8896b9d741681b7f`;
- accepted ledger tree: `b86fcf32d882d5baefd8ced024da2cd261ade191`;
- final source head: `ad9d114ede2fe8b5337331eee8268e830a0e4577`;
- squash merge: `43f96dfbf9d18eee2364f14778e6b94312c8abac`;
- merged tree: `959f199e145c316b4b76e40a561413c0e6d57134`;
- disposition: implementation and ledger independently accepted; exact-head CI
  retry, CodeQL, CodeRabbit, and independent review passed; PR #96 merged with
  authorization.

Stripe integrity activation checkpoint:

- base and merge-base: `43f96dfbf9d18eee2364f14778e6b94312c8abac`;
- base tree: `959f199e145c316b4b76e40a561413c0e6d57134`;
- branch: `codex/oc01-stripe-integrity-activation`;
- first coherent implementation commit:
  `a0da08297630502ebee5290b5bbb4cd2e79359bf`;
- scope: adjacent runtime receipt, real renderer-source binding, strict offline
  receipt proof, and exact legal-provenance reconciliation only;
- accepted candidate: `dab2b8f24552f89cc2b1d705181a1ac4a2b97847`,
  tree `1abee072de1fce813731bfc3e02706c31cd0eb6d`;
- disposition: independently accepted by developer and designer reviewers,
  clean and preserved locally; no push or integration is authorized.

OC-01M7 product-site social metadata checkpoint:

- direct parent: accepted Stripe candidate
  `dab2b8f24552f89cc2b1d705181a1ac4a2b97847`, tree
  `1abee072de1fce813731bfc3e02706c31cd0eb6d`;
- fresh-main merge-base: `43f96dfbf9d18eee2364f14778e6b94312c8abac`,
  tree `959f199e145c316b4b76e40a561413c0e6d57134`;
- branch: `codex/oc01m7-product-site-social`;
- first implementation commit:
  `e342c68fff9abc9be1d744da8fb7c3c73f6e6c81`, tree
  `f1aba174a207b6db02afbcee6a3f9892d18f14fa`;
- scope: product homepage and `/docs` initial HTML only, with route-specific
  title, description, canonical URL, `og:url`, complete Open Graph image
  structure/alt, explicit X Card tags, and the existing Manja preview asset;
- exclusions: standalone renderer metadata, catalog focus, active theme, hosted
  SaaS, hybrid SSR/Wasm/offline runtime, packaging, and lifecycle actions;
- accepted candidate: `fd2f16f36a423a8fea2f4273695aaad39525614f`,
  tree `dc411a51a9e6674c21f5bfc187d455ecc26c9f26`;
- disposition: independently accepted by technical and design reviewers, clean
  and preserved locally; no push or integration is authorized.

OC-01M8 standalone public-docs social metadata checkpoint:

- direct parent: accepted OC-01M7 candidate
  `fd2f16f36a423a8fea2f4273695aaad39525614f`, tree
  `dc411a51a9e6674c21f5bfc187d455ecc26c9f26`;
- fresh-main merge-base: `43f96dfbf9d18eee2364f14778e6b94312c8abac`,
  tree `959f199e145c316b4b76e40a561413c0e6d57134`;
- branch: `codex/oc01m8-public-docs-social`;
- implementation commit: `5e0b3cfd3f2e4f79a84fb81c072f0b016db06b16`,
  tree `61e5620116aacdd3c19f4694fa079bd3e57fc7b3`;
- rejected ledger candidate: `8f0dda961aac098f4eda2395f9648b8ba67eab85`,
  tree `7ee2c647e3a9e8fa0f7c593359b097b8c1b5ff8c`;
- technical verdict: `REJECT` for Host-header authority, non-root publication
  path loss, and unresolved selection/canonical mismatch; design scope verdict:
  accepted;
- web correction: `63f6edf2f66fbf31d016c07dc611c31146eae1f6`,
  tree `4073d395a16a110f6dae1d4242125ba93fda45b9`;
- self-hosted wiring correction:
  `a5e88520c153673f34b58963b3051773817e4711`, tree
  `03f83599944910f88c4c35f436142fe59fb20080`;
- scope: standalone public docs overview, operation, and schema initial HTML,
  with route-specific title, plain-text description, canonical URL, `og:url`,
  complete Open Graph image structure/alt, explicit X Card tags, and the
  existing approved Manja preview asset; remote canonical/image authority is
  operator-configured through `-public-origin`, Host/forwarded headers remain
  untrusted, and loopback HTTP is development-only;
- exclusions: product-site shell, catalog focus, active theme, hosted SaaS,
  hybrid SSR/Wasm/offline runtime, packaging, and lifecycle actions;
- accepted candidate: `cb3c2ac9df78482472a3929476ec77e0a885a2f1`,
  tree `9011e5def2caf30c40f18c9737f43bb2b2d8976c`;
- disposition: independently accepted by technical and design reviewers, clean
  and preserved locally; no push or integration is authorized.

OC-01M9 catalog-search focus-indicator checkpoint:

- direct parent: accepted OC-01M8 candidate
  `cb3c2ac9df78482472a3929476ec77e0a885a2f1`, tree
  `9011e5def2caf30c40f18c9737f43bb2b2d8976c`;
- fresh-main merge-base: `43f96dfbf9d18eee2364f14778e6b94312c8abac`,
  tree `959f199e145c316b4b76e40a561413c0e6d57134`;
- branch: `codex/oc01m9-catalog-search-focus`;
- implementation commit: `d1b464b23bd47b45dfe537b4c7333562f35a0684`;
- scope: visible `:focus-visible` outline on the catalog-search combobox only,
  using existing light/dark primary tokens, with no box-model change;
- tests: rendered focus-class and no-layout-shift contract, computed browser
  outline/token/contrast proof, focused catalog/template/race/vet gates, strict
  Muamba, stable templ regeneration, catalog E2E, and full root suite;
- exclusions: active-theme redesign, standalone/social metadata changes,
  hosted SaaS, hybrid SSR/Wasm/offline runtime, legal/runtime bytes, packaging,
  and lifecycle actions;
- accepted candidate: `57643af0bae0727bf2b9e78622130fb1aa6e26ec`,
  tree `ec973970d9545d7a7e3b20ea0bfc9d4d22938d19`;
- disposition: independently accepted by technical and design reviewers, clean
  and preserved locally; no push or integration is authorized.

The final moving candidate head and tree are bound by the immutable external
review packet and control plane. A commit cannot embed its own final commit and
tree identity without changing that identity recursively.

Goal: reconcile the approved Open Core plan with current `origin/main` and
produce current, behavior-backed provenance and shipped-artifact evidence
without making an Apache-2.0 claim while authority remains blocked.

Required work:

- inspect current first-party, copied, generated, browser, Go, site, and OCI
  inputs;
- update stale evidence in `docs/legal/provenance.md` and
  `docs/legal/shipped-artifacts.md` only when current commands prove it;
- resolve mechanical provenance gaps where repository evidence is sufficient;
- identify authority decisions that require the rights holder rather than
  inventing them;
- verify current architecture, external-module, root, and site gates;
- keep licensing/package-generation Task 8 stopped while provenance is
  `BLOCKED`.

Acceptance:

- isolated clean worktree from current `origin/main`;
- narrow meaningful commits, each with command receipts;
- no SaaS or theme changes;
- exact independent reviewer verdict for every candidate checkpoint;
- accepted commits integrated into the PM branch promptly.

### OC-02: License, notice, SBOM, and reproducible package gate

Status: blocked on OC-01 provenance `PASS` and explicit rights-holder evidence.

### OC-03: Self-hosted installation and operator lifecycle

Status: queued after packaging boundary is current.

### OC-04: Hybrid SSR/Wasm and offline runtime

Status: Open Core and active as bounded checkpoints. OC-04A adds immutable HTTP
transport for deterministic projection-v2 detail and schema-node shards. OC-04B
adds explicit anonymous-public eligibility, strict immutable descriptor
admission, and the self-hosted composition kill switch while leaving SSR/no-JS
authoritative. OC-04C adds the descriptor-bound immutable manifest transport and
a Wasm-compatible admission prerequisite. OC-04D verifies and strictly decodes
admitted detail/schema-node bytes into exact selected projection records without
parser, network, filesystem, template, or HTML dependencies. OC-04E prepares and
renders a bounded schema-node HTML fragment through one templ-escaped component
shared byte-for-byte with SSR. OC-04F prepares and renders the operation identity
header through one bounded templ-escaped component shared byte-for-byte with
SSR. OC-04G prepares and renders the Path, Query, and Header parameter groups
through one bounded templ-escaped component shared byte-for-byte with SSR.
OC-04H prepares and renders request-body media badges, shallow schema labels,
and optional schema links through one bounded templ-escaped component shared
byte-for-byte with SSR.
These checkpoints are groundwork, not proof of a complete operation body/main
Wasm HTML renderer, browser activation, Service Worker, offline storage,
rollback, tombstones, recursive request-body rendering, response rendering,
parity beyond the four bounded operation fragments, or performance
acceptance.

## PR Gate

At `2026-08-11T18:54:35Z`, [PR
#94](https://github.com/araihu/manja/pull/94) is `MERGED` via squash at exact
`9733949b8dde9eb0fe1ef285ceb3ecbeb88f5d06`, tree
`10c75d6c0fa3e945093bc47e46df0d33e0ad40de`. Local PM ref and worktree
`coord/opencore-product` remain preserved at
`d520edd5b6115904fbed247729dcf997716e2d03`, tree
`eb9c05086b0f6d0380fc5da593ef96c8e3febd2d`. Remote
`refs/heads/coord/opencore-product` is absent after the authorized squash merge;
this checkpoint does not recreate or push it.

[PR #96](https://github.com/araihu/manja/pull/96) was squash-merged with
authorization at `2026-08-12T02:12:14Z`. Base was exact
`9733949b8dde9eb0fe1ef285ceb3ecbeb88f5d06`; source head was exact
`ad9d114ede2fe8b5337331eee8268e830a0e4577`; merge commit is exact
`43f96dfbf9d18eee2364f14778e6b94312c8abac`, tree
`959f199e145c316b4b76e40a561413c0e6d57134`, with the base as its sole parent.
Exact-head CI retry, CodeQL, CodeRabbit, and independent review passed only for
that immutable source head.

Before merge:

- head SHA and tree match reviewed candidate;
- worktree is clean: `git status --porcelain=v1 --untracked-files=all` emits no
  output, so staged, unstaged, and untracked state are all empty;
- relevant root, site, architecture, generation, and artifact gates pass;
- CodeQL check exists and succeeds;
- CodeRabbit is present and completes a substantive successful review/check
  for the final moving candidate; absence, a rate-limited no-review, or a
  presence-only status blocks merge;
- every actionable finding is fixed and rereviewed;
- final PR scope contains Open Core work only.

## Next Action

Submit the frozen OC-04H identity, clean status, the accepted OC-04G parent,
strict media/root-node/href RED/GREEN receipts, exact zero/one/multiple
whole-endpoint SSR parity, root/nested real-handler proof, preserved
OC-04A/B/C/D/E/F/G behavior, Wasm boundary, race, and root-suite
receipts for fresh independent technical and design review. PM chooses and
separately authorizes any integration path.
Any push, PR, or head movement restarts exact-head CI, CodeQL, independent
review, and substantive CodeRabbit gates; absence or failure blocks integration.
Overall provenance remains `BLOCKED`; legal authority and final-artifact
notices stay separate, and licensing/package-generation Task 8 remains stopped.
Hosted SaaS stays deferred, active-theme work stays excluded, and OC-04 hybrid
SSR/Wasm/offline remains Open Core; complete operation body/main rendering, browser ABI
and activation, Service Worker, offline storage, rollback, tombstones,
kill-switch lifecycle, and UI work stay separate from these operation-header
and operation-parameters fragment checkpoints. No push, merge, release,
deployment, cleanup, or other lifecycle action is authorized here.
