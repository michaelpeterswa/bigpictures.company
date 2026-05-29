import { desc } from "drizzle-orm";

import { db } from "@/lib/db";
import { panoramas } from "@/drizzle/schema";
import { PanoCard } from "@/components/pano-card";

// Server component. Data fetching happens on the server; thumbnails are
// served by R2 directly so this route returns a tiny HTML payload.
export const dynamic = "force-dynamic";

export default async function GalleryPage() {
  const rows = await db
    .select({
      slug: panoramas.slug,
      title: panoramas.title,
      capturedAt: panoramas.capturedAt,
      width: panoramas.width,
      height: panoramas.height,
      thumbPrefix: panoramas.thumbPrefix,
    })
    .from(panoramas)
    .orderBy(desc(panoramas.capturedAt), desc(panoramas.createdAt));

  return (
    <main className="mx-auto w-full max-w-6xl px-6 py-12">
      <header className="mb-12">
        <h1 className="text-3xl font-semibold tracking-tight">
          bigpictures
          <span className="text-zinc-400">.company</span>
        </h1>
        <p className="mt-2 text-sm text-zinc-500">
          {rows.length === 0
            ? "no panoramas yet — upload one via the `pano` CLI."
            : `${rows.length} panorama${rows.length === 1 ? "" : "s"}.`}
        </p>
      </header>
      <ul className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
        {rows.map((row) => (
          <li key={row.slug}>
            <PanoCard
              slug={row.slug}
              title={row.title}
              capturedAt={row.capturedAt}
              width={row.width}
              height={row.height}
              thumbPrefix={row.thumbPrefix}
            />
          </li>
        ))}
      </ul>
    </main>
  );
}
