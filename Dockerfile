# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.26.4-bookworm@sha256:5f68ec6805843bd3981a951ffada82a26a0bd2631045c8f7dba483fa868f5ec5 AS deps
WORKDIR /src

ENV CGO_ENABLED=0

COPY .go-version go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    expected="$(cat .go-version)" && \
    actual="$(go env GOVERSION)" && \
    actual="${actual#go}" && \
    if [ "${expected}" != "${actual}" ]; then \
      echo "Go builder version ${actual} does not match .go-version ${expected}" >&2; \
      exit 1; \
    fi && \
    go mod download

FROM deps AS source
COPY . .

FROM source AS test
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go test -mod=readonly ./...

FROM source AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build \
      -mod=readonly \
      -trimpath \
      -buildvcs=false \
      -ldflags="-s -w -buildid= -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /out/template-mcp \
      ./cmd/template-mcp

FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639 AS runtime
ARG VERSION=dev
ARG COMMIT=none
ARG SOURCE=https://github.com/meigma/template-mcp

LABEL org.opencontainers.image.title="template-mcp" \
      org.opencontainers.image.description="Meigma Go MCP server template" \
      org.opencontainers.image.source="${SOURCE}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"

USER 65532:65532
COPY --from=build /out/template-mcp /usr/local/bin/template-mcp
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/template-mcp"]
# Containers are the networked deployment: default to the Streamable HTTP
# transport bound to all interfaces. Origin protection still applies, but a
# non-loopback bind with no auth exposes every tool to any client that can reach
# the port, so --insecure is required to opt into that explicitly. For a real
# deployment, supply authentication (--auth-token, or a real OAuth 2.1 verifier)
# and drop --insecure.
CMD ["http", "--addr", "0.0.0.0:8080", "--insecure"]
