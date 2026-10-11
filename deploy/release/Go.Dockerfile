# Required reviewed, cached repository@sha256 reference; no mutable default.
ARG GO_BASE_IMAGE
FROM ${GO_BASE_IMAGE}
COPY --chown=0:0 --chmod=0555 deploy/release/runtime/server /usr/local/bin/server
COPY --chown=0:0 --chmod=0555 deploy/release/runtime/healthcheck /usr/local/bin/healthcheck
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/server"]
