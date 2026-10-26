# syntax=docker/dockerfile:1

FROM golang:1.27.3-alpine AS build

ARG MANJA_VERSION=dev
ENV GOEXPERIMENT=jsonv2

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

# Assemble the complete filesystem and its notice inventory before scratch.
RUN mkdir -p /out/rootfs/usr/local/bin /out/rootfs/app/renderer \
      /out/rootfs/etc/ssl/certs /out/rootfs/tmp \
 && cp /out/manja-runtime /out/rootfs/usr/local/bin/manja \
 && cp -a /out/renderer-data /out/rootfs/app/renderer-data \
 && cp internal/renderer/testdata/kubernetes/renderer.yaml /out/rootfs/app/renderer/renderer.yaml \
 && cp internal/renderer/testdata/kubernetes/default-allowlist.json /out/rootfs/app/renderer/default-allowlist.json \
 && cp /etc/ssl/certs/ca-certificates.crt /out/rootfs/etc/ssl/certs/ca-certificates.crt \
 && printf 'manja:x:65532:65532:Manja:/app:/sbin/nologin\n' > /out/rootfs/etc/passwd \
 && printf 'manja:x:65532:\n' > /out/rootfs/etc/group \
 && mkdir -p /out/rootfs/usr/share/licenses/manja \
 && { apk info -v ca-certificates ca-certificates-bundle && \
      printf 'Source: https://gitlab.alpinelinux.org/alpine/aports/-/tree/3.24-stable/main/ca-certificates\n'; \
    } > /out/rootfs/usr/share/licenses/manja/ca-certificates-source.txt \
 && CGO_ENABLED=0 go build -trimpath -o /out/oci-package ./cmd/oci-package \
 && /out/oci-package -source /src -root /out/rootfs \
 && chmod 1777 /out/rootfs/tmp \
 && chown -R 65532:65532 /out/rootfs/app

FROM scratch
LABEL org.opencontainers.image.source="https://github.com/araihu/manja" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/rootfs /
ENV HOME=/app
WORKDIR /app
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/manja"]
CMD ["-addr", ":8080", "-renderer-config", "/app/renderer/renderer.yaml", "-data-dir", "/app/renderer-data"]
