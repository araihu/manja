# syntax=docker/dockerfile:1

FROM golang:1.27rc2-alpine AS build

ARG MANJA_VERSION=dev

WORKDIR /src
RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${MANJA_VERSION}" -o /out/manja ./cmd/manja \
	&& CGO_ENABLED=0 GOOS=linux go build -tags=manja_runtime -trimpath -ldflags="-s -w" -o /out/manja-runtime ./cmd/manja-runtime
RUN /out/manja build \
	-renderer-config /src/internal/renderer/testdata/kubernetes/renderer.yaml \
	-data-dir /out/renderer-data \
	> /out/renderer-build-receipt.json

FROM alpine:3.24

LABEL org.opencontainers.image.source="https://github.com/araihu/manja"

RUN apk add --no-cache ca-certificates \
	&& addgroup -S manja \
	&& adduser -S -G manja -h /app manja

WORKDIR /app
COPY --from=build /out/manja-runtime /usr/local/bin/manja
COPY --from=build /out/renderer-data /app/renderer-data
COPY --from=build /src/internal/renderer/testdata/kubernetes/renderer.yaml /app/renderer/renderer.yaml
COPY --from=build /src/internal/renderer/testdata/kubernetes/default-allowlist.json /app/renderer/default-allowlist.json
COPY --from=build /src/internal/web/static ./internal/web/static

RUN chown -R manja:manja /app

USER manja
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/manja"]
CMD ["-addr", ":8080", "-renderer-config", "/app/renderer/renderer.yaml", "-data-dir", "/app/renderer-data"]
