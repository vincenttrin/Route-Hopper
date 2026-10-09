import { useMemo, useRef, useState } from 'react';
import TraceInput from './components/TraceInput.jsx';
import LineDiagram from './components/LineDiagram.jsx';
import HopMap from './components/HopMap.jsx';
import LiveStatus from './components/LiveStatus.jsx';
import { normalizeTrace, summarize, LOCAL_COLOR } from './lib/trace.js';
import { toRaw } from './lib/stream.js';
import { runTrace, streamTrace, StreamUnavailableError, StreamInterruptedError } from './services/api.js';
import { SAMPLE_TRACE, SAMPLE_ENDPOINT } from './services/sample.js';

function routeTitle(hops, endpoint) {
  const places = hops.filter((h) => h.located && h.city).map((h) => h.city);
  if (places.length < 2) return endpoint;
  return `${places[0]} to ${places[places.length - 1]}`;
}

function unreachedMessage(endpoint, trace) {
  const seen = trace.lastReply ? ` Hop ${trace.lastReply} was the last to reply.` : '';
  const dest = trace.destination ? ` (${trace.destination.ip})` : '';
  return (
    `${endpoint}${dest} never replied to the trace probes.${seen} Many servers and firewalls silently drop ` +
    'traceroute probes, so this does not mean the site is down.'
  );
}

export default function App() {
  const [endpoint, setEndpoint] = useState(SAMPLE_ENDPOINT);
  const [raw, setRaw] = useState(SAMPLE_TRACE);
  const [isExample, setExample] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [selected, setSelected] = useState(null);
  // Set while hops are arriving: when the trace started and what it is doing besides probing.
  const [live, setLive] = useState(null);
  const [cancelled, setCancelled] = useState(false);
  const abort = useRef(null);

  // A streamed trace stays partial until the stream finishes; it travels with `raw` so a restored trace keeps it.
  const partial = Boolean(raw.partial);
  const trace = useMemo(() => normalizeTrace(raw, { partial }), [raw, partial]);
  const stats = useMemo(() => summarize(trace.hops, trace.networks), [trace]);
  // Where the route was heading, when the destination never answered and has a known location.
  const unreached = !partial && !trace.reached && trace.destination?.located ? trace.destination : null;

  const submit = async (value) => {
    abort.current?.abort();
    const ctl = new AbortController();
    abort.current = ctl;
    const current = () => abort.current === ctl;
    const previous = { raw, endpoint, isExample };
    let streamed = null;
    setBusy(true);
    setError(null);
    setCancelled(false);
    try {
      try {
        await streamTrace(value, 30, ctl.signal, (state) => {
          if (!current()) return;
          // The first event means the server took the request: leave the previous trace behind.
          if (!streamed) {
            setEndpoint(value);
            setExample(false);
            setSelected(null);
            setLive({ startedAt: Date.now(), phase: null });
          }
          streamed = state;
          setRaw({ ...toRaw(state), partial: true });
          setLive((l) => (l && l.phase !== state.phase ? { ...l, phase: state.phase } : l));
        });
        if (!streamed.hops.length) throw new Error('The trace returned no hops.');
        setRaw((r) => ({ ...r, partial: false }));
      } catch (e) {
        const lostBeforeAnyHop = e instanceof StreamInterruptedError && !streamed?.hops.length;
        if (!(e instanceof StreamUnavailableError) && !lostBeforeAnyHop) throw e;
        // Streaming does not work here (an old backend, a proxy in the way): trace in one piece instead.
        streamed = null;
        setLive(null);
        const data = await runTrace(value, 30, ctl.signal);
        if (!data.hops?.length) throw new Error('The trace returned no hops.');
        if (!current()) return;
        setRaw(data);
        setEndpoint(value);
        setExample(false);
        setSelected(null);
      }
    } catch (e) {
      if (!current()) return;
      if (e.name === 'AbortError') {
        setCancelled(Boolean(streamed?.hops.length));
      } else if (e instanceof StreamInterruptedError) {
        setError(`The connection to the trace service was lost after ${streamed.hops.length} hops. The route so far is shown.`);
      } else {
        setError(e.message || 'Could not reach the trace service.');
      }
      // A trace that never produced a hop leaves the previous one on screen.
      if (!streamed?.hops.length) {
        setRaw(previous.raw);
        setEndpoint(previous.endpoint);
        setExample(previous.isExample);
        setSelected(null);
      }
    } finally {
      if (current()) {
        setBusy(false);
        setLive(null);
      }
    }
  };

  const cancel = () => abort.current?.abort();

  return (
    <div className="app">
      <header className="bar">
        <div className="route">
          <span>{routeTitle(trace.hops, endpoint)}</span>
          <b>{stats.hops} hops</b>
        </div>
        <TraceInput initial={endpoint} busy={busy} onSubmit={submit} onCancel={cancel} />
      </header>
      {error && (
        <div className="notice" role="alert">
          {error}
        </div>
      )}
      {live && <LiveStatus startedAt={live.startedAt} hops={trace.hops} phase={live.phase} />}
      {cancelled && !error && (
        <div className="notice notice-info" role="status">
          Trace cancelled after {stats.hops} hops. The route so far is shown.
        </div>
      )}
      {!isExample && !error && raw.warning && (
        <div className="notice notice-warn" role="status">
          {raw.warning}
        </div>
      )}
      {!isExample && !error && !partial && !cancelled && !trace.reached && (
        <div className="notice notice-info" role="status">
          {unreachedMessage(endpoint, trace)}
        </div>
      )}
      {isExample && !error && (
        <div className="notice notice-info">Example trace. Enter an endpoint above to trace your own route.</div>
      )}
      <main className={`main${busy ? ' is-busy' : ''}`}>
        <section className="list" aria-label="Hops">
          <LineDiagram hops={trace.hops} live={Boolean(live)} selected={selected} onSelect={setSelected} />
        </section>
        <aside className="side">
          <div>
            <h2>Geography</h2>
            <HopMap hops={trace.hops} destination={unreached} selected={selected} onSelect={setSelected} />
          </div>
          <div className="stats">
            <div>
              <b>{stats.km.toLocaleString('en-US')}</b>
              <span>km between located hops</span>
            </div>
            <div>
              <b>{stats.rtt != null ? stats.rtt.toFixed(1) : '-'}</b>
              <span>ms to destination</span>
            </div>
            <div>
              <b>{stats.networks}</b>
              <span>networks crossed</span>
            </div>
          </div>
          <div>
            <h2>Networks</h2>
            <ul className="legend">
              {trace.networks.map((n) => (
                <li key={n.name}>
                  <i style={{ background: n.color || LOCAL_COLOR }} />
                  {n.name}
                </li>
              ))}
            </ul>
          </div>
        </aside>
      </main>
    </div>
  );
}
