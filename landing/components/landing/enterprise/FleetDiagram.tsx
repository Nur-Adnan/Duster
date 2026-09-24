"use client";

import { Fragment, useRef } from "react";
import { gsap, useScrollScene } from "@/lib/gsap";

/**
 * "One binary, many workstations": du.exe on the left, wires fanning out to
 * a grid of machines. When it scrolls into view the wires draw at a constant
 * speed, so each machine lights up (and gets its check) the moment its wire
 * arrives. The resting markup is the final, all-ready state.
 */

/**
 * x0/y0: first column / row centre, dx/dy: the step between them.
 * mw/mh: monitor size, badge: check radius, bend: curve reach, sw: stroke width.
 */
const WIDE = {
  w: 720,
  h: 320,
  cols: 4,
  rows: 3,
  file: { cx: 90, cy: 180, w: 96, h: 120 },
  x0: 330,
  dx: 112,
  y0: 90,
  dy: 90,
  mw: 80,
  mh: 50,
  badge: 8,
  bend: 50,
  sw: 1.75,
};

const COMPACT: Layout = {
  w: 360,
  h: 232,
  cols: 3,
  rows: 2,
  file: { cx: 38, cy: 130, w: 56, h: 72 },
  x0: 160,
  dx: 80,
  y0: 80,
  dy: 100,
  mw: 60,
  mh: 40,
  badge: 7,
  bend: 30,
  sw: 1.5,
};

type Layout = typeof WIDE;

/** Wires draw at this many viewBox units per second. */
const SPEED = 480;
const WIRE_START = 0.25;

function stations(L: Layout) {
  const out: { cx: number; cy: number }[] = [];
  for (let r = 0; r < L.rows; r++) {
    for (let c = 0; c < L.cols; c++) out.push({ cx: L.x0 + c * L.dx, cy: L.y0 + r * L.dy });
  }
  return out;
}

const monitorTop = (L: Layout, cy: number) => cy - 4 - L.mh / 2;

/** File port -> lane above the station's row -> along the lane -> down into the station. */
function wirePath(L: Layout, cx: number, cy: number) {
  const sx = L.file.cx + L.file.w / 2 + 4;
  const sy = L.file.cy;
  const top = monitorTop(L, cy);
  const lane = top - 14;
  const r = 6;
  return [
    `M${sx} ${sy}`,
    `C${sx + L.bend} ${sy} ${sx + L.bend} ${lane} ${sx + 2 * L.bend} ${lane}`,
    `H${cx - r}`,
    `Q${cx} ${lane} ${cx} ${lane + r}`,
    `V${top}`,
  ].join(" ");
}

function FileNode({ L }: { L: Layout }) {
  const { cx, cy, w, h } = L.file;
  const x = cx - w / 2;
  const y = cy - h / 2;
  const fold = w * 0.28;
  const bar = (dy: number, len: number) => `M${x + w * 0.22} ${y + h * dy}h${w * len}`;
  return (
    <>
      <path
        d={`M${x + 4} ${y}H${x + w - fold}L${x + w} ${y + fold}V${y + h - 4}a4 4 0 0 1 -4 4H${x + 4}a4 4 0 0 1 -4 -4V${y + 4}a4 4 0 0 1 4 -4Z`}
        fill="var(--bg-raised)"
        stroke="var(--accent-text)"
        strokeWidth={L.sw}
        strokeLinejoin="round"
      />
      <path
        d={`M${x + w - fold} ${y}V${y + fold - 3}a3 3 0 0 0 3 3H${x + w}`}
        stroke="var(--accent-text)"
        strokeWidth={L.sw}
        strokeLinejoin="round"
      />
      <path
        d={`${bar(0.45, 0.5)} ${bar(0.6, 0.38)} ${bar(0.75, 0.46)}`}
        stroke="var(--ink-faint)"
        strokeWidth={L.sw}
        strokeLinecap="round"
      />
      <circle data-port cx={x + w} cy={cy} r={4} fill="var(--data-1)" />
    </>
  );
}

function Station({ L, cx, cy }: { L: Layout; cx: number; cy: number }) {
  const x = cx - L.mw / 2;
  const y = monitorTop(L, cy);
  const bottom = y + L.mh;
  const b = L.badge;
  const bx = x + L.mw;
  return (
    <>
      <rect
        x={x}
        y={y}
        width={L.mw}
        height={L.mh}
        rx={4}
        fill="var(--bg-soft)"
        stroke="var(--hairline-strong)"
        strokeWidth={L.sw}
      />
      <path
        d={`M${cx} ${bottom}V${bottom + 7}M${cx - 10} ${bottom + 7}H${cx + 10}`}
        stroke="var(--ink-faint)"
        strokeWidth={L.sw}
        strokeLinecap="round"
      />
      <g data-lit>
        <rect
          x={x}
          y={y}
          width={L.mw}
          height={L.mh}
          rx={4}
          fill="var(--accent-glow-soft)"
          stroke="var(--accent-text)"
          strokeWidth={L.sw}
        />
        <path
          d={`M${x + 8} ${y + 9}l4 3.5l-4 3.5M${x + 16} ${y + 16}h${L.mw * 0.3}`}
          stroke="var(--accent-text)"
          strokeWidth={L.sw}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </g>
      <g data-check>
        <circle cx={bx} cy={bottom} r={b} fill="var(--ok)" stroke="var(--bg)" strokeWidth={2} />
        <path
          d={`M${bx - b * 0.42} ${bottom + b * 0.02}l${b * 0.3} ${b * 0.3}l${b * 0.55} ${-b * 0.6}`}
          stroke="var(--surface-dark)"
          strokeWidth={1.75}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </g>
    </>
  );
}

