import type { SVGProps } from "react";

/** Shell for every stroke glyph on the page: 1.5px stroke, currentColor, decorative. */
export function Glyph({ children, ...props }: SVGProps<SVGSVGElement>) {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      {children}
    </svg>
  );
}

const NUDGE = "shrink-0 transition-transform duration-200";

/** Link arrow; nudges right when its `group` parent is hovered or focused. */
export function ArrowIcon({ className = "" }: { className?: string }) {
  return (
    <Glyph
      width="14"
      height="14"
      viewBox="0 0 16 16"
      className={`${NUDGE} group-hover:translate-x-0.5 group-focus-visible:translate-x-0.5 ${className}`}
    >
      <path d="M3 8h9M8.5 4.5 12 8l-3.5 3.5" />
    </Glyph>
  );
}

/** External-link arrow; nudges up-right on `group` hover or focus. */
export function ExternalArrow({ className = "" }: { className?: string }) {
  return (
    <Glyph
      width="12"
      height="12"
      viewBox="0 0 12 12"
      className={`${NUDGE} group-hover:translate-x-0.5 group-hover:-translate-y-0.5 group-focus-visible:translate-x-0.5 group-focus-visible:-translate-y-0.5 ${className}`}
    >
      <path d="M3.5 8.5 8.5 3.5M4.5 3.5h4v4" />
    </Glyph>
  );
}

export const CHECK_PATH = "M3.75 8.25 6.5 11 12.25 5";

/** The one check mark (16px grid); size it with className. */
export function CheckGlyph({ className = "" }: { className?: string }) {
  return (
    <Glyph width="16" height="16" viewBox="0 0 16 16" className={`shrink-0 ${className}`}>
      <path d={CHECK_PATH} />
    </Glyph>
  );
}

/** GitHub mark (filled, 16px grid); size it with className. */
export function GitHubGlyph({ className = "" }: { className?: string }) {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true" focusable="false" className={className}>
      <path d="M8 1a7 7 0 0 0-2.2 13.65c.35.07.48-.15.48-.34v-1.2c-1.96.43-2.38-.95-2.38-.95-.32-.82-.78-1.04-.78-1.04-.64-.44.05-.43.05-.43.7.05 1.08.73 1.08.73.63 1.07 1.65.76 2.05.58.06-.46.25-.76.45-.94-1.57-.18-3.22-.79-3.22-3.5 0-.77.28-1.4.73-1.9-.07-.18-.32-.9.07-1.87 0 0 .59-.19 1.94.72a6.8 6.8 0 0 1 3.54 0c1.35-.92 1.94-.72 1.94-.72.4.97.14 1.69.07 1.87.45.5.73 1.13.73 1.9 0 2.72-1.66 3.32-3.24 3.5.26.22.48.65.48 1.31v1.94c0 .19.13.4.49.34A7 7 0 0 0 8 1Z" />
    </svg>
  );
}
