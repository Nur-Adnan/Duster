import type { ReactNode } from "react";
import { Glyph } from "../ui/icons";

/*
 * Bespoke glyphs for the developer list, keyed by the item's text so
 * reordering the copy in content.ts cannot pair an item with the wrong glyph.
 * All decorative: the list item text next to each glyph carries the meaning.
 */
const REMOTE_SCREEN = (
  <>
    <rect x="2.75" y="3.25" width="14.5" height="10.5" rx="1.75" />
    <path d="M8 17h4M10 13.75V17" />
    <path d="M5.75 6.75l2 1.75-2 1.75M9.5 10.25h3" />
  </>
);

const DEVELOPER_GLYPHS: Record<string, ReactNode> = {
  "Structured --json output": (
    <>
      <path d="M7 3.5c-1.4 0-2 .6-2 1.9v2.3c0 1-.5 1.8-1.5 2.3 1 .5 1.5 1.3 1.5 2.3v2.3c0 1.3.6 1.9 2 1.9" />
      <path d="M13 3.5c1.4 0 2 .6 2 1.9v2.3c0 1 .5 1.8 1.5 2.3-1 .5-1.5 1.3-1.5 2.3v2.3c0 1.3-.6 1.9-2 1.9" />
      <path d="M9 10h.01M11 10h.01" strokeWidth="2" />
    </>
  ),
  "Deterministic exit codes": (
    <>
      <rect x="2.75" y="3.75" width="14.5" height="12.5" rx="3" />
      <ellipse cx="10" cy="10" rx="2.25" ry="3.1" />
      <path d="M8.6 12.3l2.8-4.6" />
    </>
  ),
  "Works headless over SSH or RDP": REMOTE_SCREEN,
};

/** The glyph for a `developers.list` item; unknown items get the terminal glyph. */
export function DeveloperGlyph({ item }: { item: string }) {
  return <Glyph>{DEVELOPER_GLYPHS[item] ?? REMOTE_SCREEN}</Glyph>;
}
