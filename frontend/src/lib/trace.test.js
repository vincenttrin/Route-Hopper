import { describe, expect, it } from 'vitest';
import { normalizeTrace, summarize, distanceKm, routeDistanceKm } from './trace.js';
import { SAMPLE_TRACE } from '../services/sample.js';

describe('normalizeTrace', () => {
  const { hops, networks } = normalizeTrace(SAMPLE_TRACE);

  it('marks the last hop as destination', () => {
    expect(hops.at(-1).destination).toBe(true);
  });

  it('treats a hop with no ip as no reply', () => {
    const miss = hops.find((h) => h.n === 6);
    expect(miss.network).toBeNull();
    expect(miss.located).toBe(false);
  });

  it('switches lane when the network changes', () => {
    expect(hops[0].lane).toBe(0);
    expect(hops[1].lane).toBe(1);
    expect(hops[1].change).toBe(true);
    expect(hops[2].change).toBe(true);
    expect(hops[3].change).toBe(false);
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
    expect(hops.map((h) => h.n)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9]);
    expect(hops.find((h) => h.n === 6).span).toBe(1);
  });

  it('treats a trace as reached unless the API says otherwise', () => {
    expect(normalizeTrace(SAMPLE_TRACE).reached).toBe(true);
    expect(normalizeTrace({ ...SAMPLE_TRACE, reached: true }).reached).toBe(true);
  });
});

describe('the hidden start of a route', () => {
  const raw = {
    hops: [
      { hopNumber: 1, hidden: true },
      { hopNumber: 2, ip: '68.1.1.37', hostname: 'chgil-cr1.cox.net', city: 'Chicago', country: 'US', org: 'Cox', lat: 41.88, lng: -87.63, rtt: 19 },
      { hopNumber: 3, ip: '93.184.216.34', hostname: 'example.com', city: 'Amsterdam', country: 'NL', lat: 52.37, lng: 4.9, rtt: 128 },
    ],
    destination: { ip: '93.184.216.34', lat: 52.37, lng: 4.9 },
  };

  it('is a local-network hop with no address, location or latency', () => {
    const { hops, networks } = normalizeTrace(raw);
    expect(hops[0]).toMatchObject({ n: 1, hidden: true, ip: null, hostname: null, city: null, located: false, rtt: null, network: 'Local network' });
    expect(networks[0].name).toBe('Local network');
  });

  it('counts as a hop but not as a network, and adds nothing to the distance', () => {
    const { hops, networks } = normalizeTrace(raw);
    const s = summarize(hops, networks);
    expect(s.hops).toBe(3);
    expect(s.networks).toBe(2);
    expect(s.located).toBe(2);
    expect(s.exactKm).toBeCloseTo(distanceKm(raw.hops[1], raw.hops[2]), 6);
  });

  it('is not folded into a silent tail', () => {
    const silent = { hopNumber: 2 };
    const { hops, reached } = normalizeTrace({ hops: [{ hopNumber: 1, hidden: true }, silent, { ...silent, hopNumber: 3 }], destination: { ip: '1.1.1.1' }, reached: false });
    expect(reached).toBe(false);
    expect(hops.map((h) => [h.n, h.span])).toEqual([
      [1, 1],
      [2, 2],
    ]);
    expect(hops[0].hidden).toBe(true);
  });

  it('keeps each hop country for the share summary', () => {
    expect(normalizeTrace(raw).hops.map((h) => h.country)).toEqual([null, 'US', 'NL']);
  });
});

