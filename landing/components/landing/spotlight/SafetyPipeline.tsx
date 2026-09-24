"use client";

import { useRef } from "react";
import { gsap, useScrollScene } from "@/lib/gsap";
import { WithFlags } from "../ui/Flags";
import {
  FATE_STYLES,
  GATE_SPAN,
  GATES,
  PIPELINE_CAPTION,
  TOKENS,
  gateCenter,
  stopFraction,
  type Token,
} from "./data";
import { FateIcon, GateGlyph } from "./glyphs";

const GATE_WIDTH = `${GATE_SPAN * 100}%`;
/** Fixed nav height; the pinned panel centers in the viewport below it. */
const NAV_H = 64;
/** Track + fates column, shared by the gate header and every lane so they align. */
const COLS = "grid grid-cols-[minmax(0,1fr)_15rem] gap-x-6 xl:gap-x-10";

/** Chip rests just left of its stop ring (or flush with the track's end). */
function restLeft(t: Token) {
  return t.stopGate === null ? "100%" : `calc(${stopFraction(t) * 100}% - 22px)`;
}

/**
 * Desktop (lg+) safety pipeline. The resting markup is the final state: each
 * token parked at its stopping gate with its fate shown. With motion allowed,
 * one scrubbed, pinned GSAP timeline replays the run from the empty track.
 */
