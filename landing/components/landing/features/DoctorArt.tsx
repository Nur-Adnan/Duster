"use client";

import { motion, type Variants } from "framer-motion";
import { ease } from "@/lib/motion";
import { FateIcon } from "../spotlight/glyphs";
import { Glyph } from "../ui/icons";

/** The ten checks du doctor actually runs (cmd/doctor.go), shortened. */
const CHECKS: { label: string; ok: boolean }[] = [
  { label: "Admin rights", ok: false },
  { label: "Temp writable", ok: true },
  { label: "PowerShell", ok: true },
  { label: "Windows build", ok: true },
  { label: "Defender", ok: true },
  { label: "Cache access", ok: true },
  { label: "Long paths", ok: false },
  { label: "Junction loops", ok: true },
  { label: "Terminal color", ok: true },
  { label: "Install path", ok: true },
];
const PASSED = CHECKS.filter((c) => c.ok).length;
const WARNED = CHECKS.length - PASSED;
const GAP = 0.09;

const pending: Variants = {
  start: { opacity: 1 },
  end: (i: number) => ({ opacity: 0, transition: { delay: 0.15 + i * GAP, duration: 0.15 } }),
};
const result: Variants = {
  start: { opacity: 0, scale: 0.4 },
  end: (i: number) => ({
    opacity: 1,
    scale: 1,
    transition: { delay: 0.15 + i * GAP, duration: 0.3, ease: ease.out },
  }),
};
const summary: Variants = {
  start: { opacity: 0, y: 4 },
  end: { opacity: 1, y: 0, transition: { delay: 0.3 + CHECKS.length * GAP, duration: 0.35, ease: ease.out } },
};

export function DoctorArt() {
  return (
    <div
      role="img"
      aria-label={`du doctor runs ten health checks: ${PASSED} pass and ${WARNED} warn (${CHECKS.filter((c) => !c.ok)
        .map((c) => c.label.toLowerCase())
        .join(" and ")}).`}
      className="flex h-full flex-col justify-center"
    >
      <ul className="grid grid-flow-col grid-cols-2 grid-rows-5 gap-x-4 gap-y-2 text-xs">
        {CHECKS.map((check, i) => (
          <li key={check.label} className="flex min-w-0 items-center gap-2">
            <span className="relative size-4 shrink-0">
              <motion.span custom={i} variants={pending} className="absolute inset-0">
                <Glyph width="16" height="16" viewBox="0 0 16 16" strokeLinecap="butt" className="text-ink-faint">
                  <circle cx="8" cy="8" r="6.25" strokeDasharray="2 2.6" />
                </Glyph>
              </motion.span>
              <motion.span custom={i} variants={result} className="absolute inset-0">
                <FateIcon fate={check.ok ? "ok" : "warn"} className={`size-4 ${check.ok ? "text-ok" : "text-warn"}`} />
              </motion.span>
            </span>
            <span className={`truncate ${check.ok ? "text-ink-muted" : "text-ink"}`}>{check.label}</span>
          </li>
        ))}
      </ul>

      <motion.p
        variants={summary}
        className="mt-4 flex items-center gap-3 border-t border-hairline pt-3 font-mono text-xs"
      >
        <span className="text-ok tabular-nums">{PASSED} passed</span>
        <span className="text-warn tabular-nums">{WARNED} warnings</span>
        <span className="ml-auto text-ink-faint">0 failed</span>
      </motion.p>
    </div>
  );
}
