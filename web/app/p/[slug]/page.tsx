import { eq } from "drizzle-orm";
import { notFound } from "next/navigation";
import Link from "next/link";

import { db } from "@/lib/db";
import { panoramas } from "@/drizzle/schema";
import { infoJSONURL, thumbURL } from "@/lib/urls";
import { Viewer } from "@/components/viewer";

import type { Metadata } from "next";

type PageProps = {
  params: Promise<{ slug: string }>;
};

async function getPanoramaBySlug(slug: string) {
  const rows = await db
    .select({
      slug: panoramas.slug,
      title: panoramas.title,
      description: panoramas.description,
      capturedAt: panoramas.capturedAt,
      tilePath: panoramas.tilePath,
      thumbPrefix: panoramas.thumbPrefix,
      width: panoramas.width,
      height: panoramas.height,
    })
    .from(panoramas)
    .where(eq(panoramas.slug, slug))
    .limit(1);
  return rows[0];
}

export async function generateMetadata(
  { params }: PageProps,
): Promise<Metadata> {
  const { slug } = await params;
  const pano = await getPanoramaBySlug(slug);
  if (!pano) return {};
  return {
    title: `${pano.title} — bigpictures.company`,
    description: pano.description ?? undefined,
    openGraph: {
      title: pano.title,
      images: [{ url: thumbURL(pano.thumbPrefix, 1200) }],
    },
  };
}

export default async function ViewerPage({ params }: PageProps) {
  const { slug } = await params;
  const pano = await getPanoramaBySlug(slug);
  if (!pano) notFound();

  return (
    <>
      <header className="flex h-14 items-center justify-between border-b border-zinc-200 bg-white px-4 dark:border-zinc-800 dark:bg-zinc-950">
        <Link href="/" className="text-sm text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100">
          ← gallery
        </Link>
        <h1 className="truncate text-sm font-medium">{pano.title}</h1>
        <div className="text-xs text-zinc-500 tabular-nums">
          {pano.width.toLocaleString()}×{pano.height.toLocaleString()}
        </div>
      </header>
      <Viewer infoJsonURL={infoJSONURL(pano.tilePath)} title={pano.title} />
    </>
  );
}
