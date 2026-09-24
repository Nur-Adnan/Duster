import { WithFlags } from "../ui/Flags";

/** Illustration-only sample data: the kind of question an issue is good for. */
const ISSUE = {
  number: 142,
  title: "du purge skipped my monorepo?",
  labels: ["question", "purge"],
  reply: "Purge only sweeps ambiguous folders like node_modules or target when a project marker sits next to them. Try --dry-run to see why.",
};

function IssueOpenIcon({ className = "" }: { className?: string }) {
  return (
    <svg viewBox="0 0 16 16" fill="none" aria-hidden="true" className={className}>
      <circle cx="8" cy="8" r="6.25" stroke="currentColor" strokeWidth="1.5" />
      <circle cx="8" cy="8" r="1.5" fill="currentColor" />
    </svg>
  );
}

/** A GitHub-issue-style card: an open question that got a straight answer. */
export function IssueCard() {
  return (
    <div
      role="img"
      aria-label={`Example GitHub issue #${ISSUE.number}, "${ISSUE.title}", status open, with a maintainer reply`}
      className="rounded-lg border border-hairline bg-surface-dark p-4"
    >
      <div className="flex items-start gap-3">
        <IssueOpenIcon className="mt-0.5 size-4 shrink-0 text-ok" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium leading-snug text-ink">{ISSUE.title}</p>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-ink-faint">
            <span className="inline-flex items-center gap-1 rounded-md border border-ok/30 bg-ok/10 px-1.5 py-0.5 font-medium text-ok">
              <IssueOpenIcon className="size-3" />
              Open
            </span>
            <span className="font-mono tabular-nums">#{ISSUE.number}</span>
            {ISSUE.labels.map((label) => (
              <span
                key={label}
                className="rounded-md border border-hairline px-1.5 py-0.5 text-ink-muted"
              >
                {label}
              </span>
            ))}
          </div>
        </div>
      </div>
      <div className="ml-7 mt-3 border-l border-hairline-strong pl-3">
        <p className="text-xs leading-relaxed text-ink-muted">
          <span className="font-medium text-accent-text">Maintainer</span>{" "}
          <WithFlags>{ISSUE.reply}</WithFlags>
        </p>
      </div>
    </div>
  );
}
