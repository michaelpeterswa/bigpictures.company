# Next.js standalone build for Fly.io. No libvips here — tiling happens locally
# via the `pano` CLI before anything ever reaches the web tier.
#
# Build context is the REPO ROOT (because fly.toml lives there). All COPY
# paths are therefore relative to the repo root, not to web/.

FROM node:22-alpine AS deps
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
RUN corepack enable
WORKDIR /app
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

FROM node:22-alpine AS build
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
RUN corepack enable
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY web/ .
# These env vars must be set so @t3-oss/env-nextjs's build-time validation
# passes. They are not baked into the bundle — runtime values come from
# Fly secrets. NEXT_PUBLIC_* exceptions are inlined at build time so they
# must match the prod value.
ARG NEXT_PUBLIC_TILE_BASE_URL
ENV NEXT_PUBLIC_TILE_BASE_URL=${NEXT_PUBLIC_TILE_BASE_URL}
# `next build` collects page data, which evaluates server modules that import
# `@/lib/env` and run `createEnv`. Even with SKIP_ENV_VALIDATION, the t3-env
# import chain pulls DATABASE_URL through `z.string().url()`, which rejects
# undefined. A throwaway URL satisfies the schema at build time; the real
# value comes from Fly secrets at runtime.
ENV SKIP_ENV_VALIDATION=1
ENV DATABASE_URL=postgres://build:build@localhost/build
RUN pnpm build

FROM node:22-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV PORT=3000
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
COPY --from=build /app/public ./public
EXPOSE 3000
CMD ["node", "server.js"]
