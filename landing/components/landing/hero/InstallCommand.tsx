"use client";

import { AnimatePresence, motion } from "framer-motion";
import { useEffect, useRef, useState } from "react";
import { hero } from "@/lib/content";
import { CheckGlyph, Glyph } from "../ui/icons";

type Status = "idle" | "copied" | "manual";

/**
 * Copyable install command. The command is truncated visually but the span is
 * `select-all`, so a click or Ctrl+A copies the whole thing.
 */
export function InstallCommand() {
  const [status, setStatus] = useState<Status>("idle");
  const textRef = useRef<HTMLSpanElement>(null);
  const { label, command } = hero.install;

  useEffect(() => {
    if (status === "idle") return;
    const t = window.setTimeout(() => setStatus("idle"), 2200);
    return () => window.clearTimeout(t);
  }, [status]);

  const selectCommand = () => {
    const el = textRef.current;
    const sel = window.getSelection();
    if (!el || !sel) return;
    const range = document.createRange();
    range.selectNodeContents(el);
    sel.removeAllRanges();
    sel.addRange(range);
  };

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command);
      setStatus("copied");
    } catch {
      // No clipboard access (insecure context or denied): select it for Ctrl+C.
      selectCommand();
      setStatus("manual");
    }
  };

  const copied = status === "copied";

  return (
    <div className="w-full max-w-xl">
      <p className="text-xs font-medium text-ink-muted">{label}</p>
      <div className="mt-2 flex h-12 items-center rounded-lg border border-hairline bg-surface-dark/80 pl-3.5 transition-colors focus-within:border-hairline-strong hover:border-hairline-strong">
        <span aria-hidden="true" className="mr-2.5 shrink-0 font-mono text-[13px] text-ink-faint">
          PS&gt;
        </span>
        <code className="min-w-0 flex-1 truncate font-mono text-[13px] text-ink">
          <span ref={textRef} className="select-all">
            {command}
          </span>
        </code>
        <button
          type="button"
          onClick={copy}
          aria-label={copied ? "Install command copied" : "Copy install command"}
          className="ml-2 flex h-11 min-w-11 shrink-0 items-center justify-center gap-1.5 rounded-md px-3 text-xs font-medium text-ink-muted transition-colors hover:text-ink"
        >
          <AnimatePresence mode="wait" initial={false}>
            <motion.span
              key={status}
              className={`flex items-center gap-1.5 ${copied ? "text-ok" : ""}`}
              initial={{ opacity: 0, scale: 0.8 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.8 }}
              transition={{ duration: 0.15 }}
            >
              {copied ? (
                <CheckGlyph className="h-3.5 w-3.5" />
              ) : (
                <Glyph width="14" height="14" viewBox="0 0 14 14">
                  <rect x="4.75" y="4.75" width="7.5" height="7.5" rx="1.5" />
                  <path d="M9.25 2.5v-.25a.5.5 0 0 0-.5-.5h-6a.5.5 0 0 0-.5.5v6a.5.5 0 0 0 .5.5h.25" />
                </Glyph>
              )}
              <span className={status === "manual" ? "" : "hidden sm:inline"}>
                {copied ? "Copied" : status === "manual" ? "Ctrl+C" : "Copy"}
              </span>
            </motion.span>
          </AnimatePresence>
        </button>
      </div>
      <p role="status" aria-live="polite" className="sr-only">
        {status === "copied"
          ? "Copied install command to clipboard"
          : status === "manual"
            ? "Clipboard unavailable. The command is selected, press Ctrl+C to copy."
            : ""}
      </p>
    </div>
  );
}