export function SafetyPipeline() {
  const scope = useRef<HTMLDivElement>(null);

  useScrollScene(scope, () => {
    const root = scope.current;
    if (!root) return;
    const mm = gsap.matchMedia(root);

    mm.add("(min-width: 1024px)", () => {
      const tl = gsap.timeline({
        defaults: { ease: "none" },
        scrollTrigger: {
          trigger: root,
          start: () => `center ${(window.innerHeight + NAV_H) / 2}px`,
          end: "+=90%",
          scrub: 1,
          pin: true,
          invalidateOnRefresh: true,
        },
      });

      // Tokens share one speed, so track position doubles as timeline time.
      GATES.forEach((_, g) => {
        tl.fromTo(`[data-gate-lit="${g}"]`, { opacity: 0 }, { opacity: 1, duration: 0.03 }, gateCenter(g));
      });

      TOKENS.forEach((token, i) => {
        const lane = `[data-lane="${i}"]`;
        const stop = stopFraction(token);
        const passed = token.stopGate ?? GATES.length;

        tl.fromTo(`${lane} [data-packet]`, { opacity: 0 }, { opacity: 1, duration: 0.03 }, 0);
        tl.fromTo(
          `${lane} [data-packet]`,
          { x: (_: number, el: HTMLElement) => -el.offsetLeft },
          { x: 0, duration: stop },
          0,
        );
        tl.fromTo(`${lane} [data-trail]`, { scaleX: 0 }, { scaleX: 1, duration: stop }, 0);

        for (let g = 0; g < passed; g++) {
          tl.fromTo(
            `${lane} [data-node-lit="${g}"]`,
            { opacity: 0, scale: 0.4 },
            { opacity: 1, scale: 1, duration: 0.03 },
            gateCenter(g),
          );
        }
        if (token.stopGate !== null) {
          tl.fromTo(
            `${lane} [data-node-stop]`,
            { opacity: 0, scale: 0.5 },
            { opacity: 1, scale: 1, duration: 0.04, ease: "back.out(2)" },
            stop,
          );
        }
        tl.fromTo(`${lane} [data-pending]`, { opacity: 1 }, { opacity: 0, duration: 0.03 }, stop);
        tl.fromTo(`${lane} [data-fate]`, { opacity: 0, x: -8 }, { opacity: 1, x: 0, duration: 0.05 }, stop + 0.01);
      });

      tl.to({}, { duration: 0.18 });
    });

    return () => mm.revert();
  });

  return (
    <div ref={scope} className="rounded-xl border border-hairline bg-bg-soft p-8">
      <div className={COLS}>
        <p className="col-start-2 row-start-1 self-start border-l-2 border-brand-accent pl-4 text-sm leading-relaxed text-ink-muted">
          {PIPELINE_CAPTION}
        </p>

        <ol
          aria-label="Safety gates, in order"
          className="col-start-1 row-start-1 grid grid-cols-7"
          style={{ width: GATE_WIDTH }}
        >
          {GATES.map((gate, g) => (
            <li key={gate.id} className="flex flex-col items-center px-1.5 text-center">
              <span className="relative grid h-12 w-12 place-items-center rounded-lg border border-hairline-strong bg-bg-raised">
                <GateGlyph id={gate.id} className="h-6 w-6 text-ink-faint" />
                <span
                  data-gate-lit={g}
                  aria-hidden="true"
                  className="absolute -inset-px grid place-items-center rounded-lg border border-brand-accent bg-brand-accent/20 text-accent-text shadow-[0_0_24px_-4px_var(--accent-glow-soft)]"
                >
                  <GateGlyph id={gate.id} className="h-6 w-6" />
                </span>
              </span>
              <span className="mt-3 text-sm font-medium leading-snug text-ink">{gate.label}</span>
              <span className="mt-1.5 hidden text-xs leading-snug text-ink-muted xl:block">
                <WithFlags>{gate.note}</WithFlags>
              </span>
            </li>
          ))}
        </ol>
      </div>

      <ul aria-label="Example paths and where each one stops" className="mt-7 divide-y divide-hairline border-t border-hairline">
        {TOKENS.map((token, i) => {
          const style = FATE_STYLES[token.fate];
          const passed = token.stopGate ?? GATES.length;
          const stop = stopFraction(token);
          return (
            <li key={token.path} data-lane={i} className={`${COLS} items-end py-4`}>
              <div className="min-w-0">
                <code className="inline-block max-w-full truncate rounded-md border border-hairline bg-surface-dark px-2.5 py-1 font-mono text-[13px] text-ink">
                  {token.path}
                </code>

                <div aria-hidden="true" className="relative mt-2 h-10 overflow-hidden">
                  <span className="absolute inset-x-0 top-[calc(50%-1px)] h-0.5 rounded-full bg-hairline-strong" />
                  <span
                    data-trail
                    className="absolute left-0 top-[calc(50%-1px)] h-0.5 origin-left rounded-full bg-brand-accent shadow-[0_0_8px_var(--accent-glow)]"
                    style={{ width: `${stop * 100}%` }}
                  />

                  <div className="absolute inset-y-0 left-0 grid grid-cols-7" style={{ width: GATE_WIDTH }}>
                    {GATES.map((gate, g) => (
                      <span key={gate.id} className="relative grid place-items-center">
                        <span className="h-3.5 w-3.5 rounded-full border-2 border-hairline-strong bg-bg-soft" />
                        {g < passed && (
                          <span
                            data-node-lit={g}
                            className="absolute h-3.5 w-3.5 rounded-full bg-accent-text shadow-[0_0_0_4px_rgb(37_99_235/0.3)]"
                          />
                        )}
                        {g === token.stopGate && (
                          <span
                            data-node-stop
                            className={`absolute grid h-8 w-8 place-items-center rounded-full border bg-bg-soft ${style.ring}`}
                          >
                            <FateIcon fate={token.fate} className="h-4 w-4" />
                          </span>
                        )}
                      </span>
                    ))}
                  </div>

                  <div data-packet className="absolute top-1/2" style={{ left: restLeft(token) }}>
                    <span
                      className={`block -translate-x-full -translate-y-1/2 whitespace-nowrap rounded-md border bg-surface-dark px-2 py-1 font-mono text-[13px] text-ink ${
                        token.stopGate === null ? "border-ok/60" : "border-hairline-strong"
                      }`}
                    >
                      {token.short}
                    </span>
                  </div>
                </div>
              </div>

              <div className="relative h-10">
                <div
                  data-pending
                  aria-hidden="true"
                  className="absolute inset-0 flex items-center gap-2.5 text-sm text-ink-faint opacity-0 xl:gap-3"
                >
                  <span className="h-8 w-8 shrink-0 rounded-full border border-dashed border-hairline-strong" />
                  In the pipeline
                </div>
                <p data-fate className={`absolute inset-0 flex items-center gap-2.5 text-sm font-medium leading-snug xl:gap-3 xl:text-[15px] ${style.text}`}>
                  <span className={`grid h-8 w-8 shrink-0 place-items-center rounded-full border ${style.ring}`}>
                    <FateIcon fate={token.fate} className="h-4 w-4" />
                  </span>
                  <span>
                    {token.fateLabel}
                    <span className="sr-only">
                      {token.stopGate === null
                        ? ", after passing all seven gates."
                        : `, stopped at the “${GATES[token.stopGate].label}” gate.`}
                    </span>
                  </span>
                </p>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
