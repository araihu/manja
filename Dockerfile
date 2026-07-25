# syntax=docker/dockerfile:1

FROM golang:1.26.1-alpine AS build

ARG MANJA_VERSION=dev

WORKDIR /src
RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${MANJA_VERSION}" -o /out/manja ./cmd/manja

FROM alpine:3.22

LABEL org.opencontainers.image.source="https://github.com/araihu/manja"

RUN apk add --no-cache ca-certificates git \
	&& addgroup -S manja \
	&& adduser -S -G manja -h /app manja

WORKDIR /app
COPY --from=build /out/manja /usr/local/bin/manja
COPY --from=build /src/internal/web/static ./internal/web/static
COPY --from=build /src/internal/adapters/openapi/testdata/github-v3-rest.json ./internal/adapters/openapi/testdata/github-v3-rest.json

RUN mkdir -p /var/lib/manja \
	&& chown -R manja:manja /app /var/lib/manja

USER manja
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/manja"]
CMD ["-addr", ":8080", "-spec", "/app/internal/adapters/openapi/testdata/github-v3-rest.json", "-data-dir", "/var/lib/manja"]
