# Manja development tools

This directory is an independent Go module for tools used to develop and
verify Manja. Keeping the tool declarations here prevents generator and audit
commands from becoming direct dependencies of the runtime module.

Run commands from the repository root with `-modfile=tools/go.mod`:

```bash
go tool -modfile=tools/go.mod muamba verify --strict
go run -modfile=tools/go.mod github.com/a-h/templ/cmd/templ generate
go run -modfile=tools/go.mod github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen \
  -generate types,strict-server -package web \
  -o internal/web/api.gen.go api/dist/openapi.yaml
```

The `envdoc` command is normally invoked through
`go generate ./internal/environment`, which keeps its output path and
post-processing in the source directive.
