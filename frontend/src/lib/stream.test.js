import { describe, expect, it } from 'vitest';
import { EMPTY_STREAM, readEvents, reduceStream, toRaw } from './stream.js';

const bodyOf = (...chunks) =>
  new ReadableStream({
    start(c) {
      const enc = new TextEncoder();
      chunks.forEach((ch) => c.enqueue(enc.encode(ch)));
      c.close();
    },
  });

describe('readEvents', () => {
  it('parses one event per line', async () => {
    const seen = [];
    const n = await readEvents(bodyOf('{"type":"start"}\n{"type":"hop","hop":{"hopNumber":1}}\n'), (e) => seen.push(e));
    expect(n).toBe(2);
    expect(seen.map((e) => e.type)).toEqual(['start', 'hop']);
  });

  it('reassembles lines split across chunks, even inside a multi-byte character', async () => {
    const line = '{"type":"hop","hop":{"hopNumber":2,"city":"Zürich"}}\n';
    const bytes = new TextEncoder().encode(line);
    const cut = bytes.indexOf(0xc3) + 1; // between the two bytes of "ü"
    const body = new ReadableStream({
      start(c) {
        c.enqueue(bytes.slice(0, cut));
        c.enqueue(bytes.slice(cut));
        c.close();
      },
    });
    const seen = [];
    await readEvents(body, (e) => seen.push(e));
    expect(seen).toEqual([{ type: 'hop', hop: { hopNumber: 2, city: 'Zürich' } }]);
  });

  it('delivers an event as soon as its line is complete, before the stream ends', async () => {
    let push;
    let finish;
    const body = new ReadableStream({
      start(c) {
        push = (s) => c.enqueue(new TextEncoder().encode(s));
        finish = () => c.close();
      },
    });
    const seen = [];
    const reading = readEvents(body, (e) => seen.push(e.type));
    push('{"type":"start"}\n{"type":"ho');
    await new Promise((r) => setTimeout(r, 10));
    expect(seen).toEqual(['start']);
    push('p"}\n');
    finish();
    await reading;
    expect(seen).toEqual(['start', 'hop']);
  });

  it('handles a last line without a newline and skips blank lines', async () => {
    const seen = [];
    await readEvents(bodyOf('\n{"type":"start"}\n\n{"type":"done"}'), (e) => seen.push(e.type));
    expect(seen).toEqual(['start', 'done']);
  });
});

describe('reduceStream', () => {
  const run = (...events) => events.reduce(reduceStream, EMPTY_STREAM);
  const dest = { ip: '1.1.1.1', lat: 1, lng: 2 };
  const hop = (n) => ({ type: 'hop', hop: { hopNumber: n, ip: `10.0.0.${n}` } });

  it('collects hops in arrival order', () => {
    const s = run({ type: 'start', destination: dest }, hop(1), hop(2));
    expect(s.hops.map((h) => h.hopNumber)).toEqual([1, 2]);
    expect(s.destination).toEqual(dest);
    expect(s.done).toBe(false);
  });

  it('shows the ICMP retry as a phase and drops the UDP hops on reset', () => {
    const mid = run({ type: 'start' }, hop(1), hop(2), { type: 'phase', phase: 'icmp' });
    expect(mid.phase).toBe('icmp');
    expect(mid.hops).toHaveLength(2);
    const after = reduceStream(mid, { type: 'reset' });
    expect(after.hops).toEqual([]);
    expect(after.phase).toBeNull();
  });

  it('keeps hops and records the outcome when the trace finishes', () => {
    const s = run({ type: 'start', destination: dest }, hop(1), { type: 'done', destination: dest, reached: false, warning: 'w' });
    expect(s).toMatchObject({ done: true, reached: false, warning: 'w' });
    expect(toRaw(s)).toMatchObject({ reached: false, warning: 'w' });
  });

  it('only reports reached once the trace is done', () => {
    expect(toRaw(run({ type: 'start' }, hop(1))).reached).toBeUndefined();
  });

  it('keeps the hops seen so far when the server reports an error', () => {
    const s = run({ type: 'start' }, hop(1), { type: 'error', error: 'trace timed out', status: 504 });
    expect(s.error).toBe('trace timed out');
    expect(s.hops).toHaveLength(1);
  });

  it('ignores event types it does not know', () => {
    expect(run({ type: 'start' }, { type: 'from-the-future' }).hops).toEqual([]);
  });
});
