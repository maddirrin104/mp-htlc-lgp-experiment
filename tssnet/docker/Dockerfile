# Multi-party TSS services (coordinator, gateway, node)

FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src

# 1) Copy go.mod/go.sum trước để cache dependency layer
COPY go/go.mod go/go.sum* ./
RUN go mod download

# 2) Copy TOÀN BỘ contents của thư mục go/ vào module root (/src)
#    (quan trọng: dùng go/. chứ không phải go)
COPY go/. ./

ARG CMD
RUN go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${CMD}

FROM alpine:3.20
RUN apk add --no-cache ca-certificates iproute2
COPY --from=builder /out/app /usr/local/bin/app
COPY tssnet/scripts/node_entrypoint.sh /usr/local/bin/node_entrypoint.sh
RUN chmod +x /usr/local/bin/node_entrypoint.sh

# default entrypoint; node uses wrapper to optionally apply tc netem
ENTRYPOINT ["/usr/local/bin/app"]
