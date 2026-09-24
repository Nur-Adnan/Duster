import { ImageResponse } from "next/og";
import { hero } from "@/lib/content";

export const size = { width: 1200, height: 630 };
export const contentType = "image/png";
export const alt = "Duster: your drive, swept clean. A Windows CLI that cleans caches safely.";

// Treemap-style strip echoing the hero's disk-sweep visual.
const BLOCKS = [
  { w: 310, c: "#3b82f6", label: "Browser caches", size: "3.1 GB" },
  { w: 280, c: "#38bdf8", label: "npm / pnpm", size: "2.8 GB" },
  { w: 190, c: "#818cf8", label: "Windows Update", size: "1.9 GB" },
  { w: 120, c: "#2dd4bf", label: "Shaders", size: "640 MB" },
];

/** Social card (1200x630); twitter inherits it from openGraph. */
export default function Image() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          padding: "72px 80px",
          background: "radial-gradient(ellipse 70% 60% at 85% 0%, rgba(37,99,235,0.35), transparent 70%), #0b0d12",
          color: "#e7eaf0",
          fontFamily: "sans-serif",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 16, fontSize: 30, fontWeight: 600 }}>
          <div
            style={{
              display: "flex",
              width: 44,
              height: 44,
              borderRadius: 999,
              background: "#2563eb",
              alignItems: "center",
              justifyContent: "center",
              fontSize: 22,
            }}
          >
            du
          </div>
          Duster
        </div>

        <div style={{ display: "flex", flexDirection: "column" }}>
          <div style={{ fontSize: 96, fontWeight: 700, letterSpacing: -4, lineHeight: 1 }}>{hero.headline[0]}</div>
          <div style={{ fontSize: 96, fontWeight: 700, letterSpacing: -4, lineHeight: 1.05, color: "#7fb0ff" }}>
            {hero.headline[1]}
          </div>
          <div style={{ marginTop: 28, fontSize: 30, color: "#8a93a6" }}>
            Windows CLI deep cleaner. Dry-run first, every delete logged.
          </div>
        </div>

        <div style={{ display: "flex", gap: 10 }}>
          {BLOCKS.map((b) => (
            <div
              key={b.label}
              style={{
                display: "flex",
                flexDirection: "column",
                justifyContent: "flex-end",
                width: b.w,
                height: 96,
                padding: "12px 16px",
                borderRadius: 12,
                background: `${b.c}33`,
                border: `2px solid ${b.c}`,
                fontSize: 20,
              }}
            >
              <div style={{ color: "#e7eaf0" }}>{b.label}</div>
              <div style={{ color: "#8a93a6" }}>{b.size}</div>
            </div>
          ))}
          <div
            style={{
              display: "flex",
              flex: 1,
              height: 96,
              borderRadius: 12,
              border: "2px dashed rgba(255,255,255,0.18)",
              alignItems: "center",
              justifyContent: "center",
              fontSize: 20,
              color: "#8a93a6",
            }}
          >
            Your files: locked
          </div>
        </div>
      </div>
    ),
    size,
  );
}
