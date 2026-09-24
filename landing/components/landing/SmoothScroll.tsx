"use client";

import Lenis from "lenis";
import { useEffect } from "react";
import { gsap, MOTION_REDUCED, ScrollTrigger } from "@/lib/gsap";

/**
 * Lenis smooth scrolling, driven by GSAP's ticker so ScrollTrigger and Lenis
 * read the same frame. Skipped entirely under reduced motion (native scroll).
 */
export function SmoothScroll() {
  useEffect(() => {
    if (window.matchMedia(MOTION_REDUCED).matches) return;

    const navH = parseFloat(getComputedStyle(document.documentElement).fontSize) * 4;
    const lenis = new Lenis({ anchors: { offset: -navH }, lerp: 0.1 });

    lenis.on("scroll", ScrollTrigger.update);
    const tick = (time: number) => lenis.raf(time * 1000);
    gsap.ticker.add(tick);
    gsap.ticker.lagSmoothing(0);

    return () => {
      gsap.ticker.remove(tick);
      lenis.destroy();
    };
  }, []);

  return null;
}
