import { showcase } from "@/lib/content";

/**
 * Decimal (SI) units: 1 GB = 1000 MB. The copy writes sizes as "3.1 GB" and
 * "640 MB", and a reader adds those up as 3.1 + 0.64, so totals use the same
 * base: the displayed parts always sum to the displayed total.
 */
export const GB = 1e9;
const MB = 1e6;

/** "3.1 GB" -> bytes. Throws on anything else so bad copy fails the build, not the page. */
export function parseSize(value: string): number {
  const match = /^(\d+(?:\.\d+)?)\s*(MB|GB)$/i.exec(value.trim());
  if (!match) throw new Error(`Unparseable size: "${value}"`);
  return Math.round(parseFloat(match[1]) * (match[2].toUpperCase() === "GB" ? GB : MB));
}

/** Bytes -> "11.3 GB" / "640 MB", one decimal from GB up. */
export function formatSize(bytes: number): string {
  return bytes >= GB ? `${(bytes / GB).toFixed(1)} GB` : `${Math.round(bytes / MB)} MB`;
}

/**
 * A category's size as the showcase tiles state it, so an illustration that
 * names the same category shows the same number. Throws on an unknown label.
 */
export function showcaseSize(label: string): string {
  const found = showcase.tiles.find((t) => t.label === label);
  if (!found) throw new Error(`No showcase tile named "${label}"`);
  return found.value;
}
