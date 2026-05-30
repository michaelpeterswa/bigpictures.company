import type { NextConfig } from "next";

const tileHostname = (() => {
  const url = process.env.NEXT_PUBLIC_TILE_BASE_URL ?? "https://tiles.bigpictures.company";
  try {
    return new URL(url).hostname;
  } catch {
    return "tiles.bigpictures.company";
  }
})();

const nextConfig: NextConfig = {
  output: "standalone",
  images: {
    // Thumbnails are pre-sized at upload time (300/600/1200). Skipping the
    // optimizer keeps Fly CPU low and avoids re-encoding already-WebP images.
    unoptimized: true,
    remotePatterns: [
      { protocol: "https", hostname: tileHostname },
    ],
  },
};

export default nextConfig;
