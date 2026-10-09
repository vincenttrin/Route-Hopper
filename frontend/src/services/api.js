import { readEvents, reduceStream, EMPTY_STREAM } from '../lib/stream.js';

export async function runTrace(endpoint, maxHops = 30, signal) {
  const res = await fetch('/api/trace', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ endpoint, maxHops }),
    signal,
  });
  if (!res.ok) {
    const text = (await res.text()).trim();
    throw new Error(text || `Trace failed (HTTP ${res.status})`);
  }
  return res.json();
}

/** Streaming cannot be used here, but the plain trace endpoint may still work. */
export class StreamUnavailableError extends Error {}

/** The stream ended before the trace finished; what arrived so far is still valid. */
export class StreamInterruptedError extends Error {}

// A backend or proxy that predates /api/trace/stream answers with one of these.
const NOT_STREAMING = new Set([404, 405, 501]);

/**
 * Traces `endpoint` over POST /api/trace/stream and calls `onUpdate(state)` with
 * the trace so far (see reduceStream) after every event, so hops can be shown as
 * they are found. Resolves with the final state. Rejects with:
 * - StreamUnavailableError when nothing could be streamed (the caller can fall back to runTrace),
 * - StreamInterruptedError when the connection dropped mid-trace,
 * - an Error carrying the server's message when it rejected or failed the trace,
 * - the AbortError of `signal` when cancelled.
 */
export async function streamTrace(endpoint, maxHops = 30, signal, onUpdate) {
  let res;
  try {
    res = await fetch('/api/trace/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ endpoint, maxHops }),
      signal,
    });
  } catch (e) {
    if (e.name === 'AbortError') throw e;
    throw new StreamUnavailableError(e.message);
  }
  if (!res.ok) {
    if (NOT_STREAMING.has(res.status)) throw new StreamUnavailableError(`HTTP ${res.status}`);
    const text = (await res.text()).trim();
    throw new Error(text || `Trace failed (HTTP ${res.status})`);
  }
  if (!res.body) throw new StreamUnavailableError('response is not readable as a stream');

  let state = EMPTY_STREAM;
  let seen = 0;
  try {
    await readEvents(res.body, (ev) => {
      seen++;
      state = reduceStream(state, ev);
      onUpdate(state);
    });
  } catch (e) {
    if (e.name === 'AbortError') throw e;
    if (!seen) throw new StreamUnavailableError(e.message);
    throw new StreamInterruptedError(e.message);
  }
  if (state.error) throw new Error(state.error);
  if (!state.done) {
    if (!seen) throw new StreamUnavailableError('empty response');
    throw new StreamInterruptedError('connection closed before the trace finished');
  }
  return state;
}
