"use client";

import { motion, useInView, useReducedMotion, type Variants } from "framer-motion";
import { useEffect, useRef, useState } from "react";
import { ease } from "@/lib/motion";
import { useHydrated } from "@/lib/useHydrated";
import { TerminalWindow } from "../ui/TerminalWindow";

type Tok = [cls: string, text: string];
type Line = { kind: "prompt"; cmd: string } | { kind: "out"; toks: Tok[] };

const PUNCT = "text-ink-muted";
const KEY = "text-accent-text";
const STR = "text-data-4";
const NUM = "text-data-5";

/*
 * Illustration data shaped like `du purge --json` (runHeadlessPurge in cmd/purge.go):
 * scanned_path, total_found, total_size_bytes, artifacts[].
 */
const PLAN = {
  scanned_path: "C:\\dev",
  total_found: 3,
  total_size_bytes: 2903457792,
  artifacts: [
    { path: "C:\\dev\\web\\node_modules", name: "node_modules", type: "node_modules", framework: "Node.js", size: 1503238553, selected: true },
  ],
};

/** Colors one line of indented JSON: indent, optional key, value, optional comma. */
function jsonLine(text: string): Line {
  const [, indent, key, value, comma] = /^(\s*)(?:(".*?"): )?(.*?)(,?)$/.exec(text)!;
  const cls = value.startsWith('"') ? STR : /^(\d+|true|false)$/.test(value) ? NUM : PUNCT;
  const toks: Tok[] = [["", indent], [KEY, key], [PUNCT, key && ": "], [cls, value], [PUNCT, comma]];
  return { kind: "out", toks: toks.filter(([, t]) => t) };
}

const LINES: Line[] = [
  { kind: "prompt", cmd: "du purge --dry-run --json" },
  // The last two artifacts (target, .gradle) are collapsed to keep the window short.
  ...JSON.stringify(PLAN, null, 2).replace(/\n    }\n  ]/, "\n    },\n    { … }, { … }\n  ]").split("\n").map(jsonLine),
  { kind: "prompt", cmd: "$plan = du purge --dry-run --json | ConvertFrom-Json" },
  { kind: "prompt", cmd: "$plan.artifacts.name" },
  { kind: "out", toks: [["text-ink", "node_modules\ntarget\n.gradle"]] },
];

const list: Variants = {
  hidden: {},
  visible: { transition: { staggerChildren: 0.03 } },
};
const line: Variants = {
  hidden: { opacity: 0, x: -6, transition: { duration: 0 } },
  visible: { opacity: 1, x: 0, transition: { duration: 0.24, ease: ease.out } },
};

/**
 * Scripted column visual: a PowerShell session reading du's JSON plan.
 * The server markup is fully visible; after hydration the lines hide while
 * off-screen and type in once, in order, when the window scrolls into view.
 */
export function PurgeJsonTerminal() {
  const ref = useRef<HTMLPreElement>(null);
  const inView = useInView(ref, { once: true, amount: 0.4 });
  const reduce = useReducedMotion();
  const hydrated = useHydrated();
  const hidden = hydrated && !reduce && !inView;

  // Only a tab stop when there is something to scroll sideways to.
  const [overflows, setOverflows] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => setOverflows(el.scrollWidth > el.clientWidth + 1);
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  return (
    <TerminalWindow title="PowerShell" className="flex h-full flex-col" bodyClassName="flex-1 min-w-0">
      <pre
        ref={ref}
        tabIndex={overflows ? 0 : undefined}
        aria-label={`PowerShell session: du purge --dry-run --json prints a JSON plan, which ConvertFrom-Json turns into objects.${
          overflows ? " Scroll sideways to see full lines." : ""
        }`}
        data-lenis-prevent-horizontal
        className="h-full max-w-full overflow-x-auto px-4 py-5 font-mono text-xs leading-[1.7] text-ink sm:px-5 focus-visible:outline-offset-[-2px]"
      >
        <motion.code className="block" variants={list} initial={false} animate={hidden ? "hidden" : "visible"}>
          {LINES.map((l, i) => (
            <motion.span key={i} variants={line} className="block whitespace-pre">
              {l.kind === "prompt" ? (
                <>
                  <span className={PUNCT}>PS C:\dev&gt; </span>
                  <span className="text-ink">{l.cmd}</span>
                </>
              ) : (
                l.toks.map(([cls, text], j) => (
                  <span key={j} className={cls || undefined}>
                    {text}
                  </span>
                ))
              )}
            </motion.span>
          ))}
        </motion.code>
      </pre>
    </TerminalWindow>
  );
}
