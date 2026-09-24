"use client";

import gsap from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";
import { type RefObject, useEffect, useRef } from "react";

if (typeof window !== "undefined") {
  gsap.registerPlugin(ScrollTrigger);
}

export { gsap, ScrollTrigger };

const MOTION_OK = "(prefers-reduced-motion: no-preference)";
export const MOTION_REDUCED = "(prefers-reduced-motion: reduce)";

type Setup = (context: gsap.Context) => void | (() => void);

/**
 * The one way to run GSAP in this app: scoped to a ref, only when the user
 * allows motion, and fully reverted (tweens, ScrollTriggers, inline styles)
 * on unmount. Selectors inside `setup` only match within `scope`.
 * Under reduced motion nothing runs: resting markup must be the final state.
 */
export function useScrollScene(
  scope: RefObject<HTMLElement | SVGElement | null>,
  setup: Setup,
) {
  // Keep the latest callback without re-running the scene every render.
  const setupRef = useRef(setup);
  useEffect(() => {
    setupRef.current = setup;
  });

  useEffect(() => {
    if (!scope.current) return;
    const mm = gsap.matchMedia(scope.current);
    mm.add(MOTION_OK, (ctx) => setupRef.current(ctx));
    return () => mm.revert();
  }, [scope]);
}
