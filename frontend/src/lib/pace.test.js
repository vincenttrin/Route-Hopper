import { describe, expect, it } from 'vitest';
import { HOP_MS, PACE_MS, revealDelay } from './pace.js';

describe('revealDelay', () => {
  it('waits out the rest of the interval since the last hop', () => {
    expect(revealDelay(10_300, 10_000, 1200)).toBe(900);
  });

  it('does not wait once the interval has passed', () => {
    expect(revealDelay(11_200, 10_000, 1200)).toBe(0);
    expect(revealDelay(20_000, 10_000, 1200)).toBe(0);
  });

  it('shows the first hop at once', () => {
    expect(revealDelay(5, -Infinity, 1200)).toBe(0);
  });

  it('defaults to the app pace', () => {
    expect(revealDelay(0, 0)).toBe(PACE_MS);
  });
});

describe('pace', () => {
  it('is slow enough to watch, and the hop animation fits inside it', () => {
    expect(PACE_MS).toBeGreaterThanOrEqual(1000);
    expect(HOP_MS).toBeLessThan(PACE_MS);
  });
});
