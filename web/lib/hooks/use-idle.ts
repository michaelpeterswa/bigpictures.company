"use client";

import { useEffect, useState } from "react";

/**
 * useIdle reports whether the pointer has been stationary inside `el` for at
 * least `timeoutMs`. Any mousemove resets the timer. The viewer fades its
 * controls when idle becomes true.
 */
export function useIdle(
  el: HTMLElement | null,
  timeoutMs: number,
): boolean {
  const [idle, setIdle] = useState(false);
  useEffect(() => {
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
  }, [el, timeoutMs]);
  return idle;
}
