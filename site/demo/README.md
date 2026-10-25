# Static public demo inputs

The demo keeps the original GitHub and Stripe specs, versions and provenance
receipts. It replaces the larger Kubernetes catalog with the official Twilio
Messaging API (58 operations, 38 named schemas, 491,482 source bytes).

- Upstream: https://github.com/twilio/twilio-oai
- Commit: `d9775142962ea67d5f3d50c5021a23ae6d330c5b`
- File: `spec/json/twilio_messaging_v1.json`
- SHA-256: `28993c66048625cb9421c8534609d9b3573365d0e25a5170b5026806634b3f9a`
- Specification license: Apache 2.0 (`info.license`), preserved in the source.
- Repository license: MIT, including Twilio's copyright notice.

The spec is acquired during the build from its pinned Git commit, with no
changes to its bytes. `twilio-openapi.integrity.json` verifies the commit, tree,
blob identity, size and SHA-256. The source stays in build state rather than Git
tracking the upstream example account identifiers.
Both license texts and attribution are published at
`/manja-assets/twilio-messaging-notices.txt` and linked from the Twilio catalog.
The original Kubernetes fixture remains available for renderer tests.

The Twilio catalog mark uses the Simple Icons symbol with the same circular
wrapper as the GitHub and Stripe marks. `twilio-mark.provenance.json` records
the pinned source revision, CC0 license identity, unchanged path data and local
SVG hashes. Both icon files ship as local static assets.
