# bigpictures.company

A self-hosted gigapixel panorama viewer. A Go CLI (`pano`) tiles gigapixel
TIFFs via `vips dzsave` (IIIF Image API 3.0, WebP, 256px), uploads to
Cloudflare R2, and writes metadata to Neon Postgres. A Next.js app on Fly.io
renders an OpenSeadragon viewer that pulls tiles directly from R2.

The full design lives in [`SPEC.md`](./SPEC.md). The phased build plan lives at
`~/.claude/plans/enchanted-honking-donut.md`.

## Quick start

### Prerequisites

- Go (latest stable, currently 1.25+)
- `libvips` ≥ 8.11 with WebP support (`brew install vips`, etc.)
- Cloudflare R2 bucket + custom domain
- Neon Postgres project (PostGIS extension)

### Build

```
make build
./dist/pano --version
```

### Configure

Copy `.env.example` to `.env` and fill in the values, or export the
variables directly. All CLI config is via `PANO_*` environment variables —
no config file.

```
cp .env.example .env
$EDITOR .env
set -a; source .env; set +a
./dist/pano config show   # debug: print resolved config (secrets redacted)
```

### Migrate the database

```
./dist/pano migrate up
```

### Upload a panorama

```
./dist/pano upload ./mt-rainier.tiff --title "Mt Rainier from Crystal Lookout"
```

### Run the viewer

```
cd web
pnpm install
pnpm dev
# open http://localhost:3000
```

## Repo layout

```
cmd/pano/             # Go CLI entrypoint + subcommands
internal/
  config/             # PANO_* env loader (caarlos/env)
  problems/           # RFC 9457 Problem Details helpers
  version/            # build-info ldflags
  db/                 # pgx + migrations (Phase 1)
  tile/               # vips dzsave wrapper (Phase 2)
  upload/             # R2 parallel uploader (Phase 3)
  exif/               # EXIF extraction (Phase 4)
  pipeline/           # upload orchestrator (Phase 5)
  tui/                # Bubble Tea progress + huh forms (Phase 6)
migrations/           # SQL migrations (Phase 1)
web/                  # Next.js app (Phases 7–9)
fly.toml              # Fly deployment (Phase 10)
```

## Errors

All Go-side errors are RFC 9457 Problem Details. On a TTY they render as
pretty text; piped output gets `application/problem+json` to stderr. Every
problem type URI lives under `https://bigpictures.company/problems/<area>/<kind>`.

## CI

Workflows live in `.github/workflows/` and follow the layout from
[michaelpeterswa/go-start](https://github.com/michaelpeterswa/go-start):

- **`pull_request.yml`** — runs on every PR:
  - `commitlint` against conventional-commits
  - `golangci-lint` (Go 1.25.x)
  - `yamllint`
  - `hadolint` against `web/Dockerfile`
  - `go test -race ./...` (installs `libvips-tools` for the tile-package tests)
  - web pipeline: `pnpm install` → `pnpm lint` → `pnpm tsc --noEmit` → `pnpm build`
- **`push_main.yml`** — runs on merges to `main`:
  - `semantic-release` cuts a GitHub release from conventional-commits.
    Requires a `GH_RELEASE_PAT` repository secret.
- **`deploy_fly.yml`** — runs when a GitHub release is created (also
  available as `workflow_dispatch`):
  - `flyctl deploy --remote-only` against `fly.toml` at the repo root.
    `fly.toml` pins `build.dockerfile = "web/Dockerfile"`, so the build
    context is the repo root and Fly's remote builder handles the
    multi-stage Next.js build.
  - `NEXT_PUBLIC_TILE_BASE_URL` is read from repository **vars** (not
    secrets) and passed through as a build arg so the value is inlined
    into the bundle.

The release → deploy chain: merge a conventional-commit to `main` →
`push_main.yml` cuts a GitHub release → release `created` event fires
`deploy_fly.yml` → Fly rolls out the new machine.

To run every check locally:

```
make ci
```

This runs `golangci-lint run`, `go test -race ./...`, `pnpm lint`,
`pnpm tsc --noEmit`, and `pnpm build` in sequence.
