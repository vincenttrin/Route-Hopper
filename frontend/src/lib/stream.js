/**
 * Reads newline-delimited JSON from a fetch body and calls `onEvent` with each
 * parsed object as soon as its line is complete. Returns how many events it saw.
 */
export async function readEvents(body, onEvent) {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let pending = '';
  let count = 0;
  const flush = (line) => {
    if (!line.trim()) return;
    count++;
    onEvent(JSON.parse(line));
  };
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    pending += decoder.decode(value, { stream: true });
    let nl = pending.indexOf('\n');
    while (nl >= 0) {
      flush(pending.slice(0, nl));
      pending = pending.slice(nl + 1);
      nl = pending.indexOf('\n');
    }
  }
  pending += decoder.decode();
  flush(pending);
  return count;
}

export const EMPTY_STREAM = {
  hops: [],
  destination: null,
  reached: undefined,
  warning: undefined,
  phase: null,
  done: false,
  error: null,
};

/**
 * Folds one stream event into the trace seen so far. Event types are documented
 * in backend/README.md: start, hop, phase, reset, done and error.
 */
export function reduceStream(state, ev) {
  switch (ev.type) {
    case 'start':
      return { ...EMPTY_STREAM, destination: ev.destination ?? null };
    case 'hop':
      return { ...state, hops: [...state.hops, ev.hop] };
    case 'phase':
      return { ...state, phase: ev.phase };
    case 'reset':
      // The ICMP retry got further than the UDP pass, whose hops are dropped.
      return { ...state, hops: [], phase: null };
    case 'done':
      return {
        ...state,
        destination: ev.destination ?? state.destination,
        reached: ev.reached !== false,
        warning: ev.warning,
        phase: null,
        done: true,
      };
    case 'error':
      return { ...state, error: ev.error || 'The trace failed.', phase: null };
    default:
      return state;
  }
}

/** The shape normalizeTrace takes, for a trace that may still be running. */
export function toRaw(state) {
  return {
    hops: state.hops,
    destination: state.destination,
    reached: state.done ? state.reached : undefined,
    warning: state.warning,
  };
}
