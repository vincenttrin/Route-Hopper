import { useEffect, useRef, useState } from 'react';

/** The least time, in ms, between two hops appearing, however fast the backend finds them. */
export const PACE_MS = 1200;

/** How long a hop's jump takes, in ms. Shorter than PACE_MS so the bunny lands and rests before the next one. */
export const HOP_MS = 800;

/** How long to wait before the next hop may be shown: the rest of the pace interval since the last one. */
export function revealDelay(now, lastAt, interval = PACE_MS) {
  return Math.max(0, lastAt + interval - now);
}

/**
 * Reveals hops one at a time. `total` is how many hops have been found so far; the returned count
 * climbs towards it, one hop per `interval` at most, and never runs ahead of it. The first hop of
 * a run shows at once. `setShown` jumps the count (to 0 for a new trace, to `total` to show
 * everything, to the count on screen to stop revealing).
 */
export function usePacedReveal(total, initial = 0, interval = PACE_MS) {
  const [shown, setShown] = useState(initial);
  const lastAt = useRef(-Infinity);
  useEffect(() => {
    if (shown >= total) return undefined;
    const id = setTimeout(() => {
      lastAt.current = Date.now();
      setShown((n) => Math.min(n + 1, total));
    }, revealDelay(Date.now(), lastAt.current, interval));
    return () => clearTimeout(id);
  }, [shown, total, interval]);
  return [Math.min(shown, total), setShown];
}
