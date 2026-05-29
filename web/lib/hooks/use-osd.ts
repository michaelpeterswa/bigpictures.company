"use client";

import { useEffect, useRef, useState } from "react";
import type OpenSeadragon from "openseadragon";

type Options = {
  containerRef: React.RefObject<HTMLDivElement | null>;
  infoJsonURL: string;
};

/**
 * useOpenSeadragon mounts an OSD viewer into the given container and exposes
 * the live zoom level. We disable OSD's default UI here — controls are
 * rebuilt in React + Tailwind via <ViewerControls>.
 *
 * OSD's IIIF 3.0 detection keys off the info.json `@context`, which dzsave
 * writes correctly, so we just pass the URL as `tileSources`.
 */
export function useOpenSeadragon({ containerRef, infoJsonURL }: Options) {
  const viewerRef = useRef<OpenSeadragon.Viewer | null>(null);
  const [zoom, setZoom] = useState(1);
  const [home, setHome] = useState(1);

  useEffect(() => {
    if (!containerRef.current) return;
    let cancelled = false;
    let viewer: OpenSeadragon.Viewer | null = null;

    (async () => {
      const OSD = (await import("openseadragon")).default;
      if (cancelled || !containerRef.current) return;
      viewer = OSD({
        element: containerRef.current,
        tileSources: infoJsonURL,
        showNavigationControl: false,
        showNavigator: false,
        immediateRender: false,
        animationTime: 0.4,
        blendTime: 0.1,
        gestureSettingsMouse: { clickToZoom: false },
      });
      viewerRef.current = viewer;
      viewer.addHandler("open", () => {
        if (!viewer) return;
        setHome(viewer.viewport.getHomeZoom());
        setZoom(viewer.viewport.getZoom());
      });
      viewer.addHandler("zoom", (e) => {
        // e.zoom is the requested final zoom level.
        setZoom(e.zoom);
      });
    })();

    return () => {
      cancelled = true;
      if (viewer) {
        viewer.destroy();
      }
      viewerRef.current = null;
    };
  }, [containerRef, infoJsonURL]);

  return { viewer: viewerRef, zoom, home };
}
