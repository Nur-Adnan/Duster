"use client";

import { motion, useInView, useReducedMotion } from "framer-motion";
import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
} from "react";
import { spring } from "@/lib/motion";
import { useHydrated } from "@/lib/useHydrated";
import { formatSize, parseSize, showcaseSize } from "../features/bytes";
import { CHECK_PATH } from "../ui/icons";
import { TerminalWindow } from "../ui/TerminalWindow";

/* Illustration data: names follow du clean's categories; sizes match the showcase and the hero treemap (hero/DiskSweep.tsx). */
const ROWS = [
  { id: "browsers", name: "Browser caches", size: showcaseSize("Browser caches") },
  { id: "temp", name: "Temporary files", size: "2.9 GB" },
  { id: "update", name: "Windows Update cache", size: showcaseSize("Windows Update cache") },
  { id: "thumbs", name: "Thumbnail cache", size: showcaseSize("Thumbnail cache") },
  { id: "wer", name: "Windows error reports", size: "184 MB" },
  { id: "delivery_opt", name: "Delivery Optimization", size: "612 MB" },
  { id: "recycle", name: "Recycle Bin", size: "650 MB" },
];
const ROW_H = 32; // px, matches h-8 on each row
const STEPS = 3; // the demo walks down three rows, ticking each
const STEP_MS = 420;

type TuiState = { cursor: number; checked: boolean[] };

/** State after `step` demo steps: the cursor moves down one row and ticks it. */
function stateAt(step: number): TuiState {
  return { cursor: step, checked: ROWS.map((_, i) => i <= step) };
}
const END = stateAt(STEPS);

/**
 * Interactive column: a `du clean` checklist mock that plays a short
 * move-and-tick sequence once in view and replays on mouse hover. Focusing the
 * list (keyboard or click) never replays: it freezes the demo where it is and
 * hands control over, so arrows, space and enter act on what is on screen.
 */
