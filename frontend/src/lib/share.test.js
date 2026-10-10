import { describe, expect, it, vi } from 'vitest';
import { appLink, countriesOf, endpointLabel, shareTrip, tripSummary } from './share.js';
import { normalizeTrace, summarize } from './trace.js';
import { judgeRound } from './game.js';
import { SAMPLE_TRACE } from '../services/sample.js';

const trace = normalizeTrace(SAMPLE_TRACE);
const stats = summarize(trace.hops, trace.networks);

describe('endpointLabel', () => {
  it('keeps just the host', () => {
    expect(endpointLabel('example.com')).toBe('example.com');
    expect(endpointLabel('  https://user:pw@Example.com:8443/a/b?token=secret#x ')).toBe('example.com');
    expect(endpointLabel('1.1.1.1')).toBe('1.1.1.1');
    expect(endpointLabel('[2606:4700::1111]')).toBe('2606:4700::1111');
    expect(endpointLabel('')).toBe('somewhere');
  });
});

describe('countriesOf', () => {
  it('lists each country once, in route order, by name', () => {
    const hops = [
      { located: true, country: 'US' },
      { located: true, country: 'US' },
      { located: false, country: 'FR' },
      { located: true, country: 'IE' },
      { located: true, country: null },
      { located: true, country: 'NL' },
    ];
    expect(countriesOf(hops)).toEqual(['United States', 'Ireland', 'Netherlands']);
  });
});

describe('tripSummary', () => {
  const withCountries = {
    hops: [
      { hopNumber: 1, hidden: true },
      { hopNumber: 2, ip: '68.1.1.37', hostname: 'chgil-cr1.cox.net', city: 'Chicago', country: 'US', org: 'Cox', lat: 41.88, lng: -87.63, rtt: 19 },
      { hopNumber: 3, ip: '213.200.80.1', hostname: 'ae-12.dub.gtt.net', city: 'Dublin', country: 'IE', org: 'GTT', lat: 53.35, lng: -6.26, rtt: 113 },
      { hopNumber: 4, ip: '93.184.216.34', hostname: 'example.com', city: 'Amsterdam', country: 'NL', org: 'Edgecast', lat: 52.37, lng: 4.9, rtt: 128 },
    ],
    destination: { ip: '93.184.216.34', city: 'Amsterdam', lat: 52.37, lng: 4.9 },
  };
  const t = normalizeTrace(withCountries);
  const s = summarize(t.hops, t.networks);

  it('summarizes the trip and the score', () => {
    const result = judgeRound(5000, s.exactKm, s.located);
    const text = tripSummary({ endpoint: 'https://example.com/path?x=1', trace: t, stats: s, result, unit: 'km' });
    expect(text.split('\n')).toEqual([
      '🐰 I followed a packet to example.com (Amsterdam, Netherlands)!',
      `It hopped 4 times and travelled about ${Math.round(s.exactKm).toLocaleString('en-US')} km across 3 networks, passing through United States, Ireland, and Netherlands.`,
      `I guessed 5,000 km and scored ${result.score}/100. Can you beat my bunny score? 🥕`,
    ]);
  });

  it('uses the unit the player picked', () => {
    const result = judgeRound(3000, s.exactKm, s.located);
    expect(tripSummary({ endpoint: 'example.com', trace: t, stats: s, result, unit: 'mi' })).toMatch(/ mi/);
  });

  it('invites a guess when none was made, and skips the distance it cannot give', () => {
    const one = normalizeTrace({ hops: [{ hopNumber: 1, hidden: true }, withCountries.hops[3]], destination: withCountries.destination });
    const text = tripSummary({ endpoint: 'example.com', trace: one, stats: summarize(one.hops, one.networks), result: { status: 'unscoreable', reason: 'few-hops' } });
    expect(text).not.toMatch(/travelled/);
    expect(text).toMatch(/Think you can guess/);
    expect(text).toMatch(/It hopped 2 times across 1 network, passing through Netherlands\./);
  });

  it('never says where the trace started', () => {
    const text = tripSummary({ endpoint: 'example.com', trace, stats, result: null });
    for (const leak of ['192.168', 'gateway', 'Omaha', 'cox.net', '68.1.', 'Local network', 'hidden']) {
      expect(text).not.toContain(leak);
    }
    // No address or host name of any hop either, only the destination the player typed.
    expect(text).not.toMatch(/\d+\.\d+\.\d+\.\d+/);
    expect(text).not.toMatch(/lumen|gtt/i);
  });

  it('has no em dashes', () => {
    expect(tripSummary({ endpoint: 'example.com', trace, stats, result: null })).not.toContain('—');
  });
});

