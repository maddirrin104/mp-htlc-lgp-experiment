# Multi-party TSS services (coordinator, gateway, node)

FROM golang:1.22-alpine AS builder
WORKDIR /src

# Copy module files first for caching
COPY go/go.mod go/go.sum* ./go/
COPY go ./go

WORKDIR /src/go
RUN go mod download

ARG CMD
RUN go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${CMD}

FROM alpine:3.20
RUN apk add --no-cache ca-certificates iproute2
COPY --from=builder /out/app /usr/local/bin/app
COPY tssnet/scripts/node_entrypoint.sh /usr/local/bin/node_entrypoint.sh
RUN chmod +x /usr/local/bin/node_entrypoint.sh

# default entrypoint; node uses wrapper to optionally apply tc netem
ENTRYPOINT ["/usr/local/bin/app"]
