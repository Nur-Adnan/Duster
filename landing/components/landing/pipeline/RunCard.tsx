import type { integrations } from "@/lib/content";
import { CheckGlyph } from "../ui/icons";
import { BranchGlyph, ClockGlyph, WorkflowGlyph } from "./glyphs";

type Row = (typeof integrations.statusRows)[number];

/**
 * A release run summary in the shape of a CI run page: header, then one row
 * per job with an icon and a written status, so color is never the only cue.
 */
export function RunCard({ rows }: { rows: readonly Row[] }) {
  const done = rows.filter((r) => r.status === "done").length;
  const open = rows.length - done;

  return (
    <div className="rounded-xl border border-hairline bg-bg-soft transition-colors duration-200 hover:border-hairline-strong">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-hairline px-5 py-4">
        <WorkflowGlyph className="shrink-0 text-ink-muted" />
        <h3 className="text-sm font-medium text-ink">Release run</h3>
        <span className="font-mono text-xs text-ink-muted tabular-nums">#214</span>
        <span className="ml-auto inline-flex items-center gap-1.5 rounded-md border border-hairline bg-surface-dark px-2 py-1 font-mono text-xs text-accent-text">
          <BranchGlyph width="14" height="14" className="text-ink-faint" />
          <span className="sr-only">Branch </span>
          main
        </span>
      </div>

      <ul className="divide-y divide-hairline">
        {rows.map((row) => (
          <li key={row.label} className="flex items-center gap-3 px-5 py-3.5">
            {row.status === "done" ? (
              <span
                aria-hidden="true"
                className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-ok/15 text-ok"
              >
                <CheckGlyph className="h-3 w-3 [stroke-width:2.33]" />
              </span>
            ) : (
              // "Configuring" is a state, not a process: a still dotted clock, no spinner.
              <ClockGlyph width="16" height="16" className="shrink-0 text-warn" />
            )}
            <span className="min-w-0 flex-1 font-mono text-[13px] leading-snug text-ink/85">
              {row.label}
            </span>
            <span
              className={`shrink-0 font-mono text-xs ${row.status === "done" ? "text-ok" : "text-warn"}`}
            >
              {row.status}
            </span>
          </li>
        ))}
      </ul>

      <p className="border-t border-hairline px-5 py-3 font-mono text-xs text-ink-muted tabular-nums">
        {done} of {rows.length} jobs done
        {open > 0 && <span className="text-warn">, {open} configuring</span>}
      </p>
    </div>
  );
}
