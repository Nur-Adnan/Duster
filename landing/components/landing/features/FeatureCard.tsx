"use client";

import { motion, useAnimationControls, useInView, useReducedMotion } from "framer-motion";
import { type ReactNode, useCallback, useEffect, useRef } from "react";

/**
 * Bento card for one command. Its illustration children use "start" / "end"
 * variants; the card plays start -> end once when it enters view and replays
 * on hover. The card itself is not interactive (no lift, no tabIndex): hover
 * only strengthens the border. Keyboard users get the in-view play, and
 * reduced motion rests on "end".
 */
export function FeatureCard({
  command,
  description,
  className = "",
  children,
}: {
  command: string;
  description: string;
  className?: string;
  children: ReactNode;
}) {
  const ref = useRef<HTMLElement>(null);
  const controls = useAnimationControls();
  const reduce = useReducedMotion();
  const inView = useInView(ref, { once: true, amount: 0.45 });
  const busy = useRef(false);

  const play = useCallback(async () => {
    if (reduce || busy.current) return;
    busy.current = true;
    controls.set("start");
    try {
      await controls.start("end");
    } finally {
      busy.current = false;
    }
  }, [controls, reduce]);

  useEffect(() => {
    if (reduce) controls.set("end");
  }, [controls, reduce]);

  useEffect(() => {
    if (inView) void play();
  }, [inView, play]);

  const headingId = `feature-${command}`;

  return (
    <motion.article
      ref={ref}
      aria-labelledby={headingId}
      initial="start"
      animate={controls}
      onHoverStart={() => void play()}
      className={`flex flex-col rounded-xl border border-hairline bg-bg-soft p-2 transition-colors duration-300 hover:border-hairline-strong ${className}`}
    >
      <div className="relative h-56 overflow-hidden rounded-lg border border-hairline bg-surface-dark p-4">
        {children}
      </div>
      <div className="px-3 pb-3 pt-5">
        <h3 id={headingId} className="font-mono text-[0.95rem] font-semibold text-ink">
          <span className="text-ink-faint">du </span>
          {command}
        </h3>
        <p className="mt-2 max-w-[48ch] text-sm leading-relaxed text-ink-muted">{description}</p>
      </div>
    </motion.article>
  );
}
