/**
 * Illustration-only data for the safety pipeline. Page copy lives in
 * lib/content.ts; these are the diagram's labels, example paths and fates.
 */

export type GateId = "path" | "link" | "cloud" | "protected" | "dryrun" | "log" | "remove";
export type Fate = "ok" | "warn" | "danger";

export const PIPELINE_CAPTION =
  "Three paths enter the same safety layer. Only one of them comes out the other side.";

export const GATES: { id: GateId; label: string; note: string }[] = [
  { id: "path", label: "Path validated", note: "Every target is normalized and checked first" },
  { id: "link", label: "Symlink never followed", note: "Links and junctions are unlinked, never walked" },
  { id: "cloud", label: "OneDrive placeholder skipped", note: "Cloud-only files are never pulled down" },
  { id: "protected", label: "Protected folder refused", note: "System folders and Windows.old stay put" },
  { id: "dryrun", label: "Dry-run preview", note: "Nothing moves without --yes or a confirm" },
  { id: "log", label: "Operation logged", note: "Each action lands in operations.log" },
  { id: "remove", label: "Recycled or deleted", note: "Recycle Bin where the command offers it" },
];

export type Token = {
  path: string;
  /** Short name shown on the chip that travels the track. */
  short: string;
  /** Index into GATES where the token stops, or null when it passes all of them. */
  stopGate: number | null;
  fate: Fate;
  fateLabel: string;
};

export const TOKENS: Token[] = [
  {
    path: "%LOCALAPPDATA%\\Temp\\*.tmp",
    short: "*.tmp",
    stopGate: null,
    fate: "ok",
    fateLabel: "Removed, logged",
  },
  {
    path: "C:\\Users\\you\\Projects\\link → D:\\data",
    short: "link",
    stopGate: 1,
    fate: "warn",
    fateLabel: "Skipped: link not followed",
  },
  {
    path: "C:\\Windows\\System32\\drivers",
    short: "drivers",
    stopGate: 3,
    fate: "danger",
    fateLabel: "Refused: protected",
  },
];

/** Share of the desktop track the gates occupy; the rest is the exit zone. */
export const GATE_SPAN = 0.9;

/** Horizontal center of gate `g` as a fraction of the desktop track. */
export const gateCenter = (g: number) => (GATE_SPAN * (g + 0.5)) / GATES.length;

/** Where a token's chip comes to rest, as a fraction of the desktop track. */
export const stopFraction = (t: Token) => (t.stopGate === null ? 1 : gateCenter(t.stopGate));

/** Literal class strings per fate, so Tailwind can see them. */
export const FATE_STYLES: Record<Fate, { text: string; ring: string; card: string }> = {
  ok: { text: "text-ok", ring: "border-ok/70 bg-ok/15 text-ok", card: "border-ok/25 bg-ok/[0.05]" },
  warn: { text: "text-warn", ring: "border-warn/70 bg-warn/15 text-warn", card: "border-warn/25 bg-warn/[0.05]" },
  danger: {
    text: "text-danger",
    ring: "border-danger/70 bg-danger/15 text-danger",
    card: "border-danger/25 bg-danger/[0.05]",
  },
};
