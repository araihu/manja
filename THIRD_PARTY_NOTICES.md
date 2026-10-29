# Third-party notices

Manja is MIT licensed; upstream materials retain their own terms. OCI images
include the complete texts below `/usr/share/licenses/manja`, together with a
SHA-256 file inventory and the modules read from the actual runtime binary.
See [OCI publication](docs/legal/oci-publication.md) for the build and checks.

## Arai Hû Assets v0.2.5

Manja's pinned asset manifest (`araihu-assets.json`) identifies release
`v0.2.5`, revision
`da708ce30a3806757644278df421ae39fa4ec37c`, and release SHA-256
`9f5331dc4b6c075aa2dee9b653b0fd7e5ea67d64069155f20da93777fa1df857`.
The upstream release commit has tree
`30ed94741462571739d00339db62367278f41e13`.

The upstream `LICENSE` is Apache-2.0 (11,358 bytes, Git blob
`d645695673349e3947e8e5ae42332d0ac3164cd7`, SHA-256
`cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30`). The
upstream `NOTICE` is 633 bytes, Git blob
`61949028cb73348c69d488a844a36fb6253b46b7`, SHA-256
`8ab9628587c91891e424abdc1c16fdd8d9a89191e56fe496dadc979994bd6366`.

The exact upstream notice text is:

> Arai Hû Shared Assets
> Copyright 2026 AraiHu
>
> Except where a file or directory carries a more specific notice, this
> repository is licensed under the Apache License, Version 2.0.
>
> Third-party material retains its original license. See LICENSES/ for the
> corresponding notices and license texts.
>
> The Apache License does not grant permission to use Arai Hû project names,
> logos, or trademarks to imply endorsement or affiliation. Projects owned by
> the Arai Hû GitHub organization are expressly authorized to use and
> redistribute the assets in this repository as part of their products,
> documentation, websites, and release artifacts.

The upstream holder/year above apply to Arai Hû Assets only. They are not a
Manja first-party holder/year assertion. The full upstream Apache-2.0 terms and
per-file notices remain bound to the pinned release; asset trademark scope is
not expanded by this file.

The same `LICENSE` and `NOTICE` blobs are present at the independently checked
upstream `origin/main` commit `9a1fce17ad1a99892e81bf3b3b36e7ed48448b63`,
tree `2be5242f515b452052e514c8dd495a95791e5925`. That cross-check does not
replace Manja's pinned release receipt above.

## Goshtoso v0.4.4

Manja resolves `github.com/araihu/goshtoso v0.4.4`. The immutable release
commit is `234eaaf97a7058d137dfed942423dcfb77f2ace6`, tree
`0242bf344b601a019a10b3bc08fc11364f8c4907`, and the upstream root `LICENSE`
is mode `100644`, 1,078 bytes, Git blob
`0a7743398ecbeacc05ed822e1f74023ee9b36842`, SHA-256
`cacf68ff9920c026f5de2ebf992333c1a243e45d81aaa5b4577e05b52c5a9584`.

The upstream root license identifies Goshtoso contributors and MIT terms:

> MIT License
>
> Copyright (c) 2025 Goshtoso contributors
>
> Permission is hereby granted, free of charge, to any person obtaining a copy
> of this software and associated documentation files (the "Software"), to deal
> in the Software without restriction, including without limitation the rights
> to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
> copies of the Software, and to permit persons to whom the Software is
> furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all
> copies or substantial portions of the Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
> IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
> FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
> AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
> LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
> OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
> SOFTWARE.

The upstream holder/year above apply to Goshtoso only. They are not a Manja
first-party holder/year assertion.

The same `LICENSE` blob is present at the independently checked upstream
`origin/main` commit `2c475800dcbe3b08e3a44ac9c857b9f7163335eb`, tree
`4cdc27f08ab89768683b36e9f58dafdd7c83d6ab`. That cross-check does not replace
the exact `v0.4.4` source receipt above.

## Runtime and browser dependencies

The image build reads Go module versions from `manja-runtime`, copies each
module's retained license/notice files (including nested browser libraries),
and includes Go's LICENSE, PATENTS, and vendored license notices. Build-only
Go modules and the compiler are absent from the final scratch image.

The two Manja browser bundles use the packages listed in
[the browser inventory](docs/legal/browser-bundles.md). Complete Muamba-retained
license texts are packaged under `browser/`. Goshtoso's HTMX, Alpine.js and
Heroicons notices and Margo's Mermaid notice are under `go/`.

## API descriptions and icons

Kubernetes OpenAPI data is Apache-2.0 licensed; its full license accompanies
the compiled catalog. GitHub and Stripe OpenAPI descriptions are MIT licensed;
the exact source-revision license texts are retained in `licenses/oci/`.
Their immutable source receipts accompany the image under `provenance/`.

GitHub and Stripe icons derive from Simple Icons 16.28.0, and the Twilio icon
from Simple Icons 9.21.0, under CC0-1.0. Local adaptations add a circular
background and transform the unchanged icon paths. The two CC0 texts and
adaptation receipts are included. Names and marks remain with their owners;
these attributions do not imply endorsement or grant trademark rights.

The strawberry image is the unmodified Twemoji 14.0.2 `1f353.png`, by Twitter,
Inc and other contributors, licensed under CC BY 4.0. Source:
https://github.com/twitter/twemoji/tree/v14.0.2/assets/72x72/1f353.png .
It replaces the previous GitHub-hosted emoji at the same local asset path.
The complete graphics license is included under `upstream/`.

## Certificates and image base

The runtime starts from scratch and includes no Alpine executables, libraries,
or package manager. The PEM CA bundle is copied from the build stage's
`ca-certificates` package, derived from Mozilla's root store (MPL-2.0).
The bundle is distributed in its readable PEM form, with the full MPL-2.0
text, Alpine package version, and upstream source location in the inventory.
Manja's own artwork and social previews are covered by the project license;
the separately sourced Arai Hû brand assets retain the Apache-2.0 terms above.
