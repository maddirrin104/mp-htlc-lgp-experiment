FROM golang:1.23-alpine AS build
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/mpc-node ./cmd/mpc-node

FROM alpine:3.20
RUN apk add --no-cache ca-certificates bash iproute2 curl
WORKDIR /app
COPY --from=build /out/mpc-node /usr/local/bin/mpc-node
ENV LISTEN_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/mpc-node"]
