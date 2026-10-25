# Manja Site

This module builds two independent static sites for Cloudflare Workers:

| Worker | Domain | Output | Content |
| --- | --- | --- | --- |
| `manja-site` | `manja.araihu.com` | `dist/site` | Landing page and `/docs` |
| `manja-demo` | `demo.manja.araihu.com` | `dist/demo` | API documentation demo with browser search |

Both Workers use Static Assets without a Worker script, Go server, database or
runtime secrets. The demo uses Manja's verified HTML exporter with the original
GitHub REST API and Stripe specs plus the smaller official Twilio Messaging
spec, configured in `demo/renderer.yaml`. Existing source revisions, profiles
and Stripe provenance receipts are preserved. Management,
sync and publication actions are available only in the local Go workbench.
The GitHub label emoji is bundled as a hash-verified local asset; the OpenAPI
source bytes are preserved. Twilio's spec declares Apache 2.0 and the upstream
repository includes an MIT license. Both license texts and attribution are
published with the demo; see [the input provenance](demo/README.md).

The demo exports 12,329 files (about 150 MB), within the Workers Free limit of
20,000 assets per Worker. Browser acceptance checks reject an export above that limit. Static
asset storage and requests have no usage charge; Workers Paid is unnecessary
for this demo.
See [Cloudflare's limits](https://developers.cloudflare.com/workers/platform/limits/)
and [static asset billing](https://developers.cloudflare.com/workers/static-assets/billing-and-limitations/).

## Build and preview

Run from `site/`, using the Go version declared in `go.mod` and Node >=22.13:

```bash
npm ci
npm run build
npm test
npx playwright install chromium
npm run test:browser
# Each preview runs in its own terminal:
npm run dev:site # http://localhost:8787
npm run dev:demo # http://localhost:8788
```

`build:site` and `build:demo` build each output separately. The demo export
verifies its file inventory, hashes, links, fragments and search artifacts
in `dist/demo-export`, then copies that tree to `dist/demo` with Cloudflare's
`_headers` metadata. Keeping that metadata separate preserves export verification
and incremental reuse. Compiled snapshots remain in `dist/demo-data`, isolated
by configuration hash so removed catalogs cannot survive in navigation or search.
They stay outside the published directory; the Stripe and Twilio sources are
acquired only during the build. Do not add files or modify exported
HTML/JS: the integrity sidecars must match the published bytes. Preview the
demo through HTTP, rather than opening files directly.

The landing's Demo links open the demo domain. Existing `/demo` and `/demo/*`
product-site URLs redirect there. For local demo preview, open port 8788
directly. Unknown demo routes return 404; there is no SPA fallback.

The browser check launches both Workers on available local ports and checks
landing/docs, redirects, operation deep-link reloads, cross-catalog search,
schema navigation, mobile layout and missing-route responses.
It compares every full HTML shell and a distributed sample of assets against
the local export's SHA-256 inventory, requires the published manifest to match
that build, and checks every landing/docs file against its local bytes.
Set `MANJA_VERIFY_ALL_ASSETS=1` to audit every
deployed file. The complete local export is always verified before publication.
Both Workers send `Cache-Control: no-transform` to preserve generated HTML,
including offline shells, against Cloudflare's JavaScript Detections injection.

## Production deployment through CI

The `deploy-sites` job in the root CI workflow runs after successful verification
on pushes to `main`. Pull requests and tags never deploy. A shared concurrency
group serializes production publication. Dagger builds both static trees with
the pinned toolchain, publishes the demo before the landing, and runs the same
browser checks against the public domains.

Configure the repository variable `CLOUDFLARE_ACCOUNT_ID` and repository secret
`CLOUDFLARE_API_TOKEN` before merging. Scope the token to the target account and
the Workers/DNS permissions needed by these custom domains. Credentials are
injected only at publication time and removed before browser validation.

## Manual deployment for initial validation

The initial deployment may be performed through the authenticated Cloudflare
MCP: upload the generated assets, publish each Worker with the routing settings
in its Wrangler config, then assign its custom domain. This uses the same
static output that CI publishes. The MCP connection is separate from Wrangler
authentication and must not be copied into CI.

The initial synthetic-demo deployment passed checks on both
`workers.dev` URLs and the two production domains. The landing domain was moved
from its previous VPS A record to the `manja-site` Worker, and the demo's managed
certificate is active.
The current demo inputs replace Kubernetes with Twilio Messaging to remain
within the Free asset limit while preserving the original GitHub and Stripe
specs.

The zone's legacy `manja_catalogs_per_client` rate-limit rule protects dynamic
catalog rendering at 20 requests per 10 seconds. Its expression excludes only
`demo.manja.araihu.com`: static search and document navigation fetch multiple
catalog assets and would otherwise receive HTTP 429. The rule remains enabled
with its existing threshold for all other hosts.

For a manual Wrangler deployment instead, authenticate against the Cloudflare
account managing `araihu.com`:

```bash
npx wrangler login
npx wrangler whoami
npm run deploy:demo
npm run deploy:site
```

Deploy the demo first so the landing's links have a destination. Each command
builds only its own output and deploys its corresponding configuration:
`wrangler.demo.jsonc` or `wrangler.jsonc`. Each config declares its custom
domain and enables a `workers.dev` URL. The domains must be available for
Workers Custom Domains in the selected account. If the landing domain already
points to another host, resolve that DNS conflict when switching hosting.

No credentials belong in this repository.
For Workers Builds, set the root directory to `site`, the build command to
`npm run build:site` or `npm run build:demo`, and the deploy command to
`npx wrangler deploy --config wrangler.jsonc` or
`npx wrangler deploy --config wrangler.demo.jsonc`, respectively. Make the
repository's Go toolchain available during the build.

After publication, validate the actual domains without starting local Workers:

```bash
MANJA_SITE_URL=https://manja.araihu.com \
MANJA_DEMO_URL=https://demo.manja.araihu.com npm run test:deployed
```

The demo keeps directory-style URLs for catalogs, documents and offline
shells. Preserve `auto-trailing-slash` and the complete generated tree. Avoid
post-build HTML rewriting and blanket immutable caching for fragments,
sidecars, manifests and the service worker. See
[static export](../docs/static-export.md) for the runtime contract.

## Local Go workbench

The original server remains available for local development:

```bash
go run ./cmd/server
go test ./...
```

Local routes:

- `/` product landing page
- `/demo` public OpenAPI renderer demo
- `/docs` setup and publishing docs
