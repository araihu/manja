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
go run ./cmd/manja -spec internal/adapters/openapi/testdata/petstore.yaml
```

Open <http://localhost:8080>.
