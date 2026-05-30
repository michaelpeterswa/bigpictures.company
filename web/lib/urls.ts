import { env } from "@/lib/env";

// All public-facing URLs the browser hits live under TILE_BASE_URL. Keeping
// the helpers here means the rest of the app never has to know the layout —
// rename, restructure, or move buckets in one place.
const base = env.NEXT_PUBLIC_TILE_BASE_URL.replace(/\/$/, "");

export function tileBaseURL(path: string): string {
  return `${base}/${path.replace(/^\//, "")}`;
}

export function infoJSONURL(tilePath: string): string {
  return tileBaseURL(`${tilePath}/info.json`);
}

export function thumbURL(thumbPrefix: string, width: 300 | 600 | 1200): string {
  return tileBaseURL(`${thumbPrefix}/thumb-${width}.webp`);
}

export function viewerHref(slug: string): string {
  return `/p/${slug}`;
}
