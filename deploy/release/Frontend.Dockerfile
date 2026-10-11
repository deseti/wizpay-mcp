# Artifacts must be freshly built in the isolated release workspace.
ARG FRONTEND_BASE_IMAGE
FROM ${FRONTEND_BASE_IMAGE}
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1
COPY --chown=0:0 deploy/release/runtime/frontend/package.json ./package.json
COPY --chown=0:0 deploy/release/runtime/frontend/node_modules ./node_modules
COPY --chown=0:0 deploy/release/runtime/frontend/.next ./.next
USER node
CMD ["node", "node_modules/next/dist/bin/next", "start", "--hostname", "0.0.0.0", "--port", "3000"]
