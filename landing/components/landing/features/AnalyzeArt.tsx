"use client";

import { motion, type Variants } from "framer-motion";
import { ease } from "@/lib/motion";
import { showcaseSize } from "./bytes";

type Tile = { name: string; size: string; x: number; y: number; w: number; h: number; color: string };

/**
 * Layout units on a 280 x 160 grid, placed as percentages. Small tiles stack in
 * one right-hand column so every label keeps 12px text down to the narrowest
 * card (about 240px wide at the sm breakpoint).
 */
const W = 280;
const H = 160;

/** C:\Users\dev, before drilling in. AppData is the heavy folder. */
const ROOT: Tile[] = [
  { name: "AppData", size: "18.4 GB", x: 0, y: 0, w: 156, h: 160, color: "var(--data-1)" },
  { name: "Projects", size: "9.2 GB", x: 158, y: 0, w: 122, h: 78, color: "var(--data-3)" },
  { name: "Videos", size: "6.1 GB", x: 158, y: 80, w: 122, h: 50, color: "var(--data-2)" },
  { name: "Other", size: "3.3 GB", x: 158, y: 132, w: 122, h: 28, color: "var(--data-4)" },
];

/** npm's cache is the same "npm + pnpm store" the showcase counts, so it shows the same size. */
const NPM = showcaseSize("npm + pnpm store");

/**
 * Inside AppData, drawn at full size once zoomed. Docker's WSL disk is the
 * VM image (du vdisk shrinks it), not the cache du clean counts.
 */
const DRILLED: Tile[] = [
  { name: "Docker disk", size: "9.8 GB", x: 0, y: 0, w: 150, h: 160, color: "var(--data-1)" },
  { name: "JetBrains", size: "3.9 GB", x: 152, y: 0, w: 128, h: 71, color: "var(--data-3)" },
  { name: "npm-cache", size: NPM, x: 152, y: 73, w: 128, h: 51, color: "var(--data-2)" },
  { name: "Other", size: "1.9 GB", x: 152, y: 126, w: 128, h: 34, color: "var(--data-4)" },
];

const HEAVY = ROOT[0];
const ZOOM_AT = 0.55;

const select: Variants = {
  start: { opacity: 0 },
  end: { opacity: [0, 1, 1, 0], transition: { duration: 0.9, times: [0, 0.2, 0.7, 1], delay: 0.1 } },
};

const rootLayer: Variants = {
  start: { opacity: 1 },
  end: { opacity: 0, transition: { delay: ZOOM_AT + 0.1, duration: 0.35 } },
};

/** The drilled layer grows out of AppData's own rectangle. */
const drillLayer: Variants = {
  start: { opacity: 0, scaleX: HEAVY.w / W, scaleY: HEAVY.h / H },
  end: {
    opacity: 1,
    scaleX: 1,
    scaleY: 1,
    transition: { delay: ZOOM_AT, duration: 0.6, ease: ease.out, opacity: { delay: ZOOM_AT, duration: 0.25 } },
  },
};

const crumb: Variants = {
  start: { opacity: 0, x: -6 },
  end: { opacity: 1, x: 0, transition: { delay: ZOOM_AT, duration: 0.35, ease: ease.out } },
};

const totalBefore: Variants = {
  start: { opacity: 1 },
  end: { opacity: 0, transition: { delay: ZOOM_AT, duration: 0.2 } },
};
const totalAfter: Variants = {
  start: { opacity: 0 },
  end: { opacity: 1, transition: { delay: ZOOM_AT + 0.15, duration: 0.3 } },
};

const box = (t: { x: number; y: number; w: number; h: number }) => ({
  left: `${(t.x / W) * 100}%`,
  top: `${(t.y / H) * 100}%`,
  width: `${(t.w / W) * 100}%`,
  height: `${(t.h / H) * 100}%`,
});

/** HTML tiles, not SVG text: labels stay 12px however narrow the card gets. */
function Tiles({ tiles }: { tiles: Tile[] }) {
  return tiles.map((t) => (
    <div key={t.name} className="absolute p-px" style={box(t)}>
      <div
        className={`flex h-full overflow-hidden rounded-sm border-[1.5px] px-1.5 text-xs leading-4 ${
          t.h < 40 ? "items-center gap-1.5" : "flex-col pt-1"
        }`}
        style={{
          borderColor: `color-mix(in srgb, ${t.color} 70%, transparent)`,
          background: `color-mix(in srgb, ${t.color} 16%, transparent)`,
        }}
      >
        <span className="whitespace-nowrap font-medium text-ink">{t.name}</span>
        <span className="whitespace-nowrap font-mono tabular-nums text-ink-muted">{t.size}</span>
      </div>
    </div>
  ));
}

export function AnalyzeArt() {
  return (
    <div
      role="img"
      aria-label="du analyze maps C:\Users\dev as a treemap, then drills into AppData, the heaviest folder at 18.4 GB, where the Docker disk takes 9.8 GB."
      className="flex h-full flex-col"
    >
      <div className="flex items-baseline justify-between gap-3 font-mono text-xs">
        <p className="truncate text-ink-muted">
          C:\Users\dev
          <motion.span variants={crumb} className="inline-block text-accent-text">
            \AppData
          </motion.span>
        </p>
        <span className="relative tabular-nums text-ink">
          <motion.span variants={totalBefore} className="inline-block">
            37.0 GB
          </motion.span>
          <motion.span variants={totalAfter} className="absolute right-0 top-0">
            18.4 GB
          </motion.span>
        </span>
      </div>

      <div className="relative mt-3 min-h-0 flex-1" aria-hidden="true">
        <motion.div variants={rootLayer} className="absolute inset-0">
          <Tiles tiles={ROOT} />
        </motion.div>
        <motion.div
          variants={select}
          className="absolute rounded-sm border-2 border-accent-text"
          style={box(HEAVY)}
        />
        <motion.div variants={drillLayer} className="absolute inset-0" style={{ originX: 0, originY: 0 }}>
          <Tiles tiles={DRILLED} />
        </motion.div>
      </div>
    </div>
  );
}
