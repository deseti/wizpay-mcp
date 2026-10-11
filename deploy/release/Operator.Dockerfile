# Separate owner tooling image. Never used as the public server image.
ARG GO_BASE_IMAGE
FROM ${GO_BASE_IMAGE}
COPY --chown=0:0 --chmod=0555 deploy/release/runtime/onboarding-admin /usr/local/bin/onboarding-admin
COPY --chown=0:0 --chmod=0555 deploy/release/runtime/dbmigrate /usr/local/bin/dbmigrate
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/onboarding-admin"]
