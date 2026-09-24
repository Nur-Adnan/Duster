"use client";

import { useRef, useState } from "react";
import { gsap, useScrollScene } from "@/lib/gsap";
import { GB, parseSize } from "./bytes";

/** Six data colors, then two neutrals for the smallest slivers; the legend text carries the meaning. */
const COLORS = [
  "var(--data-1)",
  "var(--data-2)",
  "var(--data-3)",
  "var(--data-4)",
  "var(--data-5)",
  "var(--data-6)",
  "var(--ink-muted)",
  "color-mix(in srgb, var(--ink-muted) 50%, var(--bg-soft))",
];

type Tile = { label: string; value: string };

/**
 * One horizontal "disk" bar, segments proportional to each category, plus a
 * legend. Markup always renders the real sizes; GSAP only animates from zero.
 */
export function DiskBar({ tiles }: { tiles: Tile[] }) {
  const rootRef = useRef<HTMLDivElement>(null);
  const barRef = useRef<HTMLDivElement>(null);
  const totalRef = useRef<HTMLSpanElement>(null);
  // Hover or focus previews a category; a click or tap pins it (useful on touch).
  const [preview, setPreview] = useState<number | null>(null);
  const [pinned, setPinned] = useState<number | null>(null);
  const active = preview ?? pinned;

  const items = tiles
    .map((t) => ({ ...t, bytes: parseSize(t.value) }))
    .sort((a, b) => b.bytes - a.bytes)
    .map((t, i) => ({ ...t, color: COLORS[i % COLORS.length] }));
  const total = items.reduce((sum, t) => sum + t.bytes, 0);
  const totalGB = total / GB;
  const totalText = totalGB.toFixed(1);
  const pct = (bytes: number) => Math.max(1, Math.round((bytes / total) * 100));

  useScrollScene(rootRef, () => {
    const bar = barRef.current;
    const totalEl = totalRef.current;
    if (!bar || !totalEl) return;
    const segments = bar.querySelectorAll<HTMLElement>("[data-segment]");
    const counter = { v: 0 };
    // Write into React's own text node instead of replacing it via textContent.
    const write = (text: string) => {
      if (totalEl.firstChild) totalEl.firstChild.nodeValue = text;
    };
    write("0.0");

    const tl = gsap.timeline({
      scrollTrigger: { trigger: bar, start: "top 85%", toggleActions: "play none none none" },
    });
    tl.fromTo(
      segments,
      { scaleX: 0 },
      { scaleX: 1, duration: 0.6, ease: "power3.out", stagger: 0.09 },
    ).to(
      counter,
      {
        v: totalGB,
        duration: 0.6 + 0.09 * (segments.length - 1),
        ease: "power2.out",
        onUpdate: () => {
          write(counter.v.toFixed(1));
        },
      },
      0,
    );

    // matchMedia revert restores inline styles but not text: put the truth back.
    return () => {
      write(totalText);
    };
  });

  const activeItem = active === null ? null : items[active];

  return (
    <div ref={rootRef} className="rounded-xl border border-hairline bg-bg-soft p-5 sm:p-8">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="font-mono text-xs text-ink-faint">Local Disk (C:)</p>
          <p className="mt-2">
            {/* GSAP counts the visible number up from 0; screen readers get the real total. */}
            <span className="sr-only">{totalText} GB reclaimable</span>
            <span aria-hidden="true" className="flex items-baseline gap-2">
              <span className="text-[clamp(2.5rem,6vw,4rem)] font-semibold leading-none tracking-[-0.04em] text-ink tabular-nums">
                <span ref={totalRef}>{totalText}</span>
                <span className="ml-1.5 text-[0.5em] font-medium tracking-normal text-ink-muted">GB</span>
              </span>
              <span className="text-sm text-ink-muted">reclaimable</span>
            </span>
          </p>
        </div>
        <p className="text-sm text-ink-muted">
          {activeItem ? (
            <>
              <span className="text-ink">{activeItem.label}</span>: {activeItem.value},{" "}
              <span className="tabular-nums">{pct(activeItem.bytes)}%</span> of the total
            </>
          ) : (
            <>{items.length} categories on a first run</>
          )}
        </p>
      </div>

      {/* The legend below carries the same data as text, so the bar itself is decorative. */}
      <div
        ref={barRef}
        aria-hidden="true"
        className="mt-6 flex h-12 gap-0.5 overflow-hidden rounded-lg sm:h-14"
        onMouseLeave={() => setPreview(null)}
      >
        {items.map((item, i) => (
          <div
            key={item.label}
            data-segment
            className="h-full origin-left transition-opacity duration-180"
            style={{
              flexGrow: item.bytes,
              flexBasis: 0,
              background: item.color,
              opacity: active === null || active === i ? 1 : 0.25,
            }}
            onMouseEnter={() => setPreview(i)}
          />
        ))}
      </div>

      <ul className="mt-6 grid gap-x-6 gap-y-1 sm:grid-cols-2 lg:grid-cols-4">
        {items.map((item, i) => (
          <li key={item.label}>
            <button
              type="button"
              aria-pressed={pinned === i}
              onMouseEnter={() => setPreview(i)}
              onMouseLeave={() => setPreview(null)}
              onFocus={() => setPreview(i)}
              onBlur={() => setPreview(null)}
              onClick={() => setPinned(pinned === i ? null : i)}
              className={`flex min-h-11 w-full items-start gap-3 rounded-md px-2 py-3 text-left text-sm leading-5 transition-colors ${
                active === i ? "bg-white/[0.05] text-ink" : "text-ink-muted"
              }`}
            >
              {/* Top-aligned on shared 20px line boxes: a label that wraps grows
                  under itself while swatch, size and percent stay on line one. */}
              <span className="mt-1 size-3 shrink-0 rounded-sm" style={{ background: item.color }} aria-hidden="true" />
              <span className="min-w-0 flex-1">{item.label}</span>
              <span className="w-16 shrink-0 text-right tabular-nums text-ink">{item.value}</span>
              <span
                className={`w-9 shrink-0 text-right text-xs leading-5 tabular-nums ${active === i ? "text-ink-muted" : "text-ink-faint"}`}
              >{pct(item.bytes)}%</span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

