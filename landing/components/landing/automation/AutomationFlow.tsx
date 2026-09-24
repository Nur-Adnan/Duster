"use client";

import { type ReactNode, useRef } from "react";
import { gsap, useScrollScene } from "@/lib/gsap";

/*
 * A scheduled run, end to end: Task Scheduler starts du headless, du writes
 * its JSON report to stdout, and the exit code tells the scheduler it worked.
 * The command and JSON keys are the real ones: `du optimize` is the command
 * that takes both --yes and --json, prints `total_reclaimed_bytes`,
 * `admin_elevated` and an RFC 3339 `timestamp`, and exits 1 when a task fails.
 * The values themselves are illustration data.
 */

const COMMAND = "du optimize --yes --json";

const REPORT_LINES = [
  { key: "total_reclaimed_bytes", value: "734003200" },
  { key: "admin_elevated", value: "false" },
  { key: "timestamp", value: '"2026-09-24T03:00:04Z"' },
];

function FlowIcon({ children, tone = "accent" }: { children: ReactNode; tone?: "accent" | "ok" }) {
  return (
    <span
      className={`relative z-10 flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border bg-bg-raised ${
        tone === "ok" ? "border-ok/40 text-ok" : "border-hairline text-accent-text"
      }`}
    >
      {/* 24 unit grid at 20px: 1.8 units lands on a 1.5px stroke. */}
      <svg
        width="20"
        height="20"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
        focusable="false"
      >
        {children}
      </svg>
    </span>
  );
}

/** Arrow between two nodes: drawn vertical, rotated to horizontal on lg. */
function Connector() {
  return (
    <span aria-hidden="true" className="flow-connector pointer-events-none">
      <svg
        className="absolute left-[2.375rem] top-full h-8 w-3 -translate-x-1/2 lg:left-[calc(100%+1rem)] lg:top-[2.625rem] lg:-translate-y-1/2 lg:-rotate-90"
        viewBox="0 0 12 32"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <path className="flow-line text-hairline-strong" d="M6 2V28" pathLength={1} strokeDasharray="1" />
        <path className="flow-head text-accent-text" d="M2.5 24.5 6 28.5l3.5-4" />
      </svg>
    </span>
  );
}

function Node({ icon, children, last = false }: { icon: ReactNode; children: ReactNode; last?: boolean }) {
  return (
    <li className="flow-node relative flex gap-4 rounded-lg border border-hairline bg-surface-dark p-4 lg:flex-col lg:gap-4 lg:p-5">
      {icon}
      <div className="min-w-0">{children}</div>
      {!last && <Connector />}
    </li>
  );
}

export function AutomationFlow() {
  const scope = useRef<HTMLDivElement>(null);

  useScrollScene(scope, () => {
    const root = scope.current;
    if (!root) return;
    const nodes = Array.from(root.querySelectorAll<HTMLElement>(".flow-node"));
    const connectors = Array.from(root.querySelectorAll<HTMLElement>(".flow-connector"));

    // The run travels left to right (top to bottom on small screens): each
    // node wakes up, then the arrow to the next one draws, and the exit code
    // check lands last.
    const tl = gsap.timeline({
      defaults: { ease: "power2.out" },
      scrollTrigger: { trigger: root, start: "top 75%", once: true },
    });
    nodes.forEach((node, i) => {
      tl.from(node, { opacity: 0.3, duration: 0.3 }, i === 0 ? 0 : "-=0.05");
      const connector = connectors[i];
      if (!connector) return;
      tl.from(connector.querySelectorAll(".flow-line"), { strokeDashoffset: 1, duration: 0.35 });
      tl.from(connector.querySelectorAll(".flow-head"), { opacity: 0, duration: 0.15 }, "-=0.1");
    });
    tl.from(".flow-ok", {
      opacity: 0,
      scale: 0.4,
      transformOrigin: "50% 50%",
      ease: "back.out(2.2)",
      duration: 0.4,
    });
  });

  return (
    <div ref={scope} className="rounded-xl border border-hairline bg-bg-soft p-4 sm:p-6 lg:p-8">
      <h3 className="text-sm font-medium text-ink">What a scheduled run looks like</h3>
      <ol className="mt-5 grid gap-8 lg:grid-cols-[1fr_1.2fr_1.45fr_1fr]">
        <Node
          icon={
            <FlowIcon>
              <circle cx="12" cy="12" r="8.25" />
              <path d="M12 7.5V12l3 2" />
            </FlowIcon>
          }
        >
          <p className="text-sm font-medium text-ink">Task Scheduler</p>
          <p className="mt-1 text-sm leading-relaxed text-ink-muted">
            Daily at <span className="tabular-nums">03:00</span>, signed in or not.
          </p>
        </Node>

        <Node
          icon={
            <FlowIcon>
              <rect x="3.25" y="4.75" width="17.5" height="14.5" rx="2" />
              <path d="M7 10l2.5 2L7 14M11.5 14.5h4.5" />
            </FlowIcon>
          }
        >
          <p className="inline-block max-w-full rounded-md border border-hairline bg-bg-raised px-2 py-1 font-mono text-xs text-accent-text">
            {/* Wrap only between words, never at a flag's hyphens. */}
            {COMMAND.split(" ").map((word, i) => (
              <span key={word} className="whitespace-nowrap">
                {i > 0 ? " " : null}
                {word}
              </span>
            ))}
          </p>
          <p className="mt-2 text-sm leading-relaxed text-ink-muted">Runs headless. No prompts, no window.</p>
        </Node>

        <Node
          icon={
            <FlowIcon>
              <path d="M6 3.25h8.25L18 7v13.75H6z" />
              <path d="M14 3.25V7.25h4" />
              <path d="M10.25 10.5c-.9 0-1.25.4-1.25 1.2v.5c0 .6-.3.9-.9 1.05.6.15.9.45.9 1.05v.5c0 .8.35 1.2 1.25 1.2M13.75 10.5c.9 0 1.25.4 1.25 1.2v.5c0 .6.3.9.9 1.05-.6.15-.9.45-.9 1.05v.5c0 .8-.35 1.2-1.25 1.2" />
            </FlowIcon>
          }
        >
          <p className="font-mono text-sm text-ink">report.json</p>
          <div className="mt-2 font-mono text-xs leading-relaxed text-ink-muted">
            <span className="block text-ink-faint">{"{"}</span>
            {REPORT_LINES.map((line) => (
              <span key={line.key} className="block pl-3">
                <span className="text-data-2">&quot;{line.key}&quot;</span>
                <span className="text-ink-faint">: </span>
                <span className="inline-block whitespace-nowrap tabular-nums text-ink">{line.value}</span>
              </span>
            ))}
            <span className="block text-ink-faint">{"}"}</span>
          </div>
        </Node>

        <Node
          last
          icon={
            <FlowIcon tone="ok">
              <circle cx="12" cy="12" r="8.25" />
              <path className="flow-ok" d="M8.25 12.25 10.75 14.75 15.75 9.5" />
            </FlowIcon>
          }
        >
          <p className="text-sm font-medium text-ink">
            Exit code <span className="font-mono tabular-nums text-ok">0</span>
          </p>
          <p className="mt-1 text-sm leading-relaxed text-ink-muted">
            The scheduler logs <span className="font-mono tabular-nums">0x0</span>. A failed task exits{" "}
            <span className="font-mono tabular-nums">1</span>.
          </p>
        </Node>
      </ol>
    </div>
  );
}