function Variant({ L, name, className }: { L: Layout; name: string; className: string }) {
  const list = stations(L);
  const total = list.length;
  const labelTop = ((L.file.cy + L.file.h / 2 + 10) / L.h) * 100;
  const labelLeft = (L.file.cx / L.w) * 100;
  return (
    <div data-fleet={name} className={className}>
      <div className="flex items-center justify-between font-mono text-xs text-ink-faint">
        <span>Rollout</span>
        <span className="flex items-center gap-2 text-ink-muted">
          <span className="size-1.5 rounded-full bg-ok" />
          <span className="tabular-nums">
            <span data-count>{total}</span> of {total} ready
          </span>
        </span>
      </div>
      <div className="relative mt-4">
        <svg viewBox={`0 0 ${L.w} ${L.h}`} className="block h-auto w-full" fill="none">
          {list.map((s, i) => {
            const d = wirePath(L, s.cx, s.cy);
            return (
              <Fragment key={i}>
                <path d={d} stroke="var(--bg-raised)" strokeWidth={L.sw} />
                <path data-wire d={d} stroke="var(--data-1)" strokeWidth={L.sw} strokeLinecap="round" />
              </Fragment>
            );
          })}
          <FileNode L={L} />
          {list.map((s, i) => (
            <Station key={i} L={L} cx={s.cx} cy={s.cy} />
          ))}
        </svg>
        <span
          className="absolute -translate-x-1/2 font-mono text-xs text-ink"
          style={{ left: `${labelLeft}%`, top: `${labelTop}%` }}
        >
          du.exe
        </span>
      </div>
    </div>
  );
}

/** Runs the draw-and-light wave once inside one variant's root. */
function playWave(root: Element) {
  const wires = Array.from(root.querySelectorAll<SVGPathElement>("[data-wire]"));
  const lits = Array.from(root.querySelectorAll("[data-lit]"));
  const checks = Array.from(root.querySelectorAll("[data-check]"));
  const port = root.querySelector("[data-port]");
  const count = root.querySelector("[data-count]");
  const total = wires.length;

  const lens = wires.map((w) => {
    try {
      return w.getTotalLength();
    } catch {
      return 400;
    }
  });
  const arrivals = lens.map((len) => WIRE_START + len / SPEED);

  gsap.set(wires, {
    strokeDasharray: (i: number) => lens[i],
    strokeDashoffset: (i: number) => lens[i],
  });
  gsap.set(lits, { opacity: 0 });
  gsap.set(checks, { scale: 0, transformOrigin: "50% 50%" });
  if (port) gsap.set(port, { scale: 0.4, transformOrigin: "50% 50%" });
  if (count) count.textContent = "0";

  const tl = gsap.timeline({
    scrollTrigger: { trigger: root, start: "top 72%", once: true },
  });
  if (port) tl.to(port, { scale: 1, duration: 0.35, ease: "back.out(3)" }, 0);
  wires.forEach((wire, i) => {
    tl.to(wire, { strokeDashoffset: 0, duration: lens[i] / SPEED, ease: "none" }, WIRE_START);
    tl.to(lits[i], { opacity: 1, duration: 0.3, ease: "power1.out" }, arrivals[i]);
    tl.to(checks[i], { scale: 1, duration: 0.4, ease: "back.out(2.4)" }, arrivals[i] + 0.08);
  });
  [...arrivals]
    .sort((a, b) => a - b)
    .forEach((t, k) => {
      tl.call(
        () => {
          if (count) count.textContent = String(k + 1);
        },
        undefined,
        t,
      );
    });

  return () => {
    if (count) count.textContent = String(total);
  };
}

export function FleetDiagram() {
  const ref = useRef<HTMLDivElement>(null);

  useScrollScene(ref, () => {
    const scope = ref.current;
    if (!scope) return;
    // Only the variant on screen animates; the hidden one stays in its lit resting state.
    const mm = gsap.matchMedia(scope);
    const run = (name: string) => () => {
      const root = scope.querySelector(`[data-fleet="${name}"]`);
      return root ? playWave(root) : undefined;
    };
    mm.add("(min-width: 640px)", run("wide"));
    mm.add("(max-width: 639.98px)", run("compact"));
    return () => mm.revert();
  });

  return (
    <div
      ref={ref}
      role="img"
      aria-label="Diagram: a single du.exe file fans out through connector lines to a grid of Windows workstations, and each one lights up with a check as it becomes ready. Copy the binary to each machine and run it; there is nothing else to install."
      className="p-4 sm:p-6"
    >
      <Variant L={WIDE} name="wide" className="hidden sm:block" />
      <Variant L={COMPACT} name="compact" className="sm:hidden" />
    </div>
  );
}
