# Manja

Manja is a hosted OpenAPI renderer and publisher built with
[Goshtoso](https://github.com/araihu/goshtoso).

The first vertical slice renders read-only OpenAPI docs from a spec file and
provides Ctrl+K search across indexed operations and schemas.

## Development

```bash
go test ./...
npm ci
npm run api:bundle
npm run api:lint
go run ./cmd/manja -data-dir .manja/data
```

Open <http://localhost:8080>.

For live development, run:

```bash
npm run dev
```

Air watches Go, templ, API description, and static asset sources. On rebuild it
regenerates templ output, rebuilds schema example assets, runs `css:build` when
that npm script exists, and restarts Manja.

Generated templ Go files, static CSS bundles, and the schema example JS bundle
are excluded from the watcher so build outputs do not trigger rebuild loops.

The dev launcher chooses available app and proxy ports per worktree, then prints
the URLs before starting Air. Open the printed Air reload proxy URL, not the app
URL. To pin ports explicitly, run:

```bash
npm run dev -- --app-port 8080 --proxy-port 7331
```

Use `npm run dev -- --print-ports` to inspect the selected pair without
starting Air. Extra flags are passed through to Manja:

```bash
npm run dev -- -spec internal/adapters/openapi/testdata/petstore.yaml
```
