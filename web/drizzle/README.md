# drizzle/

This directory holds **generated** Drizzle ORM bindings, not authoritative
schema. The SQL migrations under `../../migrations` (applied by the Go CLI via
`pano migrate up`) are the only source of truth.

## Regenerate after a migration

```
cd web
pnpm db:introspect
```

That runs `drizzle-kit introspect` against `DATABASE_URL` and rewrites
`schema.ts` from whatever the live database currently contains.

## Do not run

- `drizzle-kit generate` — would create a divergent migration history.
- `drizzle-kit push` — would mutate the database from the TypeScript side.

Both are disabled by convention; the Drizzle config has no migration output
directory because it shouldn't ever produce one.

## Manual fixes after introspect

Drizzle-kit's introspect doesn't perfectly handle two things in our schema.
After each run, re-apply these tweaks (the comments at the top of `schema.ts`
also note them):

1. **`location`**: introspect emits `unknown("location")` because it can't
   parse PostGIS `geography`. Replace with the `geographyPoint("location")`
   customType already defined in `schema.ts`.
2. **Timestamps (`captured_at`, `created_at`)**: change `mode: 'string'` to
   `mode: 'date'`. The app code calls `.toISOString()` on these, which
   requires JS Date objects.

A future iteration could codegen-patch these via a post-introspect script;
for now the manual diff is small and easy to spot.
