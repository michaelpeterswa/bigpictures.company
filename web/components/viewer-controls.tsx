"use client";

import {
  Maximize2,
  Minus,
  Plus,
  RotateCw,
  Target,
} from "lucide-react";
import type OpenSeadragon from "openseadragon";
import { type RefObject } from "react";

type Props = {
  viewerRef: RefObject<OpenSeadragon.Viewer | null>;
  zoom: number;
  home: number;
  idle: boolean;
};

export function ViewerControls({ viewerRef, zoom, home, idle }: Props) {
  const v = () => viewerRef.current;
  const pct = home > 0 ? Math.round((zoom / home) * 100) : 100;
  return (
    <div
      data-idle={idle}
      className="pointer-events-none absolute inset-x-0 bottom-6 flex justify-center transition-opacity duration-300 data-[idle=true]:opacity-0"
    >
      <div className="pointer-events-auto flex items-center gap-1 rounded-full border border-white/10 bg-zinc-900/85 px-2 py-1 text-zinc-100 shadow-xl backdrop-blur">
        <Button label="zoom out" onClick={() => v()?.viewport.zoomBy(0.7)}>
          <Minus className="h-4 w-4" />
        </Button>
        <Button label="zoom in" onClick={() => v()?.viewport.zoomBy(1.4)}>
          <Plus className="h-4 w-4" />
        </Button>
        <Divider />
        <span className="px-2 text-xs tabular-nums text-zinc-300">{pct}%</span>
        <Divider />
        <Button label="reset view" onClick={() => v()?.viewport.goHome()}>
          <Target className="h-4 w-4" />
        </Button>
        <Button
          label="rotate 90°"
          onClick={() => {
            const vp = v()?.viewport;
            if (!vp) return;
            vp.setRotation((vp.getRotation() + 90) % 360);
          }}
        >
          <RotateCw className="h-4 w-4" />
        </Button>
        <Button label="fullscreen" onClick={() => v()?.setFullScreen(!v()?.isFullPage())}>
          <Maximize2 className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

function Button({
  label,
  onClick,
  children,
}: {
  label: string;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      className="rounded-full p-2 transition-colors hover:bg-white/10 focus:outline-none focus-visible:ring-2 focus-visible:ring-white/40"
    >
      {children}
    </button>
  );
}

function Divider() {
  return <span className="mx-1 h-4 w-px bg-white/10" aria-hidden />;
}
