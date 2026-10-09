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

// What traceroute's UDP probes see towards facebook.com: the path answers, then
// the destination drops the probes and every remaining hop is silent.
const SILENT_DESTINATION = {
  hops: [
    { hopNumber: 1, ip: '192.168.1.1', hostname: 'gateway.lan', lat: 0, lng: 0, rtt: 4 },
    { hopNumber: 2, ip: '96.34.20.4', hostname: 'a.charter.com', city: 'Beloit', lat: 42.5, lng: -89, rtt: 16 },
    { hopNumber: 3, ip: '157.240.69.22', hostname: 'b.tfbnw.net', city: 'Dallas', lat: 32.8, lng: -96.8, rtt: 66 },
    { hopNumber: 4, ip: '' },
    { hopNumber: 5, ip: '' },
    { hopNumber: 6, ip: '' },
  ],
  destination: { ip: '57.144.20.1', city: 'Dallas', lat: 32.78, lng: -96.8 },
  reached: false,
};

describe('a destination that never replies', () => {
  const trace = normalizeTrace(SILENT_DESTINATION);

  it('is reported as not reached and no hop is called the destination', () => {
    expect(trace.reached).toBe(false);
    expect(trace.hops.some((h) => h.destination)).toBe(false);
  });

  it('folds the silent tail into one row that spans its hops', () => {
    expect(trace.hops.map((h) => h.n)).toEqual([1, 2, 3, 4]);
    expect(trace.hops.at(-1)).toMatchObject({ n: 4, span: 3, ip: null });
    expect(summarize(trace.hops, trace.networks).hops).toBe(6);
  });

  it('keeps the destination so the map can mark where the route was heading', () => {
    expect(trace.destination).toMatchObject({ ip: '57.144.20.1', lat: 32.78, lng: -96.8, located: true });
    expect(trace.lastReply).toBe(3);
  });

  it('shows no destination latency even when the last hop to answer has an rtt', () => {
    const { hops, networks } = trace;
    expect(summarize(hops, networks).rtt).toBeNull();
    const capped = normalizeTrace({ ...SILENT_DESTINATION, hops: SILENT_DESTINATION.hops.slice(0, 3) });
    expect(summarize(capped.hops, capped.networks).rtt).toBeNull();
  });

  it('leaves a single silent hop in the middle of a route alone', () => {
    const { hops } = normalizeTrace(SAMPLE_TRACE);
    expect(hops.map((h) => h.n)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
    expect(hops.find((h) => h.n === 7).span).toBe(1);
  });

  it('treats a trace as reached unless the API says otherwise', () => {
    expect(normalizeTrace(SAMPLE_TRACE).reached).toBe(true);
    expect(normalizeTrace({ ...SAMPLE_TRACE, reached: true }).reached).toBe(true);
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

  it('reports no destination latency when the last hop has no rtt', () => {
    const raw = {
      hops: [
        { hopNumber: 1, ip: '8.8.8.8', hostname: 'a.example.com', lat: 1, lng: 1, rtt: 5 },
        { hopNumber: 2, ip: '*' },
      ],
    };
    const { hops, networks } = normalizeTrace(raw);
    expect(summarize(hops, networks).rtt).toBeNull();
  });

  it('measures a known distance', () => {
    const d = distanceKm({ lat: 51.51, lng: -0.13 }, { lat: 48.86, lng: 2.35 });
    expect(Math.round(d)).toBeGreaterThan(330);
    expect(Math.round(d)).toBeLessThan(350);
  });
});