describe('summarize', () => {
  it('computes distance and destination latency', () => {
    const { hops, networks } = normalizeTrace(SAMPLE_TRACE);
    const s = summarize(hops, networks);
    expect(s.hops).toBe(9);
    expect(s.rtt).toBe(128);
    expect(s.km).toBeGreaterThan(7000);
    expect(s.km).toBeLessThan(7500);
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

describe('normalizeTrace while the trace is running', () => {
  const hop = (hopNumber, ip, extra = {}) => ({ hopNumber, ip, hostname: '', lat: 0, lng: 0, rtt: ip ? 5 : 0, ...extra });
  const destination = { ip: '93.184.216.34', lat: 0, lng: 0 };

  it('keeps trailing silent hops as separate rows, since more may follow', () => {
    const raw = { hops: [hop(1, '192.168.1.1'), hop(2, ''), hop(3, '')], destination };
    expect(normalizeTrace(raw, { partial: true }).hops.map((h) => h.span)).toEqual([1, 1, 1]);
    expect(normalizeTrace({ ...raw, reached: false }).hops.map((h) => h.span)).toEqual([1, 2]);
  });

  it('does not call the latest hop the destination', () => {
    const raw = { hops: [hop(1, '192.168.1.1'), hop(2, '96.34.20.4')], destination };
    const t = normalizeTrace(raw, { partial: true });
    expect(t.hops.some((h) => h.destination)).toBe(false);
    expect(t.reached).toBe(false);
    expect(summarize(t.hops, t.networks).rtt).toBeNull();
  });

  it('marks the hop that has the destination address as soon as it arrives', () => {
    const raw = { hops: [hop(1, '192.168.1.1'), hop(2, '93.184.216.34')], destination };
    const t = normalizeTrace(raw, { partial: true });
    expect(t.hops[1].destination).toBe(true);
    expect(t.reached).toBe(true);
    expect(summarize(t.hops, t.networks).rtt).toBe(5);
  });

  it('treats a cancelled or interrupted trace as unfinished', () => {
    const raw = { hops: [hop(1, '192.168.1.1'), hop(2, '96.34.20.4'), hop(3, ''), hop(4, '')], destination };
    const t = normalizeTrace(raw, { partial: true });
    expect(t.hops.map((h) => h.span)).toEqual([1, 1, 1, 1]);
    expect(t.hops.some((h) => h.destination)).toBe(false);
    expect(summarize(t.hops, t.networks).rtt).toBeNull();
  });

  it('gives the same result as the one-shot response once finished', () => {
    const full = { hops: [hop(1, '192.168.1.1'), hop(2, '96.34.20.4'), hop(3, '')], destination, reached: false };
    const t = normalizeTrace(full);
    expect(t.reached).toBe(false);
    expect(t.lastReply).toBe(2);
  });
});

describe('routeDistanceKm', () => {
  const geo = (hopNumber, ip, lat, lng) => ({ hopNumber, ip, hostname: '', lat, lng, rtt: 5 });
  const route = (...hops) => normalizeTrace({ hops }).hops;
  const london = [51.51, -0.13];
  const paris = [48.86, 2.35];
  const berlin = [52.52, 13.4];

  it('adds up the legs between consecutive located hops', () => {
    const hops = route(geo(1, '1.1.1.1', ...london), geo(2, '1.1.1.2', ...paris), geo(3, '1.1.1.3', ...berlin));
    const [a, b, c] = hops;
    expect(routeDistanceKm(hops)).toBeCloseTo(distanceKm(a, b) + distanceKm(b, c), 9);
  });

  it('skips hops without a location instead of breaking the route there', () => {
    const withGaps = route(
      geo(1, '192.168.1.1', 0, 0),
      geo(2, '1.1.1.1', ...london),
      { hopNumber: 3, ip: '' },
      geo(4, '1.1.1.2', 0, 0),
      geo(5, '1.1.1.3', ...paris),
    );
    const direct = route(geo(1, '1.1.1.1', ...london), geo(2, '1.1.1.3', ...paris));
    expect(routeDistanceKm(withGaps)).toBeCloseTo(routeDistanceKm(direct), 9);
    expect(routeDistanceKm(direct)).toBeGreaterThan(330);
  });

  it('is 0 with fewer than two located hops', () => {
    expect(routeDistanceKm([])).toBe(0);
    expect(routeDistanceKm(route(geo(1, '1.1.1.1', ...london)))).toBe(0);
    expect(routeDistanceKm(route(geo(1, '192.168.1.1', 0, 0), geo(2, '1.1.1.1', ...london)))).toBe(0);
  });

  it('is 0 when every located hop shares one spot', () => {
    expect(routeDistanceKm(route(geo(1, '1.1.1.1', ...london), geo(2, '1.1.1.2', ...london)))).toBe(0);
  });

  it('is what summarize reports, before rounding', () => {
    const { hops, networks } = normalizeTrace(SAMPLE_TRACE);
    const s = summarize(hops, networks);
    expect(s.exactKm).toBe(routeDistanceKm(hops));
    expect(s.km).toBe(Math.round(s.exactKm));
    expect(s.located).toBe(hops.filter((h) => h.located).length);
  });
});
