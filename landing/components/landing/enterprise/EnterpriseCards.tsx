import type { ComponentType, ReactNode } from "react";
import { enterprise } from "@/lib/content";

/*
 * Glyphs are drawn on a 48 unit grid and rendered at 20px inside the
 * standard 44px icon box. `non-scaling-stroke` keeps every stroke at 1.5px
 * on screen whatever the scale. The cards are not interactive, so nothing
 * here moves: the fleet wave is the section's one moment.
 */

const G = {
  fill: "none",
  strokeWidth: 1.5,
  strokeLinecap: "round",
  strokeLinejoin: "round",
  vectorEffect: "non-scaling-stroke",
} as const;

function Glyph({ children }: { children: ReactNode }) {
  return (
    <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-hairline bg-bg-raised">
      <svg viewBox="0 0 48 48" width="20" height="20" className="overflow-visible" aria-hidden="true" focusable="false">
        {children}
      </svg>
    </span>
  );
}

/** A single file and dashed "dependency" ghosts that are not linked to it. */
function NoRuntimeGlyph() {
  return (
    <Glyph>
      <circle cx="5" cy="14" r="3.5" stroke="var(--ink-faint)" strokeDasharray="1.5 1.5" {...G} />
      <circle cx="5" cy="34" r="3.5" stroke="var(--ink-faint)" strokeDasharray="1.5 1.5" {...G} />
      <circle cx="43" cy="24" r="3.5" stroke="var(--ink-faint)" strokeDasharray="1.5 1.5" {...G} />
      <path
        d="M17 7h10l8 8v24a2 2 0 0 1-2 2H17a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2Z"
        stroke="var(--data-2)"
        {...G}
        fill="var(--bg-raised)"
      />
      <path d="M27 7v6a2 2 0 0 0 2 2h6" stroke="var(--data-2)" {...G} />
      <path d="M20 25h9M20 32h6" stroke="var(--ink-faint)" {...G} />
    </Glyph>
  );
}

const ROSETTE = (() => {
  const pts: string[] = [];
  for (let i = 0; i < 24; i++) {
    const a = (i / 24) * Math.PI * 2 - Math.PI / 2;
    const r = i % 2 === 0 ? 15 : 12.8;
    pts.push(`${(24 + r * Math.cos(a)).toFixed(2)} ${(19 + r * Math.sin(a)).toFixed(2)}`);
  }
  return `M${pts.join("L")}Z`;
})();

/** A seal with ribbon tails and a check. */
function SignedGlyph() {
  return (
    <Glyph>
      <path d="M17 28l-3 17 5-3 3.5 3.5L25 31Z" stroke="var(--accent-text)" {...G} fill="var(--bg-raised)" />
      <path d="M31 28l3 17-5-3-3.5 3.5L23 31Z" stroke="var(--accent-text)" {...G} fill="var(--bg-raised)" />
      <path d={ROSETTE} stroke="var(--accent-text)" {...G} fill="var(--bg-raised)" />
      <path d="M18 19.5l4.5 4.5 8-8" stroke="var(--ok)" {...G} />
    </Glyph>
  );
}

/** Log lines, each led by a timestamp, with a clock on the corner. */
function AuditGlyph() {
  return (
    <Glyph>
      <rect x="3" y="12" width="35" height="32" rx="3" stroke="var(--ink-faint)" {...G} fill="var(--bg-raised)" />
      <path d="M9 22h6M9 30h6M9 38h6" stroke="var(--data-5)" {...G} />
      <path d="M20 22h9M20 30h12M20 38h12" stroke="var(--ink-muted)" {...G} />
      <circle cx="38" cy="11" r="8.5" stroke="var(--data-5)" {...G} fill="var(--bg-raised)" />
      <path d="M38 7V11l3 2" stroke="var(--data-5)" {...G} />
    </Glyph>
  );
}

/** Two arrows chasing each other around a hash. */
function SelfUpdateGlyph() {
  return (
    <Glyph>
      <path d="M10.5 19.2A14 14 0 0 1 37.5 19.2M33.6 16.6l3.9 2.6 1.2-4.5" stroke="var(--accent-text)" {...G} />
      <path d="M37.5 28.8A14 14 0 0 1 10.5 28.8M14.4 31.4l-3.9-2.6-1.2 4.5" stroke="var(--accent-text)" {...G} />
      <path d="M21.5 18.5l-1.2 11M27.7 18.5l-1.2 11M17.5 22h13M17 26h13" stroke="var(--data-4)" {...G} />
    </Glyph>
  );
}

/**
 * Glyphs keyed by card title, so reordering `enterprise.cards` in content.ts
 * can never pair a card with the wrong picture. A title with no entry falls
 * back to the file glyph.
 */
const GLYPHS: Record<string, ComponentType> = {
  "No runtime": NoRuntimeGlyph,
  "Verified releases": SignedGlyph,
  Auditable: AuditGlyph,
  "Self-updating": SelfUpdateGlyph,
};
const FALLBACK_GLYPH = NoRuntimeGlyph;

export function EnterpriseCards() {
  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:col-span-5">
      {enterprise.cards.map((card) => {
        const Icon = GLYPHS[card.title] ?? FALLBACK_GLYPH;
        return (
          <article
            key={card.title}
            className="flex flex-col rounded-xl border border-hairline bg-bg p-5 transition-colors duration-300 hover:border-hairline-strong"
          >
            <Icon />
            <h3 className="mt-5 text-[0.95rem] font-semibold text-ink">{card.title}</h3>
            <p className="mt-2 text-sm leading-relaxed text-ink-muted">{card.body}</p>
          </article>
        );
      })}
    </div>
  );
}
