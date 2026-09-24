"use client";

import { motion, type Variants } from "framer-motion";
import { ease } from "@/lib/motion";
import { Glyph } from "../ui/icons";

type Line =
  | { branch: string; name: string; kind: "root" | "dir" }
  | { branch: string; name: string; kind: "marker"; project: number }
  | { branch: string; name: string; kind: "artifact"; project: number; size: string };

/** A projects folder: each build artifact sits next to the marker file that proves it's one. */
const TREE: Line[] = [
  { branch: "", name: "C:\\code", kind: "root" },
  { branch: "├─ ", name: "web-app", kind: "dir" },
  { branch: "│  ├─ ", name: "package.json", kind: "marker", project: 0 },
  { branch: "│  └─ ", name: "node_modules", kind: "artifact", project: 0, size: "412 MB" },
  { branch: "├─ ", name: "api", kind: "dir" },
  { branch: "│  ├─ ", name: "Cargo.toml", kind: "marker", project: 1 },
  { branch: "│  └─ ", name: "target", kind: "artifact", project: 1, size: "1.1 GB" },
  { branch: "└─ ", name: "android", kind: "dir" },
  { branch: "   ├─ ", name: "build.gradle", kind: "marker", project: 2 },
  { branch: "   └─ ", name: ".gradle", kind: "artifact", project: 2, size: "380 MB" },
];

const STEP = 0.4;
const at = (project: number) => 0.15 + project * STEP;

const markerGlow: Variants = {
  start: { opacity: 0 },
  end: (p: number) => ({ opacity: 1, transition: { delay: at(p), duration: 0.25 } }),
};
const markerTag: Variants = {
  start: { opacity: 0, x: -4 },
  end: (p: number) => ({ opacity: 1, x: 0, transition: { delay: at(p), duration: 0.3, ease: ease.out } }),
};
const strike: Variants = {
  start: { scaleX: 0 },
  end: (p: number) => ({ scaleX: 1, transition: { delay: at(p) + 0.22, duration: 0.3, ease: ease.out } }),
};
const dim: Variants = {
  start: { opacity: 1 },
  end: (p: number) => ({ opacity: 0.6, transition: { delay: at(p) + 0.3, duration: 0.3 } }),
};
const bin: Variants = {
  start: { opacity: 0, scale: 0.6 },
  end: (p: number) => ({ opacity: 1, scale: 1, transition: { delay: at(p) + 0.3, duration: 0.3, ease: ease.out } }),
};

function BinIcon() {
  return (
    <Glyph width="12" height="12" viewBox="0 0 12 12" strokeWidth="1.2">
      <path d="M1.5 3h9M4.5 3V1.75h3V3M2.75 3l.6 7.25h5.3L9.25 3M5 5.25v3M7 5.25v3" />
    </Glyph>
  );
}

export function PurgeArt() {
  return (
    <div
      role="img"
      aria-label="du purge flags node_modules, target and .gradle for removal, each only because a project marker file (package.json, Cargo.toml, build.gradle) sits beside it."
      className="flex h-full flex-col justify-center font-mono text-xs leading-[17px]"
    >
      {TREE.map((line) => (
        <div key={line.branch + line.name} className="relative flex items-center justify-between gap-3">
          {line.kind === "marker" && (
            <motion.span
              custom={line.project}
              variants={markerGlow}
              className="absolute -inset-x-1.5 inset-y-0 rounded-sm bg-[rgb(127_176_255/0.08)]"
            />
          )}
          <span className="relative whitespace-pre">
            <span className="text-ink-faint">{line.branch}</span>
            {line.kind === "artifact" ? (
              <motion.span custom={line.project} variants={dim} className="relative inline-block text-ink">
                {line.name}
                <motion.span
                  custom={line.project}
                  variants={strike}
                  className="absolute inset-x-0 top-1/2 h-px origin-left bg-danger"
                />
              </motion.span>
            ) : (
              <span
                className={
                  line.kind === "marker" ? "text-accent-text" : line.kind === "root" ? "text-ink-muted" : "text-ink"
                }
              >
                {line.name}
              </span>
            )}
          </span>

          {line.kind === "marker" && (
            <motion.span custom={line.project} variants={markerTag} className="relative text-accent-text/80">
              marker
            </motion.span>
          )}
          {line.kind === "artifact" && (
            <span className="relative flex items-center gap-1.5 tabular-nums text-ink-muted">
              {line.size}
              <motion.span custom={line.project} variants={bin} className="text-danger">
                <BinIcon />
              </motion.span>
            </span>
          )}
        </div>
      ))}
    </div>
  );
}
