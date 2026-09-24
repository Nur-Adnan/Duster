"use client";

import { motion, type Variants } from "framer-motion";
import { ease } from "@/lib/motion";
import { CheckGlyph, Glyph } from "../ui/icons";
import { formatSize, parseSize, showcaseSize } from "./bytes";

/** Sizes come from the showcase tiles so a category reads the same everywhere on the page. */
const LAYERS = [
  { label: "Browser caches", size: showcaseSize("Browser caches"), color: "var(--data-1)" },
  { label: "Windows Update cache", size: showcaseSize("Windows Update cache"), color: "var(--data-2)" },
  { label: "GPU shader cache", size: showcaseSize("GPU shader cache"), color: "var(--data-3)" },
];
const TOTAL = formatSize(LAYERS.reduce((sum, l) => sum + parseSize(l.size), 0));

const BLOCKS = 24;
const SWEEP_AT = 0.2;
const SWEEP_FOR = 1.15;
const DONE_AT = SWEEP_AT + SWEEP_FOR;

/** Each file block clears the moment the sweep line passes its column. */
const block: Variants = {
  start: { opacity: 1, scale: 1 },
  end: (col: number) => ({
    opacity: 0.1,
    scale: 0.55,
    transition: { delay: SWEEP_AT + (col / BLOCKS) * SWEEP_FOR, duration: 0.28, ease: ease.out },
  }),
};

const sweep: Variants = {
  start: { x: "-100%", opacity: 0 },
  end: {
    x: ["-100%", "0%"],
    opacity: [0, 1, 1, 0],
    transition: {
      x: { delay: SWEEP_AT, duration: SWEEP_FOR, ease: ease.inOut },
      opacity: { delay: SWEEP_AT, duration: SWEEP_FOR + 0.25, times: [0, 0.08, 0.85, 1] },
    },
  },
};

const strike: Variants = {
  start: { scaleX: 0 },
  end: { scaleX: 1, transition: { delay: DONE_AT - 0.1, duration: 0.3, ease: ease.out } },
};

const before: Variants = {
  start: { opacity: 1 },
  end: { opacity: 0, transition: { delay: DONE_AT, duration: 0.2 } },
};

const after: Variants = {
  start: { opacity: 0, y: 4 },
  end: { opacity: 1, y: 0, transition: { delay: DONE_AT + 0.1, duration: 0.35, ease: ease.out } },
};

export function CleanArt() {
  return (
    <div
      role="img"
      aria-label={`du clean sweeps three cache layers: browser caches ${LAYERS[0].size}, Windows Update cache ${LAYERS[1].size} and GPU shader cache ${LAYERS[2].size}, freeing ${TOTAL}.`}
      className="flex h-full flex-col font-mono text-xs"
    >
      <p className="text-ink-faint">
        C:\&gt; <span className="text-ink-muted">du clean --yes</span>
      </p>

      <div className="relative mt-3 flex flex-1 flex-col justify-center gap-3 overflow-hidden">
        {LAYERS.map((layer) => (
          <div key={layer.label}>
            <div className="flex items-baseline justify-between gap-3">
              <span className="text-ink-muted">{layer.label}</span>
              <span className="relative tabular-nums text-ink">
                {layer.size}
                <motion.span
                  variants={strike}
                  className="absolute inset-x-0 top-1/2 h-px origin-left bg-ink-muted"
                />
              </span>
            </div>
            <div className="mt-1.5 flex gap-0.75">
              {Array.from({ length: BLOCKS }, (_, col) => (
                <motion.span
                  key={col}
                  custom={col}
                  variants={block}
                  className="h-2.5 flex-1 rounded-sm"
                  style={{ background: layer.color }}
                />
              ))}
            </div>
          </div>
        ))}

        {/* The sweep: a vertical accent line with a broom head, crossing left to right. */}
        <motion.div variants={sweep} className="pointer-events-none absolute inset-y-0 left-0 w-full">
          <div className="absolute inset-y-0 right-0 w-10 bg-linear-to-r from-transparent to-[rgb(127_176_255/0.14)]" />
          <div className="absolute inset-y-0 right-0 w-px bg-accent-text" />
          <Glyph width="22" height="22" viewBox="0 0 22 22" className="absolute -right-2.5 top-0 text-accent-text">
            <path d="M11 1v9" />
            <path d="M6 10h10l2 10H4l2-10ZM8.5 14v5M11 14v5M13.5 14v5" className="fill-surface-dark" />
          </Glyph>
        </motion.div>
      </div>

      <div className="relative mt-3 h-4">
        <motion.p variants={before} className="absolute inset-0 text-ink-faint">
          Found <span className="tabular-nums text-ink-muted">{TOTAL}</span> in 3 categories
        </motion.p>
        <motion.p variants={after} className="absolute inset-0 flex items-center gap-1.5 text-ok">
          <CheckGlyph className="size-3.5" />
          <span>
            <span className="tabular-nums">{TOTAL}</span> freed, logged
          </span>
        </motion.p>
      </div>
    </div>
  );
}
