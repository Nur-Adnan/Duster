"use client";

import { Fragment, useRef } from "react";
import { gsap, useScrollScene } from "@/lib/gsap";
import { TerminalWindow } from "../ui/TerminalWindow";
import styles from "./help.module.css";

/**
 * The captured `du --help` output, printed verbatim. Each line is wrapped in
 * a span only so GSAP can stagger it in; the text content (lines joined with
 * "\n") is byte-for-byte the string it was given. Resting markup is fully
 * visible, so no-JS and reduced motion both see the finished output.
 *
 * Long lines scroll inside the window, never the page. Edge fades show only
 * while there is more to scroll in that direction (pure CSS, help.module.css).
 */
export function HelpOutput({ text }: { text: string }) {
  const scope = useRef<HTMLDivElement>(null);
  const lines = text.split("\n");

  useScrollScene(scope, () => {
    // Keep the whole reveal at or under ~1.2s however long the output is.
    const each = Math.min(0.03, 0.9 / lines.length);
    gsap.from(".help-line", {
      opacity: 0,
      duration: 0.25,
      ease: "none",
      stagger: each,
      scrollTrigger: { trigger: scope.current, start: "top 80%", once: true },
    });
  });

  return (
    <div ref={scope}>
      <TerminalWindow title="du --help">
        <pre
          tabIndex={0}
          aria-label="du --help output. Scroll sideways to see full lines."
          data-lenis-prevent-horizontal
          className={`${styles.viewport} max-w-full overflow-x-auto px-5 py-5 font-mono text-xs leading-relaxed text-ink/85 sm:px-6 sm:py-6`}
        >
          <code>
            {lines.map((line, i) => (
              <Fragment key={i}>
                <span className="help-line">{line}</span>
                {i < lines.length - 1 ? "\n" : null}
              </Fragment>
            ))}
          </code>
        </pre>
      </TerminalWindow>
    </div>
  );
}
