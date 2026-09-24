import type { ReactNode, SVGProps } from "react";
import { Glyph } from "../ui/icons";

/**
 * Bespoke 20px glyphs for the release pipeline: one grid, 1.5px stroke,
 * currentColor. All decorative: the stage label next to each carries meaning.
 */
type GlyphProps = SVGProps<SVGSVGElement>;

/** Stage glyphs keyed by the pipeline label in lib/content.ts. */
const STAGE_GLYPHS: Record<string, ReactNode> = {
  // CI: a build loop, two arrows chasing each other.
  CI: (
    <>
      <path d="M15.5 8.5A5.75 5.75 0 0 0 5 6.2" />
      <path d="M4.5 11.5A5.75 5.75 0 0 0 15 13.8" />
      <path d="M5 3.5v2.9h2.9M15 16.5v-2.9h-2.9" />
    </>
  ),
  // Vet: a magnifier reading one line of code.
  Vet: (
    <>
      <circle cx="8.5" cy="8.5" r="5" />
      <path d="M12.2 12.2 16.5 16.5M6.5 8.5h4" />
    </>
  ),
  // Fmt: indented source lines snapped to a margin.
  Fmt: (
    <>
      <path d="M3.5 3.5v13" strokeOpacity="0.5" />
      <path d="M6.5 5h9M9.5 8.5h6M9.5 12h4M6.5 15.5h7" />
    </>
  ),
  // Sign: a seal with a ribbon.
  Sign: (
    <>
      <circle cx="10" cy="8" r="4.5" />
      <path d="M8 8l1.4 1.4L12.2 6.6M7.2 11.6 6 17l4-1.8 4 1.8-1.2-5.4" />
    </>
  ),
  // Hash: a digest mark.
  Hash: <path d="M8 3.5 6.5 16.5M13.5 3.5 12 16.5M4 7.5h12.5M3.5 12.5H16" />,
  // Tag: a version label with its eyelet.
  Tag: (
    <>
      <path d="M3.5 4.5v5l7 7 6-6-7-7h-5a1 1 0 0 0-1 1Z" />
      <circle cx="7" cy="7" r="1.1" />
    </>
  ),
  // Smoke: a Windows window with a heartbeat running through it.
  Smoke: (
    <>
      <rect x="3" y="4" width="14" height="12" rx="1.5" />
      <path d="M3 7h14M5.5 12h2l1.2-2.5 1.8 4 1.2-1.5h2.8" />
    </>
  ),
  // E2E: two endpoints wired together.
  E2E: (
    <>
      <circle cx="4.75" cy="10" r="2" />
      <circle cx="15.25" cy="10" r="2" />
      <path d="M6.75 10h6.5" strokeDasharray="1.6 1.9" />
    </>
  ),
};

export function StageGlyph({ label, ...props }: GlyphProps & { label: string }) {
  return <Glyph {...props}>{STAGE_GLYPHS[label]}</Glyph>;
}

/** Not-yet-live mark: a dotted dial with its hands, drawn to sit in an 18px badge. */
export function ClockGlyph(props: GlyphProps) {
  return (
    <Glyph viewBox="0 0 18 18" {...props}>
      <circle cx="9" cy="9" r="6.25" strokeDasharray="1.5 2.1" />
      <path d="M9 5.9V9l2.1 1.4" />
    </Glyph>
  );
}

/** Actions-style workflow mark: three jobs feeding one. */
export function WorkflowGlyph(props: GlyphProps) {
  return (
    <Glyph {...props}>
      <rect x="3" y="3" width="5" height="5" rx="1" />
      <rect x="12" y="12" width="5" height="5" rx="1" />
      <path d="M5.5 8v2.5a2 2 0 0 0 2 2H12" />
    </Glyph>
  );
}

export function BranchGlyph(props: GlyphProps) {
  return (
    <Glyph {...props}>
      <circle cx="6" cy="4.5" r="1.75" />
      <circle cx="6" cy="15.5" r="1.75" />
      <circle cx="14" cy="6.5" r="1.75" />
      <path d="M6 6.25v7.5M14 8.25c0 3.5-8 2.5-8 5.5" />
    </Glyph>
  );
}
