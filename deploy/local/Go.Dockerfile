# Requires separate approval if this exact base is not cached.
# Build the binary locally first, using existing approved modules, without downloads.
FROM golang:1.26.3-bookworm
COPY --chown=0:0 --chmod=0555 deploy/local/runtime/wizpay-mcp-server /usr/local/bin/wizpay-mcp-server
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/wizpay-mcp-server"]
