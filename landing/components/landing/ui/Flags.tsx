import { Fragment, type ReactNode } from "react";

// CLI flags (`--dry-run`) and commands (`du update`).
const CODE = /(--[a-z][a-z-]*|\bdu [a-z]+\b)/g;

/**
 * Renders copy with CLI flags and commands as inline code that never breaks at
 * its hyphens. The text stays copy-pasteable (real hyphens). `chip` adds a
 * bordered background, for dense answer text.
 */
export function WithFlags({ children, chip = false }: { children: string; chip?: boolean }): ReactNode {
  return children.split(CODE).map((part, i) =>
    i % 2 === 1 ? (
      <code
        key={i}
        className={`whitespace-nowrap font-mono text-ink ${
          chip ? "rounded-md border border-hairline bg-white/4 px-1.5 py-px text-[max(0.875em,12px)]" : "text-[max(0.9em,12px)]"
        }`}
      >
        {part}
      </code>
    ) : (
      <Fragment key={i}>{part}</Fragment>
    ),
  );
}
