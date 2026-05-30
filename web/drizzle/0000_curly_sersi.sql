-- Current sql file was generated after introspecting the database
-- If you want to run this migration please uncomment this code before executing migrations
/*
CREATE TABLE "panoramas" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"slug" text NOT NULL,
	"title" text NOT NULL,
	"description" text,
	"captured_at" timestamp with time zone,
	"location" "geography",
	"width" integer NOT NULL,
	"height" integer NOT NULL,
	"tile_path" text NOT NULL,
	"thumb_prefix" text NOT NULL,
	"original_path" text NOT NULL,
	"exif" jsonb,
	"tags" text[] DEFAULT '{""}' NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	CONSTRAINT "panoramas_slug_key" UNIQUE("slug")
);
--> statement-breakpoint
CREATE INDEX "panoramas_captured_at_idx" ON "panoramas" USING btree ("captured_at" timestamptz_ops);--> statement-breakpoint
CREATE INDEX "panoramas_location_gix" ON "panoramas" USING gist ("location" gist_geography_ops);--> statement-breakpoint
CREATE INDEX "panoramas_tags_gin" ON "panoramas" USING gin ("tags" array_ops);
*/