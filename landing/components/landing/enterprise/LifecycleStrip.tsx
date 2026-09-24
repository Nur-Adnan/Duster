import { enterprise } from "@/lib/content";

/**
 * The fleet lifecycle as a compact numbered loop. A row of steps from `sm`
 * up with a dashed return path from the last step back to the first; a
 * vertical list on phones with the return path running up its left edge.
 */

function Chevron({ className, dir }: { className: string; dir: "right" | "up" }) {
  const d = dir === "right" ? "M3.5 2.5 7 6l-3.5 3.5" : "M2.5 7.5 6 4l3.5 3.5";
  return (
    <svg viewBox="0 0 12 12" className={`size-3 text-ink-faint ${className}`} fill="none" aria-hidden="true">
      <path d={d} stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

export function LifecycleStrip() {
  const steps = enterprise.nodes;
  const last = steps.length - 1;
  // Centre of the first and last columns in the row layout.
  const edge = `${50 / steps.length}%`;
  return (
    <div className="border-t border-hairline p-4 sm:p-6">
      <p id="enterprise-lifecycle" className="text-sm text-ink-muted">
        Fleet lifecycle
      </p>
      <div className="relative mt-4 pl-8 sm:pb-10 sm:pl-0">
        <ol aria-labelledby="enterprise-lifecycle" className="grid gap-2 sm:auto-cols-fr sm:grid-flow-col sm:gap-0">
          {steps.map((step, i) => (
            <li
              key={step}
              className="relative flex min-h-9 items-center gap-3 sm:min-h-0 sm:flex-col sm:gap-2 sm:text-center"
            >
              <span className="flex size-7 shrink-0 items-center justify-center rounded-md border border-hairline-strong bg-bg-raised font-mono text-xs tabular-nums text-accent-text">
                {i + 1}
              </span>
              <span className="text-sm text-ink">{step}</span>
              {i < last && (
                <span aria-hidden="true" className="hidden sm:block">
                  <span className="absolute left-[calc(50%+1.25rem)] right-[calc(-50%+1.5rem)] top-3.5 h-px bg-hairline-strong" />
                  <Chevron dir="right" className="absolute right-[calc(-50%+1.25rem)] top-3.5 -translate-y-1/2" />
                </span>
              )}
            </li>
          ))}
        </ol>

        {/* Return path, from the last step back to the first. */}
        <span aria-hidden="true" className="sm:hidden">
          <span className="absolute bottom-[1.125rem] left-1.5 top-[1.125rem] w-3 rounded-l-md border-y border-l border-dashed border-hairline-strong" />
          <Chevron dir="right" className="absolute left-3 top-[1.125rem] -translate-y-1/2" />
        </span>
        <span aria-hidden="true" className="hidden sm:block">
          <span
            className="absolute bottom-2 h-5 rounded-b-lg border-x border-b border-dashed border-hairline-strong"
            style={{ left: edge, right: edge }}
          />
          <span className="absolute bottom-7 -translate-x-1/2" style={{ left: edge }}>
            <Chevron dir="up" className="block" />
          </span>
        </span>
      </div>
      <p className="sr-only">
        After {steps[last]}, the cycle starts again at {steps[0]}.
      </p>
    </div>
  );
}
