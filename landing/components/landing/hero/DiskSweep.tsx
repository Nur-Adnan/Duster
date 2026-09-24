"use client";

import {
  AnimatePresence,
  motion,
  useInView,
  useReducedMotion,
  type PanInfo,
} from "framer-motion";
import {
  type FocusEvent,
  type KeyboardEvent,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { hero } from "@/lib/content";
import { ease } from "@/lib/motion";
import { useHydrated } from "@/lib/useHydrated";
import { formatSize, parseSize, showcaseSize } from "../features/bytes";
import { CheckGlyph, Glyph } from "../ui/icons";
import { TerminalWindow } from "../ui/TerminalWindow";

/* Illustration-only data. Coordinates are percentages of the map, laid out
   by hand as a squarified treemap: cleanable blocks are sized roughly by GB
   (the smallest get a floor so label and size fit) inside the left 60%; the
   user's own files fill the right 40% and are never sized against them.
   Categories the showcase also lists take its sizes; the rest close the gap
   to the 12.4 GB the scan step reports. The three small caches get at least
   24% x 19.3% so two 16px lines fit at the narrowest desktop map (440px).
   Classes are spelled out in full so Tailwind can see them. */
type Block = {
  id: string;
  label: string;
  size?: string;
  x: number;
  y: number;
  w: number;
  h: number;
  /** Cleanable blocks carry a data tone (fill and border); kept blocks do not. */
  tone?: string;
};

// du clean's TUI starts with every category checked, so every cleanable
// block is selected in the Reviewing state.
const BROWSER = showcaseSize("Browser caches");
const NPM = showcaseSize("npm + pnpm store");
const WU = showcaseSize("Windows Update cache");
const TEMP = "2.9 GB";
const GPU = showcaseSize("GPU shader cache");
const RECYCLE = "650 MB";
const DUMPS = showcaseSize("Crash dumps");

const BLOCKS: Block[] = [
  { id: "browser", label: "Browser caches", size: BROWSER, x: 0, y: 0, w: 31.5, h: 42, tone: "bg-data-1/15 border-data-1/45" },
  { id: "npm", label: "npm + pnpm store", size: NPM, x: 31.5, y: 0, w: 28.5, h: 42, tone: "bg-data-2/15 border-data-2/45" },
  { id: "temp", label: "Temp files", size: TEMP, x: 0, y: 42, w: 36, h: 33, tone: "bg-data-3/15 border-data-3/45" },
  { id: "wu", label: "Windows Update", size: WU, x: 0, y: 75, w: 36, h: 25, tone: "bg-data-4/15 border-data-4/45" },
  { id: "gpu", label: "GPU shader", size: GPU, x: 36, y: 42, w: 24, h: 19.4, tone: "bg-data-6/15 border-data-6/45" },
  { id: "recycle", label: "Recycle Bin", size: RECYCLE, x: 36, y: 61.4, w: 24, h: 19.3, tone: "bg-white/[0.05] border-hairline-strong" },
  { id: "dumps", label: "Crash dumps", size: DUMPS, x: 36, y: 80.7, w: 24, h: 19.3, tone: "bg-data-5/15 border-data-5/45" },
  { id: "projects", label: "Projects", x: 60, y: 0, w: 40, h: 42 },
  { id: "documents", label: "Documents", x: 60, y: 42, w: 40, h: 30 },
  { id: "pictures", label: "Pictures", x: 60, y: 72, w: 20, h: 28 },
  { id: "onedrive", label: "OneDrive", x: 80, y: 72, w: 20, h: 28 },
];

const bytes = (...sizes: string[]) => sizes.reduce((sum, v) => sum + parseSize(v), 0);

/* Phones: fewer, larger blocks, so every one shows its full label and size.
   The small caches merge into "Other caches", the user's folders into one
   locked "Your files" block along the bottom. */
const MOBILE_BLOCKS: Block[] = [
  { id: "browser", label: "Browser caches", size: BROWSER, x: 0, y: 0, w: 52.5, h: 34, tone: "bg-data-1/15 border-data-1/45" },
  { id: "npm", label: "npm + pnpm store", size: NPM, x: 52.5, y: 0, w: 47.5, h: 34, tone: "bg-data-2/15 border-data-2/45" },
  { id: "temp", label: "Temp files", size: TEMP, x: 0, y: 34, w: 46, h: 36, tone: "bg-data-3/15 border-data-3/45" },
  { id: "wu", label: "Windows Update", size: WU, x: 46, y: 34, w: 54, h: 18, tone: "bg-data-4/15 border-data-4/45" },
  { id: "other", label: "Other caches", size: formatSize(bytes(GPU, RECYCLE, DUMPS)), x: 46, y: 52, w: 54, h: 18, tone: "bg-data-5/15 border-data-5/45" },
  { id: "files", label: "Your files", x: 0, y: 70, w: 100, h: 30 },
];

// Every cleanable block is selected, so the reclaimed total is their sum.
const RECLAIMED = formatSize(bytes(...BLOCKS.flatMap((b) => (b.size ? [b.size] : []))));
const INTERVAL_MS = 5000;
const STEPS = hero.previewStates;
const LAST = STEPS.length - 1;

const SUMMARY = [
  "Scanning the drive: cache categories resolve as the scan passes. Projects, Documents, Pictures and OneDrive are mapped and locked.",
  "Reviewing: every cache category is selected, as du clean does by default, Recycle Bin included. Projects, Documents, Pictures and OneDrive are locked and never touched.",
  `Cleaned: the selected caches are gone and ${RECLAIMED} is free. All of your files are exactly where they were.`,
];

export function DiskSweep() {
  const [active, setActive] = useState(0);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const [userPlaying, setUserPlaying] = useState<boolean | null>(null);
  // Only user-initiated step changes are announced, never the auto-advance.
  const [announcement, setAnnouncement] = useState("");
  // useReducedMotion() is null on the server but already true on the first
  // client render, which would hydrate a different Play/Pause button. Treat
  // motion as allowed until hydration is done, then use the real preference.
  const hydrated = useHydrated();
  const prefersReduced = useReducedMotion();
  const reduce = hydrated && prefersReduced;
  const rootRef = useRef<HTMLDivElement>(null);
  const tabRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const inView = useInView(rootRef, { amount: 0.35 });
  const uid = useId();

  // Reduced motion: never auto-advance unless the viewer presses play.
  const playing = userPlaying ?? !reduce;
  const advancing = playing && !hovered && !focused && inView;

  useEffect(() => {
    if (!advancing) return;
    const t = window.setTimeout(() => setActive((a) => (a + 1) % STEPS.length), INTERVAL_MS);
    return () => window.clearTimeout(t);
  }, [advancing, active]);

  const go = (i: number) => {
    const n = (i + STEPS.length) % STEPS.length;
    setActive(n);
    setAnnouncement(`Step ${n + 1} of ${STEPS.length}, ${STEPS[n].label}: ${STEPS[n].detail}`);
  };

  const onTabKey = (e: KeyboardEvent<HTMLButtonElement>, i: number) => {
    const next =
      e.key === "ArrowRight" ? i + 1 : e.key === "ArrowLeft" ? i - 1 : e.key === "Home" ? 0 : e.key === "End" ? LAST : null;
    if (next === null) return;
    e.preventDefault();
    const n = (next + STEPS.length) % STEPS.length;
    go(n);
    tabRefs.current[n]?.focus();
  };

  const onDragEnd = (_: unknown, info: PanInfo) => {
    if (info.offset.x < -50) go(active + 1);
    else if (info.offset.x > 50) go(active - 1);
  };

  // Keyboard focus pauses; a mouse click on a tab should not pin it paused.
  const onFocus = (e: FocusEvent<HTMLDivElement>) => {
    if (e.target.matches(":focus-visible")) setFocused(true);
  };

  const onBlur = (e: FocusEvent<HTMLDivElement>) => {
    if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setFocused(false);
  };

  const state = STEPS[active];
  const panelId = `${uid}-panel`;
  const tabId = (i: number) => `${uid}-tab-${i}`;

  return (
    <div
      ref={rootRef}
      onPointerEnter={(e) => e.pointerType === "mouse" && setHovered(true)}
      onPointerLeave={(e) => e.pointerType === "mouse" && setHovered(false)}
      onFocus={onFocus}
      onBlur={onBlur}
    >
      <TerminalWindow title="du clean" bodyClassName="p-4 sm:p-5">
        {/* Status line: visual only. Screen readers get the announcement below. */}
        <div className="flex min-h-6 flex-wrap items-center justify-between gap-x-4 gap-y-1 font-mono text-xs">
          <span className="flex items-center gap-2 text-ink">
            <StatusGlyph step={active} />
            {state.label}
          </span>
          <span className="text-ink-muted tabular-nums">{state.detail}</span>
        </div>
        <p role="status" className="sr-only">
          {announcement}
        </p>

        <motion.div
          id={panelId}
          role="tabpanel"
          aria-labelledby={tabId(active)}
          drag="x"
          dragConstraints={{ left: 0, right: 0 }}
          dragElastic={0.12}
          onDragEnd={onDragEnd}
          className="mt-4 cursor-grab active:cursor-grabbing"
        >
          {/* Beam and resolve-in: only on screen, while playing, with motion allowed. */}
          <DiskMap step={active} animateScan={!reduce && inView && playing} />
        </motion.div>

        <Legend />

        {/* Step controls: a tablist, 44px targets, plus a WCAG 2.2.2 pause control. */}
        <div className="mt-4 flex items-stretch gap-2 border-t border-hairline pt-3">
          <div role="tablist" aria-label="Cleaning demo steps" className="relative grid flex-1 grid-cols-3">
            {STEPS.map((s, i) => (
              <button
                key={s.label}
                ref={(el) => {
                  tabRefs.current[i] = el;
                }}
                id={tabId(i)}
                type="button"
                role="tab"
                aria-selected={active === i}
                aria-controls={panelId}
                tabIndex={active === i ? 0 : -1}
                onClick={() => go(i)}
                onKeyDown={(e) => onTabKey(e, i)}
                className={`flex min-h-11 items-center justify-center gap-2 rounded-md px-1 text-xs font-medium transition-colors sm:text-[13px] ${
                  active === i ? "text-ink" : "text-ink-faint hover:text-ink-muted"
                }`}
              >
                <span
                  aria-hidden="true"
                  className={`hidden h-5 w-5 items-center justify-center rounded-md border font-mono text-xs tabular-nums sm:flex ${
                    i <= active ? "border-brand-accent/60 text-accent-text" : "border-hairline-strong"
                  }`}
                >
                  {i + 1}
                </span>
                {s.label}
              </button>
            ))}
            {/* Progress through the sweep: scaleX only. */}
            <span aria-hidden="true" className="pointer-events-none absolute inset-x-0 -top-3 h-px bg-hairline">
              <motion.span
                className="block h-full origin-left bg-brand-accent"
                initial={false}
                animate={{ scaleX: (active + 1) / STEPS.length }}
                transition={{ duration: 0.5, ease: ease.out }}
              />
            </span>
          </div>
          <button
            type="button"
            onClick={() => setUserPlaying(!playing)}
            aria-label={playing ? "Pause demo" : "Play demo"}
            className="flex h-11 w-11 shrink-0 items-center justify-center rounded-md border border-hairline text-ink-muted transition-colors hover:border-hairline-strong hover:text-ink"
          >
            <Glyph width="14" height="14" viewBox="0 0 14 14">
              <path
                d={playing ? "M4.5 2.5v9M9.5 2.5v9" : "M4 2.6v8.8a.5.5 0 0 0 .77.42l6.6-4.4a.5.5 0 0 0 0-.84l-6.6-4.4A.5.5 0 0 0 4 2.6Z"}
              />
            </Glyph>
          </button>
        </div>
      </TerminalWindow>
    </div>
  );
}

function DiskMap({ step, animateScan }: { step: number; animateScan: boolean }) {
  const scanning = step === 0;
  const cleaned = step === LAST;

  return (
    <div
      role="img"
      aria-label={`Disk map. ${SUMMARY[step]}`}
      className="@container relative aspect-square w-full overflow-hidden rounded-lg border border-hairline bg-bg sm:aspect-16/10"
    >
      <div className="absolute inset-0 sm:hidden">
        {MOBILE_BLOCKS.map((b, i) => (
          <MapBlock key={b.id} block={b} step={step} index={i} scan={animateScan} />
        ))}
      </div>
      <div className="absolute inset-0 max-sm:hidden">
        {BLOCKS.map((b, i) => (
          <MapBlock key={b.id} block={b} step={step} index={i} scan={animateScan} />
        ))}
      </div>

      {/* Scan beam: a transform string, so it runs on the compositor. Rendered
          only while scanning, playing and on screen, never under reduced motion. */}
      {scanning && animateScan && (
        <motion.span
          aria-hidden="true"
          className="pointer-events-none absolute inset-y-0 left-0 w-1/5"
          style={{
            background:
              "linear-gradient(90deg, transparent, rgb(127 176 255 / 0.10) 70%, rgb(127 176 255 / 0.55) 98%, transparent)",
          }}
          animate={{ transform: ["translateX(-100%)", "translateX(500%)"] }}
          transition={{ duration: 2.4, ease: "linear", repeat: Infinity, repeatDelay: 0.4 }}
        />
      )}

      {/* Reclaimed readout over the cleanable region. */}
      <AnimatePresence>
        {cleaned && (
          <motion.div
            aria-hidden="true"
            className="absolute top-0 left-0 flex h-[70%] w-full items-center justify-center p-3 sm:h-[75%] sm:w-[60%]"
            initial={{ opacity: 0, scale: 0.94 }}
            animate={{ opacity: 1, scale: 1, transition: { delay: 0.45, duration: 0.45, ease: ease.out } }}
            exit={{ opacity: 0, transition: { duration: 0.2 } }}
          >
            <div className="rounded-lg border border-hairline-strong bg-surface-dark/90 px-4 py-3 text-center shadow-[0_20px_60px_-20px_rgb(0_0_0/0.9)]">
              <span className="flex items-center justify-center gap-1.5 text-xs text-ok">
                <CheckGlyph className="h-3 w-3" />
                Reclaimed
              </span>
              <span className="mt-1 block text-2xl font-semibold tracking-tight text-ink tabular-nums sm:text-3xl">
                {RECLAIMED}
              </span>
              <span className="mt-1 hidden text-xs text-ink-muted sm:block">Your files untouched</span>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

function MapBlock({
  block: b,
  step,
  index,
  scan,
}: {
  block: Block;
  step: number;
  index: number;
  /** The scan beam is running (motion allowed and on screen). */
  scan: boolean;
}) {
  const cleanable = Boolean(b.tone);
  const reviewing = step >= 1;
  const gone = cleanable && step === LAST;

  // With the beam running, scanning replays the resolve-in; the delay follows
  // the block's x so blocks appear as the beam passes them. Without the beam
  // (reduced motion, off screen) there is nothing to resolve against, so the
  // blocks rest fully visible. Only "Cleaned" hides the selected caches.
  const resolve =
    step === 0 && scan
      ? {
          opacity: [0, 1],
          scale: [0.92, 1],
          transition: { duration: 0.45, ease: ease.out, delay: 0.15 + (b.x / 100) * 1.6 },
        }
      : gone
        ? {
            opacity: 0,
            scale: 0.7,
            transition: { duration: 0.4, ease: ease.inOut, delay: index * 0.05 },
          }
        : { opacity: 1, scale: 1, transition: { duration: 0.35, ease: ease.out } };

  return (
    <div
      className="absolute p-[3px]"
      style={{ left: `${b.x}%`, top: `${b.y}%`, width: `${b.w}%`, height: `${b.h}%` }}
    >
      {/* Ghost of freed space, left behind when a block is cleaned. */}
      {cleanable && (
        <motion.span
          aria-hidden="true"
          className="absolute inset-[3px] rounded-md border border-dashed border-hairline-strong"
          initial={false}
          animate={{ opacity: step === LAST ? 1 : 0 }}
          transition={{ duration: 0.3, delay: step === LAST ? 0.3 : 0 }}
        />
      )}

      <motion.div
        initial={false}
        animate={resolve}
        className={`relative h-full w-full overflow-hidden rounded-md border ${
          cleanable ? b.tone : "border-hairline bg-white/[0.025] text-ink-faint"
        }`}
        style={
          cleanable
            ? undefined
            : {
                backgroundImage:
                  "repeating-linear-gradient(135deg, rgb(255 255 255 / 0.035) 0 1px, transparent 1px 7px)",
              }
        }
      >
        {/* Accent outline for selected categories. */}
        {cleanable && (
          <motion.span
            aria-hidden="true"
            className="absolute inset-0 rounded-md ring-2 ring-brand-accent ring-inset"
            initial={false}
            animate={{ opacity: reviewing ? 1 : 0 }}
            transition={{ duration: 0.3 }}
          />
        )}

        {/* The map is a container: below 34rem (phones, 1024px desktops) the
            smallest tiles cannot fit a label beside the mark, so every tile
            moves its mark to the bottom-right corner, beside the short size
            line, and the label gets the full width. From 34rem up, the mark
            sits top-right and only the label row reserves room for it. */}
        <div className="relative flex h-full flex-col justify-between px-2 py-1.5 @min-[34rem]:p-2.5">
          <span
            className={`min-w-0 text-xs leading-4 font-medium @min-[34rem]:pr-5 ${cleanable ? "text-ink" : "text-ink-muted"}`}
          >
            {b.label}
          </span>
          <div className="flex items-center justify-between gap-1.5">
            {b.size ? (
              <span className="font-mono text-xs leading-4 whitespace-nowrap text-ink tabular-nums">{b.size}</span>
            ) : (
              <span className="text-xs leading-4 whitespace-nowrap text-ink-muted">Locked</span>
            )}
            {/* The lock shows in every state: user files are never selectable. */}
            <span className="flex @min-[34rem]:absolute @min-[34rem]:top-2.5 @min-[34rem]:right-2.5">
              {cleanable ? <Checkbox visible={reviewing} /> : <LockGlyph />}
            </span>
          </div>
        </div>
      </motion.div>
    </div>
  );
}

function Checkbox({ visible }: { visible: boolean }) {
  return (
    <motion.span
      aria-hidden="true"
      className="flex h-4 w-4 shrink-0 items-center justify-center rounded-[4px] border border-brand-accent bg-brand-accent"
      initial={false}
      animate={{ opacity: visible ? 1 : 0, scale: visible ? 1 : 0.6 }}
      transition={{ duration: 0.25, ease: ease.out }}
    >
      <Glyph width="10" height="10" viewBox="0 0 10 10" className="text-white">
        <motion.path
          d="M1.8 5.2 4 7.4 8.2 2.6"
          initial={false}
          animate={{ pathLength: visible ? 1 : 0 }}
          transition={{ duration: 0.3, delay: visible ? 0.15 : 0 }}
        />
      </Glyph>
    </motion.span>
  );
}

function LockGlyph({ className = "h-4 w-4 text-ink-muted" }: { className?: string }) {
  return (
    <Glyph width="16" height="16" viewBox="0 0 16 16" className={`shrink-0 ${className}`}>
      <rect x="3.25" y="7" width="9.5" height="6.5" rx="1.5" />
      <path d="M5.5 7V5a2.5 2.5 0 0 1 5 0v2" />
    </Glyph>
  );
}

function StatusGlyph({ step }: { step: number }) {
  return (
    <span aria-hidden="true" className="flex h-4 w-4 items-center justify-center">
      {step === LAST ? (
        <CheckGlyph className="h-3.5 w-3.5 text-ok" />
      ) : (
        <Glyph width="14" height="14" viewBox="0 0 14 14" className="text-accent-text">
          {step === 0 ? (
            <>
              <circle cx="6" cy="6" r="3.75" />
              <path d="m9 9 3 3" />
            </>
          ) : (
            <>
              <rect x="1.75" y="1.75" width="10.5" height="10.5" rx="2" />
              <path d="M4.3 7.1 6.2 9 9.8 5" />
            </>
          )}
        </Glyph>
      )}
    </span>
  );
}

function Legend() {
  return (
    <div aria-hidden="true" className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-1.5 text-xs text-ink-muted">
      <span className="flex items-center gap-2">
        <span className="h-2.5 w-2.5 rounded-[3px] border border-data-1/60 bg-data-1/30" />
        Cleanable cache
      </span>
      <span className="flex items-center gap-2">
        <LockGlyph className="h-3 w-3 text-ink-faint" />
        Your files, never touched
      </span>
    </div>
  );
}
