FROM node:22.12-alpine AS builder

# Install git to clone the upstream repo
RUN apk add --no-cache git

# Clone the MCP servers repo and extract everything server
WORKDIR /tmp
RUN git clone --depth 1 https://github.com/modelcontextprotocol/servers.git && \
    mkdir -p /app && \
    cp -r servers/src/everything/* /app/ && \
    cp servers/tsconfig.json /tsconfig.json

WORKDIR /app

RUN --mount=type=cache,target=/root/.npm npm install && npm run build

FROM node:22-alpine AS release

WORKDIR /app

COPY --from=builder /app/dist /app/dist
COPY --from=builder /app/package.json /app/package.json
COPY --from=builder /app/package-lock.json /app/package-lock.json

ENV NODE_ENV=production

RUN npm ci --ignore-scripts --omit-dev

# Run with streamableHttp transport for MCP Gateway compatibility
CMD ["node", "dist/index.js", "streamableHttp"]
