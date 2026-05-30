"use client";

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <main className="mx-auto flex w-full max-w-xl flex-col items-start gap-4 px-6 py-24">
      <h1 className="text-2xl font-semibold">something broke</h1>
      <p className="text-sm text-zinc-500">
        {error.message || "unknown error"}
      </p>
      <button
        type="button"
        onClick={reset}
        className="rounded border border-zinc-300 px-3 py-1.5 text-sm hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-900"
      >
        retry
      </button>
    </main>
  );
}
