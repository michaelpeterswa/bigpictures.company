"use client";

import { useEffect, useState, type RefObject } from "react";

/**
 * useIdle reports whether the pointer has been stationary inside the element
 * pointed to by `ref` for at least `timeoutMs`. Any mousemove resets the
 * timer. The viewer fades its controls when idle becomes true.
 *
 * Takes a RefObject (not the element directly) because React 19's
 * react-hooks/refs rule forbids reading `ref.current` during render. Reading
 * inside the effect is fine.
 */
export function useIdle(
  ref: RefObject<HTMLElement | null>,
  timeoutMs: number,
): boolean {
  const [idle, setIdle] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const reset = () => {
      setIdle(false);
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => setIdle(true), timeoutMs);
    };
    reset();
    el.addEventListener("mousemove", reset);
    el.addEventListener("pointerdown", reset);
    el.addEventListener("touchstart", reset);
    return () => {
      el.removeEventListener("mousemove", reset);
      el.removeEventListener("pointerdown", reset);
      el.removeEventListener("touchstart", reset);
      if (timer) clearTimeout(timer);
    };
  }, [ref, timeoutMs]);
  return idle;
}
