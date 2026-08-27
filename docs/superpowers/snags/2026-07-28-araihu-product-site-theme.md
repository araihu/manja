# Arai Hû Theme On The Manja Product Site

- Surface: standalone Manja product site under `site/`.
- Symptom: the renderer/demo already loaded the canonical Arai Hû stylesheet,
  but the product site kept a separate teal palette and no `data-theme` root.
- Resolution: embed the same canonical stylesheet in the site, load it before
  `site.css`, set `data-theme="araihu"`, and map site-specific aliases to the
  organization semantic tokens. The site keeps its layout CSS without owning a
  competing palette.
- Verification snag: `npm ci` restored JavaScript tooling but not Playwright's
  Chromium binary; root `go test ./...` therefore failed only in browser tests
  with the missing executable prompt. Generic `npx playwright install` fetched
  a newer incompatible revision; use
  `go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6100.0 install chromium`
  to match `go.mod` before the focused theme/mode E2E gate.
