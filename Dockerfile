# Multi-stage build for semsource CLI
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w -linkmode external -extldflags '-static'" -o /bin/semsource ./cmd/semsource

# --- Runtime ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/semsource /usr/local/bin/semsource

# Default config location
WORKDIR /etc/semsource

ENTRYPOINT ["semsource"]
CMD ["run", "--config", "/etc/semsource/semsource.json"]
