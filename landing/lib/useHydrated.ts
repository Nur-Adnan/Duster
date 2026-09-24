import { useSyncExternalStore } from "react";

const noop = () => () => {};

/**
 * false during SSR and the hydration render, true afterwards. Branch on this
 * (not on useReducedMotion() or window) so the first client render matches the server.
 */
export function useHydrated() {
  return useSyncExternalStore(noop, () => true, () => false);
}