describe('appLink', () => {
  it('links to the page unless nobody else could open it', () => {
    expect(appLink({ protocol: 'https:', hostname: 'hop.example.com', origin: 'https://hop.example.com', pathname: '/' })).toBe('https://hop.example.com/');
    expect(appLink({ protocol: 'http:', hostname: 'localhost', origin: 'http://localhost:5173', pathname: '/' })).toBeNull();
    expect(appLink({ protocol: 'http:', hostname: '127.0.0.1', origin: 'http://127.0.0.1:5173', pathname: '/' })).toBeNull();
    expect(appLink({ protocol: 'file:', hostname: '', origin: 'null', pathname: '/x' })).toBeNull();
    expect(appLink(undefined)).toBeNull();
  });
});

describe('shareTrip', () => {
  const payload = { title: 'Route Hopper', text: 'hello', url: 'https://hop.example.com/' };

  it('uses the Web Share API when there is one', async () => {
    const nav = { share: vi.fn().mockResolvedValue(), clipboard: { writeText: vi.fn() } };
    expect(await shareTrip(payload, { nav })).toBe('shared');
    expect(nav.share).toHaveBeenCalledWith(payload);
    expect(nav.clipboard.writeText).not.toHaveBeenCalled();
  });

  it('treats closing the share sheet as a cancel, not a failure', async () => {
    const nav = { share: vi.fn().mockRejectedValue(Object.assign(new Error('x'), { name: 'AbortError' })), clipboard: { writeText: vi.fn() } };
    expect(await shareTrip(payload, { nav })).toBe('cancelled');
    expect(nav.clipboard.writeText).not.toHaveBeenCalled();
  });

  it('copies to the clipboard, link included, when sharing is missing or refused', async () => {
    for (const share of [undefined, vi.fn().mockRejectedValue(Object.assign(new Error('x'), { name: 'NotAllowedError' }))]) {
      const nav = { share, clipboard: { writeText: vi.fn().mockResolvedValue() } };
      expect(await shareTrip(payload, { nav })).toBe('copied');
      expect(nav.clipboard.writeText).toHaveBeenCalledWith('hello\nhttps://hop.example.com/');
    }
    const nav = { share: vi.fn(), canShare: () => false, clipboard: { writeText: vi.fn().mockResolvedValue() } };
    expect(await shareTrip({ ...payload, url: null }, { nav })).toBe('copied');
    expect(nav.share).not.toHaveBeenCalled();
    expect(nav.clipboard.writeText).toHaveBeenCalledWith('hello');
  });

  it('falls back to selecting and copying where the clipboard API is missing', async () => {
    const area = { setAttribute: vi.fn(), select: vi.fn(), remove: vi.fn(), style: {} };
    const doc = { body: { appendChild: vi.fn() }, createElement: vi.fn(() => area), execCommand: vi.fn(() => true) };
    expect(await shareTrip(payload, { nav: {}, doc })).toBe('copied');
    expect(area.value).toBe('hello\nhttps://hop.example.com/');
    expect(doc.execCommand).toHaveBeenCalledWith('copy');
    expect(area.remove).toHaveBeenCalled();
  });

  it('reports failure when nothing works', async () => {
    const nav = { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } };
    expect(await shareTrip(payload, { nav, doc: undefined })).toBe('failed');
  });
});
