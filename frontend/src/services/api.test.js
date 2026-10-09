import { afterEach, describe, expect, it, vi } from 'vitest';
import { getGeoInfo, streamTrace, StreamInterruptedError, StreamUnavailableError } from './api.js';

const ndjson = (events, { status = 200, tail = '' } = {}) =>
  new Response(events.map((e) => JSON.stringify(e) + '\n').join('') + tail, { status });

const hop = (n) => ({ type: 'hop', hop: { hopNumber: n, ip: `10.0.0.${n}` } });
const START = { type: 'start', destination: { ip: '1.1.1.1' } };
const DONE = { type: 'done', reached: true };

function mockFetch(impl) {
  const fn = vi.fn(impl);
  vi.stubGlobal('fetch', fn);
  return fn;
}

afterEach(() => vi.unstubAllGlobals());

describe('streamTrace', () => {
  it('posts to the streaming endpoint and reports the trace after every event', async () => {
    const fetchMock = mockFetch(async () => ndjson([START, hop(1), hop(2), DONE]));
    const updates = [];
    const final = await streamTrace('example.com', 12, undefined, (s) => updates.push(s.hops.length));
    expect(updates).toEqual([0, 1, 2, 2]);
    expect(final.done).toBe(true);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/trace/stream');
    expect(JSON.parse(init.body)).toEqual({ endpoint: 'example.com', maxHops: 12 });
  });

  it('surfaces the server message for a rejected request, without falling back', async () => {
    mockFetch(async () => new Response('enter a valid host name or IP address\n', { status: 400 }));
    const err = await streamTrace('-m 1', 30, undefined, () => {}).catch((e) => e);
    expect(err).not.toBeInstanceOf(StreamUnavailableError);
    expect(err.message).toBe('enter a valid host name or IP address');
  });

  it('treats a busy or rate limited server as an error to show', async () => {
    mockFetch(async () => new Response('too many traces are running, try again shortly', { status: 503 }));
    const err = await streamTrace('example.com', 30, undefined, () => {}).catch((e) => e);
    expect(err).not.toBeInstanceOf(StreamUnavailableError);
  });

  it.each([404, 405, 501])('reports streaming unavailable for HTTP %i', async (status) => {
    mockFetch(async () => new Response('nope', { status }));
    await expect(streamTrace('example.com', 30, undefined, () => {})).rejects.toBeInstanceOf(StreamUnavailableError);
  });

  it('reports streaming unavailable when the request cannot be made', async () => {
    mockFetch(async () => {
      throw new TypeError('Failed to fetch');
    });
    await expect(streamTrace('example.com', 30, undefined, () => {})).rejects.toBeInstanceOf(StreamUnavailableError);
  });

  it('reports a server error event with its message, after delivering the hops before it', async () => {
    mockFetch(async () => ndjson([START, hop(1), { type: 'error', error: 'trace timed out', status: 504 }]));
    let hops = 0;
    const err = await streamTrace('example.com', 30, undefined, (s) => (hops = s.hops.length)).catch((e) => e);
    expect(err.message).toBe('trace timed out');
    expect(hops).toBe(1);
  });

  it('reports an interrupted stream when the connection closes before done', async () => {
    mockFetch(async () => ndjson([START, hop(1)]));
    await expect(streamTrace('example.com', 30, undefined, () => {})).rejects.toBeInstanceOf(StreamInterruptedError);
  });

  it('reports streaming unavailable when the body is empty', async () => {
    mockFetch(async () => new Response('', { status: 200 }));
    await expect(streamTrace('example.com', 30, undefined, () => {})).rejects.toBeInstanceOf(StreamUnavailableError);
  });

  it('rejects with AbortError when cancelled', async () => {
    const ctl = new AbortController();
    mockFetch(
      (_url, init) =>
        new Promise((_, reject) => {
          init.signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
        }),
    );
    const pending = streamTrace('example.com', 30, ctl.signal, () => {});
    ctl.abort();
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
  });
});

describe('getGeoInfo', () => {
  const json = (body, status = 200) => new Response(JSON.stringify(body), { status });

  it('returns what the backend reports about its database', async () => {
    const fetchMock = mockFetch(async () => json({ available: true, provider: 'DB-IP Lite', updated: '2026-10-01' }));
    expect(await getGeoInfo()).toEqual({ available: true, provider: 'DB-IP Lite', updated: '2026-10-01' });
    expect(fetchMock.mock.calls[0][0]).toBe('/api/geoip');
  });

  it('returns null when the backend cannot say', async () => {
    mockFetch(async () => json({}, 404));
    expect(await getGeoInfo()).toBeNull();
    mockFetch(async () => json({ nope: 1 }));
    expect(await getGeoInfo()).toBeNull();
    mockFetch(async () => new Response('<html>', { status: 200 }));
    expect(await getGeoInfo()).toBeNull();
    mockFetch(async () => {
      throw new TypeError('offline');
    });
    expect(await getGeoInfo()).toBeNull();
  });
});
