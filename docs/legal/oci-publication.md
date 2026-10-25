# OCI publication

The maintainer requested OCI publication and confirmed the MIT copyright
attribution as **Arai Hû, 2026** on 2026-10-25. `LICENSE` contains the operative
MIT terms and `NOTICE` records the attribution. This is the maintainer's
release instruction; it is not an independent legal opinion, assignment,
or permission to claim endorsement by an API or icon owner.

This policy supersedes the provisional first-party files and the unconditional
`OCI_LEGAL_GATE_STATUS = "BLOCKED"` publication constant. The dated August
reviews remain historical records. The generic evidence API in
`internal/distribution` remains available without relaxing its authority
checks; current OCI packaging reuses its complete-filesystem inventory.

## Image contents

The final image starts from `scratch`, runs as numeric UID/GID 65532, and ships
the statically linked `manja-runtime`, its precompiled Kubernetes/GitHub/Stripe
catalogs, renderer config, CA certificates, and notices. Public assets remain
embedded in the runtime. There is no redundant loose static directory, Alpine
shell, package manager, Go compiler, or source compiler in the image. A writable
`/app` and a mode-1777 `/tmp` preserve runtime filesystem requirements.

The OCI `licenses=MIT` annotation describes Manja. Third-party components
retain their own licenses. `/usr/share/licenses/manja` contains:

- project LICENSE, NOTICE and THIRD_PARTY_NOTICES.md;
- the exact module versions/checksums read from the built runtime, with all
  retained upstream license/notice files, including nested browser libraries;
- Go's license, patents, and vendored notices;
- Muamba's browser package licenses and source/version manifest;
- Kubernetes, GitHub, Stripe, Assets, Simple Icons and Twemoji terms and receipts;
- the readable PEM CA bundle's MPL-2.0 terms, package version and source URL;
- `inventory.json`, containing SHA-256, size and mode for every regular image
  file except the inventory itself, plus runtime and browser component lists.

The file inventory is a deterministic packaging record, not an SPDX document
or a claim about trademark ownership. The registry manifest digest identifies
the published image; the inventory does not attempt to contain its own digest.
`licenses/oci/sources.json` pins the retained upstream texts by SHA-256. Their
bytes are verified at build time. The strawberry is unmodified Twemoji artwork
(CC BY 4.0); API marks identify their corresponding catalog without endorsement.

## Checks and publication

`cmd/oci-package` assembles notices and fails on missing licenses, checksum
mismatches, replaced runtime modules, or an incomplete image filesystem. Its
verification mode rejects modified, additional, missing, linked or special
files, checks required notices and the static runtime's module closure.

Dagger pins the build base by digest. `image` and `publish-image` verify the
*final* scratch filesystem using a separate tool container, then start that
image and request the index and all three catalogs. The helper and credentials
never enter the shipped image. Publication repeats verification using the run
nonce before adding registry authentication. Only tested `main` and stable
`vX.Y.Z` tags may publish; pull requests build and smoke-test in a separate parallel job without publishing.

```sh
dagger call image --source=. sync
# Inspect an exported image root with the same verifier:
go run ./cmd/oci-package -verify -root /path/to/rootfs
```

A dependency or content change must update its source receipts/notices and pass
the image checks. The Cloudflare Workers remain separately built static sites;
this change does not move either site back into the OCI runtime.
