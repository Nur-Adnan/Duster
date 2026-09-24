import type { ReactNode } from "react";

/**
 * Windows Terminal-style chrome shared by every terminal illustration.
 * Duster is Windows-only, so the controls sit on the right, not macOS dots.
 */
export function TerminalWindow({
  title,
  children,
  className = "",
  bodyClassName = "",
}: {
  title: string;
  children: ReactNode;
  className?: string;
  bodyClassName?: string;
}) {
  return (
    <div
      className={`overflow-hidden rounded-xl border border-hairline-strong bg-surface-dark shadow-[0_40px_120px_-40px_rgb(0_0_0/0.8),0_0_0_1px_rgb(255_255_255/0.02)_inset] ${className}`}
    >
      <div className="flex h-10 items-center gap-3 border-b border-hairline bg-white/[0.025] pl-4">
        <span className="flex h-6 min-w-0 items-center gap-2 rounded-t-md border-x border-t border-hairline bg-bg-soft px-3 font-mono text-xs whitespace-nowrap text-ink-muted">
          <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
            <path d="M1 2l3 3-3 3M5 8h4" fill="none" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
          </svg>
          <span className="truncate">{title}</span>
        </span>
        <span className="ml-auto flex h-full shrink-0 items-center text-ink-faint" aria-hidden="true">
          <span className="flex h-full w-10 items-center justify-center"><span className="h-px w-2.5 bg-current" /></span>
          <span className="flex h-full w-10 items-center justify-center"><span className="h-2.5 w-2.5 border border-current" /></span>
          <span className="flex h-full w-10 items-center justify-center">
            <svg width="10" height="10" viewBox="0 0 10 10"><path d="M1 1l8 8M9 1 1 9" stroke="currentColor" strokeWidth="1.1" /></svg>
          </span>
        </span>
      </div>
      <div className={bodyClassName}>{children}</div>
    </div>
  );
}
