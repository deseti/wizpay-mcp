FROM golang:1.26.3-bookworm
COPY --chown=0:0 --chmod=0555 deploy/release/runtime/server /usr/local/bin/server
COPY --chown=0:0 --chmod=0555 deploy/release/runtime/healthcheck /usr/local/bin/healthcheck
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/server"]