export function CleanTuiColumn({ className = "", children }: { className?: string; children: ReactNode }) {
  const rootRef = useRef<HTMLElement>(null);
  const inView = useInView(rootRef, { once: true, amount: 0.5 });
  const reduce = useReducedMotion();
  // Server and first paint show the finished state, so no-JS and reduced motion read complete.
  const hydrated = useHydrated();

  const [step, setStep] = useState(0);
  const [run, setRun] = useState(0);
  const [user, setUser] = useState<TuiState | null>(null);
  const [notice, setNotice] = useState(false);
  const controlled = user !== null;
  const baseId = useId();
  const hintId = `${baseId}-hint`;

  useEffect(() => {
    if (!inView || reduce || controlled) return;
    const timers = Array.from({ length: STEPS }, (_, i) =>
      setTimeout(() => setStep(i + 1), 500 + i * STEP_MS),
    );
    return () => timers.forEach(clearTimeout);
  }, [inView, reduce, controlled, run]);

  const view: TuiState = user ?? (!hydrated || reduce ? END : stateAt(step));

  // Hover replay is mouse only: a tap or pen press also fires pointerenter, and
  // that is a click, not a hover.
  const replay = (e: PointerEvent<HTMLElement>) => {
    if (e.pointerType !== "mouse" || controlled || reduce || !inView || step < STEPS) return;
    setStep(0);
    setRun((r) => r + 1);
  };

  // Taking focus stops the demo on the current frame, so the active option a
  // screen reader just announced stays put and the first key acts on it.
  const takeControl = () => {
    if (!controlled) setUser(view);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const { cursor, checked } = view;
    let next: TuiState | null = null;
    switch (e.key) {
      case "ArrowDown":
        next = { cursor: Math.min(ROWS.length - 1, cursor + 1), checked };
        break;
      case "ArrowUp":
        next = { cursor: Math.max(0, cursor - 1), checked };
        break;
      case "Home":
        next = { cursor: 0, checked };
        break;
      case "End":
        next = { cursor: ROWS.length - 1, checked };
        break;
      case " ":
        next = { cursor, checked: checked.map((c, i) => (i === cursor ? !c : c)) };
        break;
      case "Enter":
        e.preventDefault();
        setUser(view);
        setNotice(true);
        return;
      default:
        return;
    }
    e.preventDefault();
    setNotice(false);
    setUser(next);
  };

  const selectedBytes = ROWS.reduce((sum, r, i) => (view.checked[i] ? sum + parseSize(r.size) : sum), 0);
  const selectedCount = view.checked.filter(Boolean).length;

  return (
    <article ref={rootRef} className={className} onPointerEnter={replay}>
      <TerminalWindow
        title="du clean"
        className="flex h-full flex-col"
        bodyClassName="flex flex-1 flex-col px-4 py-5 font-mono text-xs sm:px-5"
      >
        <p className="px-2 pb-4 text-ink-muted">
          <span className="text-ink-muted">[</span>
          <span className="text-accent-text">i</span>
          <span className="text-ink-muted">]</span> System scan complete. Ready for cleanup.
        </p>
        <div className="flex items-baseline justify-between px-2 pb-2 text-ink-muted">
          <span>Category</span>
          <span>Size</span>
        </div>

        <div
          role="listbox"
          aria-label="du clean categories, demo"
          aria-multiselectable="true"
          aria-describedby={hintId}
          aria-activedescendant={`${baseId}-row-${view.cursor}`}
          tabIndex={0}
          onFocus={takeControl}
          onKeyDown={onKeyDown}
          className="relative rounded-md focus-visible:outline-offset-[-1px]"
        >
          <motion.div
            aria-hidden="true"
            className="absolute inset-x-0 top-0 h-8 rounded-md bg-brand-accent/15 ring-1 ring-inset ring-brand-accent/45"
            initial={false}
            animate={{ y: view.cursor * ROW_H }}
            transition={spring}
          />
          {ROWS.map((row, i) => {
            const checked = view.checked[i];
            const active = i === view.cursor;
            return (
              <div
                key={row.id}
                id={`${baseId}-row-${i}`}
                role="option"
                aria-selected={checked}
                className={`relative flex h-8 items-center gap-2.5 px-2 transition-colors duration-200 ${active ? "text-ink" : "text-ink-muted"}`}
              >
                <span
                  aria-hidden="true"
                  className={`flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-[3px] border transition-colors duration-200 ${
                    checked ? "border-accent-text/70 bg-brand-accent/30 text-accent-text" : "border-hairline-strong text-transparent"
                  }`}
                >
                  <svg width="12" height="12" viewBox="0 0 16 16" fill="none">
                    <motion.path
                      d={CHECK_PATH}
                      stroke="currentColor"
                      strokeWidth="2"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      initial={false}
                      animate={{ pathLength: checked ? 1 : 0, opacity: checked ? 1 : 0 }}
                      transition={{ duration: 0.22 }}
                    />
                  </svg>
                </span>
                <span className="min-w-0 flex-1 truncate">{row.name}</span>
                <span className="shrink-0 text-right tabular-nums">{row.size}</span>
              </div>
            );
          })}
        </div>

        {/* Announce only while the user drives the list, never the automatic demo steps. */}
        <p aria-live={controlled ? "polite" : "off"} className="mt-3 flex items-baseline justify-between border-t border-hairline px-2 pt-3">
          {notice ? (
            <span className="text-ok">Demo only: nothing was deleted.</span>
          ) : (
            <>
              <span className="text-ink-muted tabular-nums">{selectedCount} selected</span>
              <span className="tabular-nums text-accent-text">{formatSize(selectedBytes)}</span>
            </>
          )}
        </p>

        <p id={hintId} className="mt-auto flex flex-wrap gap-x-4 gap-y-2 pt-5 text-xs text-ink-muted">
          <Hint keys="↑↓" label="move" sr="Up and down arrows" />
          <Hint keys="space" label="select" />
          <Hint keys="enter" label="clean" />
        </p>
      </TerminalWindow>
      {children}
    </article>
  );
}

function Hint({ keys, label, sr }: { keys: string; label: string; sr?: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <kbd className="rounded-md border border-hairline-strong bg-white/[0.04] px-1.5 py-0.5 font-mono text-xs leading-none text-ink-muted">
        {sr ? (
          <>
            <span aria-hidden="true">{keys}</span>
            <span className="sr-only">{sr}</span>
          </>
        ) : (
          keys
        )}
      </kbd>
      {label}
    </span>
  );
}
