# Multi-stage build for semsource CLI
FROM golang:1.26.3-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# VERSION is injected into `semsource version` and the startup log. CI passes the
# release tag (see .github/workflows/ci.yml); defaults to "dev" for a plain build.
ARG VERSION=dev
RUN CGO_ENABLED=1 go build \
    -ldflags="-s -w -linkmode external -extldflags '-static' -X main.version=${VERSION}" \
    -o /bin/semsource ./cmd/semsource

# --- Runtime ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates git tzdata

COPY --from=builder /bin/semsource /usr/local/bin/semsource

# Default config location
WORKDIR /etc/semsource

ENTRYPOINT ["semsource"]
CMD ["run", "--config", "/etc/semsource/semsource.json"]
