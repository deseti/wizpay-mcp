# Requires separate approval if this exact base is not cached.
# Uses existing owner-built Linux artifacts; installs no npm dependencies.
FROM node:22.22.2-bookworm-slim
WORKDIR /app
COPY --chown=node:node package.json ./package.json
COPY --chown=node:node node_modules ./node_modules
COPY --chown=node:node .next ./.next
USER node
CMD ["node", "node_modules/next/dist/bin/next", "start", "--hostname", "0.0.0.0", "--port", "3000"]
