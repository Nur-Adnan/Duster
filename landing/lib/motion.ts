import type { Transition, Variants } from "framer-motion";

/**
 * Motion tokens for the whole landing page. Framer Motion owns UI and
 * user-triggered motion; GSAP (lib/gsap.ts) owns scroll-linked timelines.
 * Reduced motion is handled app-wide by <MotionConfig reducedMotion="user">.
 */
export const ease = {
  out: [0.22, 1, 0.36, 1] as const,
  inOut: [0.65, 0, 0.35, 1] as const,
};

export const duration = { fast: 0.18, slow: 0.6 };

export const spring: Transition = { type: "spring", stiffness: 380, damping: 30, mass: 0.8 };

export const fadeUp: Variants = {
  hidden: { opacity: 0, y: 20 },
  visible: { opacity: 1, y: 0, transition: { duration: duration.slow, ease: ease.out } },
};

/** Parent for staggered children that use `fadeUp` (or any hidden/visible pair). */
export const stagger = (gap = 0.06, delay = 0): Variants => ({
  hidden: {},
  visible: { transition: { staggerChildren: gap, delayChildren: delay } },
});

