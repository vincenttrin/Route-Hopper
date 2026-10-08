import { describe, expect, it } from 'vitest';
import { normalizeTrace, summarize, distanceKm } from './trace.js';
import { SAMPLE_TRACE } from '../services/sample.js';

describe('normalizeTrace', () => {
  const { hops, networks } = normalizeTrace(SAMPLE_TRACE);

  it('marks the last hop as destination', () => {
    expect(hops.at(-1).destination).toBe(true);
  });

  it('treats a hop with no ip as no reply', () => {
    const miss = hops.find((h) => h.n === 7);
    expect(miss.network).toBeNull();
    expect(miss.located).toBe(false);
  });

  it('switches lane when the network changes', () => {
    expect(hops[0].lane).toBe(0);
    expect(hops[1].lane).toBe(1);
    expect(hops[1].change).toBe(true);
    expect(hops[2].change).toBe(false);
  });

  it('gives every network a colour', () => {
    expect(networks.length).toBeGreaterThan(1);
    expect(new Set(networks.map((n) => n.color)).size).toBe(networks.length);
  });
});

describe('summarize', () => {
  it('computes distance and destination latency', () => {
    const { hops, networks } = normalizeTrace(SAMPLE_TRACE);
    const s = summarize(hops, networks);
    expect(s.hops).toBe(10);
    expect(s.rtt).toBe(128);
    expect(s.km).toBeGreaterThan(7000);
    expect(s.km).toBeLessThan(8500);
  });

  it('measures a known distance', () => {
    const d = distanceKm({ lat: 51.51, lng: -0.13 }, { lat: 48.86, lng: 2.35 });
    expect(Math.round(d)).toBeGreaterThan(330);
    expect(Math.round(d)).toBeLessThan(350);
  });
});
