import Link from "next/link";

import { thumbURL, viewerHref } from "@/lib/urls";

type Props = {
  slug: string;
  title: string;
  capturedAt: Date | null;
  width: number;
  height: number;
  thumbPrefix: string;
};

// Card uses an aspect-ratio container computed from the source dimensions, so
// we can use <Image fill> without storing per-thumbnail dimensions in the
// database. R2 serves thumbnails directly — Next's image optimizer is off.
export function PanoCard({
  slug,
  title,
  capturedAt,
  width,
  height,
  thumbPrefix,
}: Props) {
  const aspect = `${width}/${height}`;
  return (
    <Link
      href={viewerHref(slug)}
      className="group block overflow-hidden rounded-lg border border-zinc-200 transition-shadow hover:shadow-lg dark:border-zinc-800"
    >
      <div
        className="relative w-full overflow-hidden bg-zinc-100 dark:bg-zinc-900"
        style={{ aspectRatio: aspect }}
      >
        {/*
          Plain <img> rather than next/image: thumbnails are pre-sized at
          upload time (300/600/1200) and we set images.unoptimized in
          next.config.ts. Using the native element lets us hand the browser a
          real srcSet — the optimizer would just rewrite that.
        */}
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={thumbURL(thumbPrefix, 600)}
          alt={title}
          sizes="(min-width: 1024px) 33vw, (min-width: 640px) 50vw, 100vw"
          srcSet={[
            `${thumbURL(thumbPrefix, 300)} 300w`,
            `${thumbURL(thumbPrefix, 600)} 600w`,
            `${thumbURL(thumbPrefix, 1200)} 1200w`,
          ].join(", ")}
          loading="lazy"
          decoding="async"
          className="absolute inset-0 h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.02]"
        />
      </div>
      <div className="flex items-baseline justify-between gap-4 p-4">
        <h2 className="truncate text-sm font-medium">{title}</h2>
        {capturedAt && (
          <time
            className="shrink-0 text-xs text-zinc-500"
            dateTime={capturedAt.toISOString()}
          >
            {capturedAt.toISOString().slice(0, 10)}
          </time>
        )}
      </div>
    </Link>
  );
}
