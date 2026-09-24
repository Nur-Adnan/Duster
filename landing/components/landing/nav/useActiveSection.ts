"use client";

import { useEffect, useState } from "react";

/**
 * Returns the id of the section that crosses a thin band just above the
 * middle of the viewport, or null when none of `ids` is there (the hero, or a
 * section that has no nav link). One IntersectionObserver for all sections.
 * `ids` must be stable (a module constant): it is an effect dependency.
 */
export function useActiveSection(ids: readonly string[]): string | null {
  const [active, setActive] = useState<string | null>(null);

  useEffect(() => {
    const targets = ids
      .map((id) => document.getElementById(id))
      .filter((el): el is HTMLElement => el !== null);
    if (targets.length === 0) return;

    const visible = new Set<string>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) visible.add(entry.target.id);
          else visible.delete(entry.target.id);
        }
        // React bails out when the id is unchanged, so this never re-renders per frame.
        setActive(ids.find((id) => visible.has(id)) ?? null);
      },
      { rootMargin: "-40% 0px -55% 0px", threshold: 0 },
    );

    targets.forEach((el) => observer.observe(el));
    return () => observer.disconnect();
  }, [ids]);

  return active;
}
