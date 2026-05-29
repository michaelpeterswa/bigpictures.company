# Deployment

End-to-end setup for a fresh deploy. Run these once; subsequent updates are
push-to-main for the web app and tag-push for the CLI.

## 1. Neon Postgres

```
neonctl projects create --name bigpictures
neonctl databases create bigpictures
```

Grab two connection strings:
- **direct** (no `-pooler` in the host) → `PANO_DATABASE_URL` for the CLI
- **pooler** (host contains `-pooler`) → `DATABASE_URL` for Fly

Apply migrations once locally:

```
PANO_DATABASE_URL='<direct url>' pano migrate up
```

## 2. Cloudflare R2

1. Create a bucket: `bigpictures` (or whatever you set in `PANO_R2_BUCKET`).
2. **Settings → Public access → Custom domain** → bind `tiles.bigpictures.company`.
3. **Settings → CORS policy** → import `infra/r2-cors.json`:
   ```
   wrangler r2 bucket cors put bigpictures --file infra/r2-cors.json
   ```
4. Mint an S3 API token: **R2 → Manage API Tokens** → Object Read & Write
   restricted to the bucket. Save the access key ID and secret as
   `PANO_R2_ACCESS_KEY_ID` / `PANO_R2_SECRET_ACCESS_KEY`. The account ID is in
   the right-hand sidebar of the R2 dashboard → `PANO_R2_ACCOUNT_ID`.

Verify CORS landed (R2 caches the policy for a few minutes):

```
curl -i -X OPTIONS https://tiles.bigpictures.company/ \
  -H "Origin: https://bigpictures.company" \
  -H "Access-Control-Request-Method: GET"
```

`Access-Control-Allow-Origin: https://bigpictures.company` should be in the
response. If not, wait a minute and retry — propagation lag is normal.

## 3. Fly.io

```
fly apps create bigpictures-company --org personal
fly secrets set --app bigpictures-company \
  DATABASE_URL='<pooler url>'
fly deploy --build-arg NEXT_PUBLIC_TILE_BASE_URL=https://tiles.bigpictures.company
```

Set a GitHub Actions secret `FLY_API_TOKEN` (run `fly tokens create deploy`)
and a repo variable `NEXT_PUBLIC_TILE_BASE_URL=https://tiles.bigpictures.company`
so the `deploy-web` workflow can take over.

## 4. DNS

Point `bigpictures.company` at the Fly app's IPv4/IPv6:

```
fly ips list --app bigpictures-company
```

`tiles.bigpictures.company` is a CNAME at Cloudflare; R2 wires it up
automatically when you bind the custom domain.

## 5. First panorama

```
export PANO_R2_ACCOUNT_ID=...
export PANO_R2_ACCESS_KEY_ID=...
export PANO_R2_SECRET_ACCESS_KEY=...
export PANO_R2_BUCKET=bigpictures
export PANO_TILE_BASE_URL=https://tiles.bigpictures.company
export PANO_DATABASE_URL='<direct neon url>'

pano upload ./first.tiff --title "First Pano"
```

Open `https://bigpictures.company/` — the thumbnail should appear. Click
through; the viewer's tile requests should hit `tiles.bigpictures.company`
directly, not Fly. Verify in DevTools.

## 6. Releases

CLI:

```
git tag v0.1.0
git push --tags
```

`release-cli` builds macOS + Linux archives via goreleaser and attaches them
to a draft GitHub release.

Web: just merge to `main`. `deploy-web` runs `fly deploy` against any change
under `web/**` or `fly.toml`.
