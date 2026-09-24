"use client";

import { motion, type Variants } from "framer-motion";
import { ease } from "@/lib/motion";

const SPARK_W = 200;
const SPARK_H = 24;

/**
 * One period of samples (0..1), drawn twice side by side. First and last match,
 * so the strip can slide by exactly one period and end where it started.
 */
function sparkPath(samples: number[]) {
  const step = SPARK_W / (samples.length - 1);
  const pts: string[] = [];
  for (let copy = 0; copy < 2; copy++) {
    samples.forEach((v, i) => {
      if (copy === 1 && i === 0) return;
      const x = copy * SPARK_W + i * step;
      const y = SPARK_H - 2 - v * (SPARK_H - 4);
      pts.push(`${pts.length ? "L" : "M"}${x.toFixed(1)} ${y.toFixed(1)}`);
    });
  }
  return pts.join(" ");
}

type Meter =
  | { label: string; kind: "spark"; value: string; color: string; path: string; scroll: number }
  | { label: string; kind: "bar"; value: string; color: string; fill: number };

const METERS: Meter[] = [
  {
    label: "CPU",
    kind: "spark",
    value: "12%",
    color: "var(--data-1)",
    scroll: 4.2,
    path: sparkPath([0.2, 0.35, 0.18, 0.5, 0.28, 0.22, 0.62, 0.3, 0.16, 0.4, 0.24, 0.2]),
  },
  { label: "RAM", kind: "bar", value: "41%", color: "var(--data-3)", fill: 0.41 },
  { label: "Disk", kind: "bar", value: "62%", color: "var(--data-4)", fill: 0.62 },
  {
    label: "Net",
    kind: "spark",
    value: "2.4 MB/s",
    color: "var(--data-2)",
    scroll: 3.4,
    path: sparkPath([0.1, 0.12, 0.7, 0.4, 0.15, 0.1, 0.3, 0.85, 0.2, 0.12, 0.1]),
  },
];

const draw: Variants = {
  start: { pathLength: 0 },
  end: (i: number) => ({ pathLength: 1, transition: { delay: 0.1 + i * 0.12, duration: 1.3, ease: ease.out } }),
};
/**
 * One-shot scroll: the double-width strip slides one period left, a transform
 * string on an HTML wrapper (compositor-friendly, no per-frame SVG repaint).
 * It plays when the card enters view and on hover, lasts under 5s (WCAG 2.2.2),
 * and the "end" frame looks identical to "start", which is where reduced
 * motion rests.
 */
const slide: Variants = {
  start: { transform: "translateX(0%)" },
  end: (m: { scroll: number }) => ({
    transform: "translateX(-50%)",
    transition: { duration: m.scroll, ease: "linear" },
  }),
};
const grow: Variants = {
  start: { scaleX: 0 },
  end: (i: number) => ({ scaleX: 1, transition: { delay: 0.1 + i * 0.12, duration: 0.8, ease: ease.out } }),
};

export function StatusArt() {
  return (
    <div
      role="img"
      aria-label="du status shows a live dashboard: CPU 12%, RAM 41%, disk 62% and network 2.4 MB/s."
      className="flex h-full flex-col font-mono text-xs"
    >
      <div className="flex items-center justify-between">
        <span className="text-ink-faint">
          C:\&gt; <span className="text-ink-muted">du status</span>
        </span>
        <span className="flex items-center gap-1.5 text-ink-muted">
          <span className="size-1.5 rounded-full bg-ok" />
          live
        </span>
      </div>

      <div className="mt-2 flex flex-1 flex-col justify-center gap-3.5">
        {METERS.map((m, i) => (
          <div key={m.label} className="grid grid-cols-[2.5rem_1fr_4.5rem] items-center gap-3">
            <span className="text-ink-muted">{m.label}</span>
            {m.kind === "spark" ? (
              <span className="relative block h-6 overflow-hidden" aria-hidden="true">
                <span className="absolute inset-x-0 bottom-0 h-px bg-hairline" />
                <motion.span custom={m} variants={slide} className="absolute inset-y-0 left-0 block w-[200%]">
                  <svg
                    viewBox={`0 0 ${SPARK_W * 2} ${SPARK_H}`}
                    preserveAspectRatio="none"
                    className="block h-full w-full"
                  >
                    <motion.path
                      custom={i}
                      variants={draw}
                      d={m.path}
                      fill="none"
                      stroke={m.color}
                      strokeWidth="1.5"
                      strokeLinejoin="round"
                      strokeLinecap="round"
                    />
                  </svg>
                </motion.span>
              </span>
            ) : (
              <span className="relative h-1.5 overflow-hidden rounded-full bg-white/[0.06]">
                <motion.span
                  custom={i}
                  variants={grow}
                  className="absolute inset-y-0 left-0 origin-left rounded-full"
                  style={{ width: `${m.fill * 100}%`, background: m.color }}
                />
              </span>
            )}
            <span className="text-right tabular-nums text-ink">{m.value}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
