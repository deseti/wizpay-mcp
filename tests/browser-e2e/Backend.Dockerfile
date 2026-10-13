# Offline artifact-only build. No dependency/image downloads are authorized here.
FROM golang:1.26.3-bookworm
ARG PHASE_B_RUN_ID
COPY --chown=0:0 --chmod=0555 tests/browser-e2e/runtime/${PHASE_B_RUN_ID}/server /usr/local/bin/server
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/server"]
