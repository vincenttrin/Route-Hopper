import { describe, expect, it, vi } from 'vitest';
import { appLink, endpointLabel, hopLine, shareTrip, tripSummary } from './share.js';
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

describe('hopLine', () => {
  const hop = (network, span) => ({ network, span });

  it('draws a bunny, a tile per hop coloured by network, and a carrot', () => {
    const hops = [hop('Local network'), hop('Cox'), hop('Cox'), hop('Lumen'), hop(null), hop('Cox'), hop('GTT')];
    expect(hopLine(hops)).toBe('🐇⬜🟪🟪🟦⬛🟪🟩🥕');
  });

  it('counts a folded run of silent hops as that many tiles and drops the carrot when the destination never replied', () => {
    expect(hopLine([hop('Cox'), hop(null, 3)], false)).toBe('🐇🟪⬛⬛⬛');
  });

  it('stays short for a long route', () => {
    const hops = Array.from({ length: 30 }, () => hop('Cox'));
    expect([...hopLine(hops)].length).toBeLessThanOrEqual(24);
    expect(hopLine(hops)).toContain('…');
  });
});

describe('tripSummary', () => {
  it('is a short card with the title, score and the hop line, and no distances', () => {
    const result = judgeRound(5000, stats.exactKm, stats.located);
    const text = tripSummary({ endpoint: 'https://example.com/path?x=1', trace, stats, result });
    expect(text.split('\n')).toEqual(['🐰 Route Hopper: example.com', `Score ${result.score}/100`, hopLine(trace.hops)]);
    expect(text).not.toMatch(/km|mi\b|Guessed|actual/);
  });

  it('shows the hop count instead of a score when nothing was guessed, without the distance', () => {
    const text = tripSummary({ endpoint: 'example.com', trace, stats, result: null });
    expect(text.split('\n')).toEqual(['🐰 Route Hopper: example.com', `${stats.hops} hops`, hopLine(trace.hops)]);
  });

  it('uses the singular for one hop', () => {
    const one = normalizeTrace({ hops: [{ hopNumber: 1, ip: '93.184.216.34', city: 'Amsterdam', lat: 52.37, lng: 4.9 }], destination: { ip: '93.184.216.34' } });
    const text = tripSummary({ endpoint: 'example.com', trace: one, stats: summarize(one.hops, one.networks), result: { status: 'unscoreable', reason: 'few-hops' } });
    expect(text.split('\n')[1]).toBe('1 hop');
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
  const payload = { text: 'hello', url: 'https://hop.example.com/' };
  const full = 'hello\nhttps://hop.example.com/';

  it('shares the card and the link together in the text, with no separate url or title', async () => {
    const nav = { share: vi.fn().mockResolvedValue(), clipboard: { writeText: vi.fn() } };
    expect(await shareTrip(payload, { nav })).toBe('shared');
    expect(nav.share).toHaveBeenCalledWith({ text: full });
    expect(nav.clipboard.writeText).not.toHaveBeenCalled();
  });

  it('shares just the card when there is no link', async () => {
    const nav = { share: vi.fn().mockResolvedValue() };
    await shareTrip({ text: 'hello', url: null }, { nav });
    expect(nav.share).toHaveBeenCalledWith({ text: 'hello' });
  });

  it('treats closing the share sheet as a cancel, not a failure', async () => {
    const nav = { share: vi.fn().mockRejectedValue(Object.assign(new Error('x'), { name: 'AbortError' })), clipboard: { writeText: vi.fn() } };
    expect(await shareTrip(payload, { nav })).toBe('cancelled');
    expect(nav.clipboard.writeText).not.toHaveBeenCalled();
  });

  it('copies the same text to the clipboard when sharing is missing or refused', async () => {
    for (const share of [undefined, vi.fn().mockRejectedValue(Object.assign(new Error('x'), { name: 'NotAllowedError' }))]) {
      const nav = { share, clipboard: { writeText: vi.fn().mockResolvedValue() } };
      expect(await shareTrip(payload, { nav })).toBe('copied');
      expect(nav.clipboard.writeText).toHaveBeenCalledWith(full);
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
    expect(area.value).toBe(full);
    expect(doc.execCommand).toHaveBeenCalledWith('copy');
    expect(area.remove).toHaveBeenCalled();
  });

  it('reports failure when nothing works', async () => {
    const nav = { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } };
    expect(await shareTrip(payload, { nav, doc: undefined })).toBe('failed');
  });
});
