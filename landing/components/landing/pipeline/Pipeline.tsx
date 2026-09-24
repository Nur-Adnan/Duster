"use client";

import { useRef } from "react";
import { gsap, useScrollScene } from "@/lib/gsap";
import { CheckGlyph } from "../ui/icons";
import { ClockGlyph, StageGlyph } from "./glyphs";

/**
 * What each gate actually runs, and when. Illustration-only, shown below lg.
 * Matches the section subhead: CI, Vet and Fmt run on every change, Hash and
 * Tag on each release, Smoke and E2E before a release. Sign is pending.
 */
const STAGE_DETAIL: Record<string, string> = {
  CI: "go test, every change",
  Vet: "vet + staticcheck, every change",
  Fmt: "gofmt, every change",
  Hash: "sha256 + attestation, releases",
  Tag: "semver release",
  Smoke: "Windows runner, before release",
  E2E: "du.exe in ConPTY, before release",
};

/** Stages per row on md, where the pipeline wraps into two rows. */
const MD_ROW = 4;

/**
 * The release gates as a connected pipeline. A progress line fills as the
 * section scrolls past and each stage resolves when the line reaches it.
 * Stages listed in `pending` are not live yet (signing): they resolve to a
 * dashed "not yet" node instead of a check, and the line runs on past them
 * because the rest of the release path does not wait on them.
 * Resting markup is the finished run, so reduced motion and no-JS show the
 * true end state; the scene animates from blank.
 */
export function Pipeline({ stages, pending = [] }: { stages: readonly string[]; pending?: readonly string[] }) {
  const scope = useRef<HTMLDivElement>(null);
  const last = stages.length - 1;
  const live = stages.filter((s) => !pending.includes(s)).length;

  useScrollScene(scope, () => {
    const root = scope.current;
    if (!root) return;
    const mm = gsap.matchMedia(root);

    const build = (axis: "scaleX" | "scaleY") => {
      const tl = gsap.timeline({
        defaults: { ease: "none" },
        scrollTrigger: {
          trigger: root.querySelector("ol"),
          start: "top 85%",
          end: "bottom 40%",
          scrub: 0.6,
        },
      });
      gsap.utils.toArray<HTMLElement>(".stage", root).forEach((stage, i) => {
        // Stage i checks at 0.5 + i; the segment after it fills over the next unit,
        // landing on stage i + 1 exactly when that one checks.
        const at = 0.5 + i;
        tl.from(stage.querySelectorAll(".stage-dim"), { opacity: 0.6, duration: 0.25 }, at).from(
          stage.querySelector(".stage-check"),
          { opacity: 0, scale: 0.4, duration: 0.25, ease: "back.out(2)" },
          at,
        );
        // Pending stages have no ring: they resolve to "not yet", not to passed.
        const ring = stage.querySelector(".stage-ring");
        if (ring) tl.from(ring, { opacity: 0, scale: 0.85, duration: 0.25, ease: "power2.out" }, at);
        const fill = stage.querySelector(".stage-fill");
        if (fill) {
          tl.from(
            fill,
            { [axis]: 0, transformOrigin: axis === "scaleX" ? "0% 50%" : "50% 0%", duration: 1 },
            at,
          );
        }
      });
      // Hold the finished run for the last stretch of the scroll range.
      tl.to({}, { duration: 0.4 });
    };

    mm.add("(min-width: 768px)", () => build("scaleX"));
    mm.add("(max-width: 767.98px)", () => build("scaleY"));
    return () => mm.revert();
  });

  return (
    <div ref={scope} className="flex h-full flex-col rounded-xl border border-hairline bg-bg-soft p-6 sm:p-8">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <h3 className="text-sm font-medium text-ink">Release gates</h3>
        <p className="font-mono text-xs text-ink-muted tabular-nums">
          {live} of {stages.length} stages live
        </p>
      </div>

      <ol
        aria-label={`Release pipeline, ${stages.length} stages`}
        className="mt-6 grid grid-cols-1 md:mt-8 md:grid-cols-4 md:gap-y-10 lg:my-auto lg:grid-cols-8 lg:py-6"
      >
        {stages.map((label, i) => {
          const waiting = pending.includes(label);
          return (
            <li
              key={label}
              className="stage relative flex h-16 min-w-0 items-center gap-4 md:h-auto md:flex-col md:gap-3 md:text-center"
            >
              {i < last && (
                // Track from this node's centre to the next one; the fill scales along it.
                <span
                  aria-hidden="true"
                  className={`absolute left-[21px] top-1/2 h-full w-0.5 bg-white/10 md:left-1/2 md:top-[21px] md:h-0.5 md:w-full ${
                    (i + 1) % MD_ROW === 0 ? "md:max-lg:hidden" : ""
                  }`}
                >
                  <span className="stage-fill absolute inset-0 origin-top bg-brand-accent md:origin-left" />
                </span>
              )}

              <span
                className={`relative z-10 flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border bg-bg-raised ${
                  waiting ? "border-dashed border-warn/60 text-ink-muted" : "border-hairline-strong text-ink"
                }`}
              >
                <span className="stage-dim flex">
                  <StageGlyph label={label} />
                </span>
                {!waiting && (
                  <span
                    aria-hidden="true"
                    className="stage-ring pointer-events-none absolute -inset-[3px] rounded-[11px] border-[1.5px] border-brand-accent shadow-[0_0_0_4px_rgb(37_99_235/0.14)]"
                  />
                )}
                <span
                  aria-hidden="true"
                  className={`stage-check absolute -right-1.5 -top-1.5 flex h-[18px] w-[18px] items-center justify-center rounded-full ring-2 ring-bg-soft ${
                    waiting ? "bg-surface-dark text-warn" : "bg-brand-accent text-white"
                  }`}
                >
                  {waiting ? <ClockGlyph width="18" height="18" /> : <CheckGlyph className="h-3 w-3 [stroke-width:2.33]" />}
                </span>
              </span>

              <span className="stage-dim min-w-0">
                <span className="block font-mono text-[13px] text-ink">{label}</span>
                {waiting ? (
                  <span aria-hidden="true" className="mt-0.5 block text-pretty font-mono text-xs text-warn">
                    <span className="lg:hidden">configuring, not yet live</span>
                    <span className="hidden lg:inline">not yet</span>
                  </span>
                ) : (
                  STAGE_DETAIL[label] && (
                    <span className="mt-0.5 block text-pretty font-mono text-xs text-ink-muted lg:hidden">
                      {STAGE_DETAIL[label]}
                    </span>
                  )
                )}
                <span className="sr-only">{waiting ? ", configuring, not yet live" : ", passed"}</span>
              </span>
            </li>
          );
        })}
      </ol>

      <div
        aria-hidden="true"
        className="mt-6 flex flex-wrap items-center gap-x-6 gap-y-2 border-t border-hairline pt-4 font-mono text-xs text-ink-muted md:mt-8"
      >
        <span className="inline-flex items-center gap-2">
          <span className="flex h-4 w-4 items-center justify-center rounded-full bg-brand-accent text-white">
            <CheckGlyph className="h-2.5 w-2.5 [stroke-width:2.33]" />
          </span>
          passed
        </span>
        <span className="inline-flex items-center gap-2">
          <span className="flex h-4 w-4 items-center justify-center text-warn">
            <ClockGlyph width="16" height="16" />
          </span>
          configuring
        </span>
        <span className="inline-flex items-center gap-2">
          <span className="h-0.5 w-5 bg-brand-accent" />
          run progress
        </span>
      </div>
    </div>
  );
}
