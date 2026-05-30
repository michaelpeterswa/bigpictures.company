"use client";

import { useEffect, useRef } from "react";

import { useIdle } from "@/lib/hooks/use-idle";
import { useOpenSeadragon } from "@/lib/hooks/use-osd";

import { ViewerControls } from "./viewer-controls";

type Props = {
  infoJsonURL: string;
  title: string;
};

const idleMs = 2_000;

export function Viewer({ infoJsonURL, title }: Props) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const wrapperRef = useRef<HTMLDivElement | null>(null);
  const { viewer, zoom, home } = useOpenSeadragon({ containerRef, infoJsonURL });
  const idle = useIdle(wrapperRef, idleMs);

  // Keyboard shortcuts not already handled by OSD. OSD owns +/-/arrows;
  // we add `0` (home), `r` (rotate), `f` (fullscreen).
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const v = viewer.current;
      if (!v) return;
      if (e.target && (e.target as HTMLElement).tagName === "INPUT") return;
      switch (e.key) {
        case "0":
          v.viewport.goHome();
          break;
        case "r":
        case "R":
          v.viewport.setRotation((v.viewport.getRotation() + 90) % 360);
          break;
        case "f":
        case "F":
          v.setFullScreen(!v.isFullPage());
          break;
        default:
          return;
      }
      e.preventDefault();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [viewer]);

  return (
    <div
      ref={wrapperRef}
      className="relative h-[calc(100vh-3.5rem)] w-full overflow-hidden bg-black"
    >
      <div ref={containerRef} className="absolute inset-0" aria-label={`viewer for ${title}`} />
      <ViewerControls viewerRef={viewer} zoom={zoom} home={home} idle={idle} />
    </div>
  );
}
