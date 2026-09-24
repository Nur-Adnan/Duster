import type { CSSProperties, ReactNode } from "react";
import styles from "./rise.module.css";

const delayStyle = (delay: number) => ({ "--rise-delay": `${delay}s` }) as CSSProperties;

/**
 * One step of the hero's page-load orchestration. Delays are set by the
 * caller so the whole hero plays as one sequence, once, on load. Pure CSS:
 * it runs from the first paint, with no JavaScript.
 */
export function Rise({
  delay,
  className = "",
  children,
}: {
  delay: number;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={`${styles.rise} ${className}`} style={delayStyle(delay)}>
      {children}
    </div>
  );
}

/**
 * A headline line that rises out of its own line box. Phrasing content only,
 * so it can live inside the h1. Translate and clip only, never opacity.
 */
export function RiseLine({ delay, children }: { delay: number; children: ReactNode }) {
  return (
    <span className={styles.lineClip}>
      <span className={styles.line} style={delayStyle(delay)}>
        {children}
      </span>
    </span>
  );
}
