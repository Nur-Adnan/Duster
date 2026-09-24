"use client";

import { type ReactNode, useRef } from "react";
import { gsap, useScrollScene } from "@/lib/gsap";
import { Container } from "../Container";

/**
 * Hero shell: static background, and the one GSAP scene (a gentle scrubbed
 * parallax as the hero leaves). GSAP moves the outer wrappers only; the
 * CSS load-in (Rise) owns the transforms on the elements inside them.
 */
export function HeroStage({ copy, visual }: { copy: ReactNode; visual: ReactNode }) {
  const ref = useRef<HTMLElement>(null);

  useScrollScene(ref, () => {
    // Stacked layouts scroll the visual into view under the copy, so the
    // parallax only reads as intended on the side-by-side layout. A nested
    // matchMedia adds and reverts it as the viewport crosses the breakpoint.
    const mm = gsap.matchMedia(ref.current ?? undefined);
    mm.add("(min-width: 1024px)", () => {
      const scrollTrigger = { trigger: ref.current, start: "top top", end: "bottom top", scrub: 0.6 };
      gsap.to("[data-hero-copy]", { y: -72, opacity: 0.35, ease: "none", scrollTrigger });
      gsap.to("[data-hero-visual]", { y: -28, ease: "none", scrollTrigger });
    });
    return () => mm.revert();
  });

  return (
    <section
      ref={ref}
      id="top"
      aria-labelledby="hero-title"
      className="relative isolate overflow-hidden pt-28 pb-20 sm:pt-36 sm:pb-28 lg:pt-40 lg:pb-32"
    >
      <HeroBackdrop />
      <Container className="grid grid-cols-1 items-center gap-14 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.08fr)] lg:gap-12 xl:gap-16">
        <div data-hero-copy>{copy}</div>
        <div data-hero-visual>{visual}</div>
      </Container>
    </section>
  );
}

/** Static: a radial accent glow and the engineering grid, masked to fade out. */
function HeroBackdrop() {
  return (
    <div aria-hidden="true" className="pointer-events-none absolute inset-0 -z-10">
      <div className="absolute inset-0 bg-grid mask-[radial-gradient(ellipse_70%_60%_at_65%_35%,#000_20%,transparent_75%)]" />
      <div
        className="absolute inset-0"
        style={{
          background:
            "radial-gradient(ellipse 55% 45% at 70% 30%, var(--accent-glow-soft), transparent 70%), radial-gradient(ellipse 60% 40% at 15% 0%, rgb(37 99 235 / 0.08), transparent 70%)",
        }}
      />
      <div className="absolute inset-x-0 bottom-0 h-32 bg-linear-to-b from-transparent to-bg" />
    </div>
  );
}
