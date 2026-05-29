// Drizzle introspect generates this file from the live database. Two manual
// fixes are required after each regeneration; see drizzle/README.md.
//
//   1. Replace `unknown("location")` with the `geographyPoint("location")`
//      customType defined below. Drizzle can't parse PostGIS geography.
//   2. Change timestamp `mode: 'string'` to `mode: 'date'` so app code gets
//      JS Date objects (used in toISOString() calls).

import { customType, index, integer, jsonb, pgTable, text, timestamp, unique, uuid } from "drizzle-orm/pg-core";

// PostGIS geography(point) doesn't have a first-party Drizzle type; we treat
// it as opaque on the TS side. The viewer doesn't decode lat/lon — that lives
// in the EXIF JSONB blob.
const geographyPoint = customType<{ data: Buffer; driverData: string }>({
  dataType() {
    return "geography(point, 4326)";
  },
});

export const panoramas = pgTable(
  "panoramas",
  {
    id: uuid().defaultRandom().primaryKey().notNull(),
    slug: text().notNull(),
    title: text().notNull(),
    description: text(),
    capturedAt: timestamp("captured_at", { withTimezone: true, mode: "date" }),
    location: geographyPoint("location"),
    width: integer().notNull(),
    height: integer().notNull(),
    tilePath: text("tile_path").notNull(),
    thumbPrefix: text("thumb_prefix").notNull(),
    originalPath: text("original_path").notNull(),
    exif: jsonb(),
    tags: text().array().notNull().default([]),
    createdAt: timestamp("created_at", { withTimezone: true, mode: "date" }).defaultNow().notNull(),
  },
  (t) => [
    index("panoramas_captured_at_idx").using("btree", t.capturedAt.desc().nullsFirst().op("timestamptz_ops")),
    index("panoramas_location_gix").using("gist", t.location.asc().nullsLast().op("gist_geography_ops")),
    index("panoramas_tags_gin").using("gin", t.tags.asc().nullsLast().op("array_ops")),
    unique("panoramas_slug_key").on(t.slug),
  ],
);

export type Panorama = typeof panoramas.$inferSelect;
