import { Glyph } from "../ui/icons";
import type { Fate, GateId } from "./data";

/** One 24px glyph per safety gate: 1.5px stroke, currentColor. Decorative. */
export function GateGlyph({ id, className = "" }: { id: GateId; className?: string }) {
  return (
    <Glyph viewBox="0 0 24 24" className={className}>
      {id === "path" && (
        <>
          <path d="M3 7.5A1.5 1.5 0 0 1 4.5 6H9l2 2h8.5A1.5 1.5 0 0 1 21 9.5v8a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 17.5z" />
          <path d="m9 13.5 2 2 4-4" />
        </>
      )}
      {id === "link" && (
        <>
          <path d="m8.6 12.4-1.8 1.8a2.8 2.8 0 0 0 4 4l1.8-1.8" />
          <path d="m15.4 11.6 1.8-1.8a2.8 2.8 0 0 0-4-4l-1.8 1.8" />
          <path d="M4.5 4.5 6.5 6.5M17.5 17.5l2 2M4 10h2M18 14h2" />
        </>
      )}
      {id === "cloud" && (
        <path
          d="M7 18.5h10a4 4 0 0 0 .6-7.96A5.5 5.5 0 0 0 7 9.6a4.5 4.5 0 0 0 0 8.9z"
          strokeDasharray="2.6 2.4"
        />
      )}
      {id === "protected" && (
        <>
          <path d="M12 3 19 6v5.5c0 4.2-2.9 7.9-7 9.5-4.1-1.6-7-5.3-7-9.5V6z" />
          <path d="M9.5 12h5" />
        </>
      )}
      {id === "dryrun" && (
        <>
          <path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12z" />
          <circle cx="12" cy="12" r="2.75" />
        </>
      )}
      {id === "log" && (
        <>
          <path d="M6 3.5h8l4 4v13H6z" />
          <path d="M14 3.5v4h4M9 12h6M9 15h6M9 18h3" />
        </>
      )}
      {id === "remove" && (
        <>
          <path d="M4.5 7h15M9.5 7V4.5h5V7M6.5 7l1 13h9l1-13" />
          <path d="M10 11v5M14 11v5" />
        </>
      )}
    </Glyph>
  );
}

/** Status icon that always travels with the fate text, so color is never the only signal. */
export function FateIcon({ fate, className = "" }: { fate: Fate; className?: string }) {
  return (
    <Glyph viewBox="0 0 16 16" className={className}>
      {fate === "ok" && (
        <>
          <circle cx="8" cy="8" r="6.25" />
          <path d="m5.2 8.2 1.9 1.9 3.7-3.9" />
        </>
      )}
      {fate === "warn" && (
        <>
          <path d="M8 2.2 14.2 13H1.8z" />
          <path d="M8 6.6v2.9M8 11.2v.05" />
        </>
      )}
      {fate === "danger" && (
        <>
          <circle cx="8" cy="8" r="6.25" />
          <path d="m3.6 3.6 8.8 8.8" />
        </>
      )}
    </Glyph>
  );
}
