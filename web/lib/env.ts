import { createEnv } from "@t3-oss/env-nextjs";
import { z } from "zod";

// Single source of truth for env validation. Throws at startup if any required
// var is missing — much better than surfacing as a runtime SQL error halfway
// through rendering a page.
export const env = createEnv({
  server: {
    DATABASE_URL: z.string().url(),
  },
  client: {
    NEXT_PUBLIC_TILE_BASE_URL: z.string().url(),
  },
  experimental__runtimeEnv: {
    NEXT_PUBLIC_TILE_BASE_URL: process.env.NEXT_PUBLIC_TILE_BASE_URL,
  },
});
