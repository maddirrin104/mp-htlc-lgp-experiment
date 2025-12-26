FROM ubuntu:22.04

RUN apt-get update && apt-get install -y iproute2 iputils-ping ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# TODO: replace this with your real TSS node binary (Go/Rust + tss-lib)
CMD ["/entrypoint.sh"]
