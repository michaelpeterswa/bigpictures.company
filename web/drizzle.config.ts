import { config as loadEnv } from "dotenv";
import { defineConfig } from "drizzle-kit";

// dotenv defaults to ".env" but Next.js dev uses ".env.local". Load both —
// .env.local wins because it's read second.
loadEnv({ path: ".env" });
loadEnv({ path: ".env.local", override: true });

// drizzle is **introspect-only** here. The SQL migrations under ../migrations
// (driven by the Go CLI via golang-migrate) are the single source of truth
// for the schema. Run `pnpm db:introspect` after any migration to regenerate
// drizzle/schema.ts. Never run `drizzle-kit push` against this project.
export default defineConfig({
  out: "./drizzle",
  schema: "./drizzle/schema.ts",
  dialect: "postgresql",
  dbCredentials: {
    url: process.env.DATABASE_URL!,
  },
  // Skip PostGIS internals (spatial_ref_sys, geography_columns, geometry_columns)
  // — they're created by the extension and have nothing to do with our schema.
  tablesFilter: ["panoramas"],
  // camel-case the generated TypeScript while keeping snake_case in the DB,
  // so app code reads `panoramas.capturedAt` not `panoramas.captured_at`.
  introspect: {
    casing: "camel",
  },
});
