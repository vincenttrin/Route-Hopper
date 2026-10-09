import { useMemo, useRef, useState } from 'react';
import TraceInput from './components/TraceInput.jsx';
import LineDiagram from './components/LineDiagram.jsx';
import HopMap from './components/HopMap.jsx';
import { normalizeTrace, summarize, LOCAL_COLOR } from './lib/trace.js';
import { runTrace } from './services/api.js';
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
  const abort = useRef(null);

  const trace = useMemo(() => normalizeTrace(raw), [raw]);
  const stats = useMemo(() => summarize(trace.hops, trace.networks), [trace]);
  // Where the route was heading, when the destination never answered and has a known location.
  const unreached = !trace.reached && trace.destination?.located ? trace.destination : null;

  const submit = async (value) => {
    abort.current?.abort();
    abort.current = new AbortController();
    setBusy(true);
    setError(null);
    try {
      const data = await runTrace(value, 30, abort.current.signal);
      if (!data.hops?.length) throw new Error('The trace returned no hops.');
      setRaw(data);
      setEndpoint(value);
      setExample(false);
      setSelected(null);
    } catch (e) {
      if (e.name !== 'AbortError') setError(e.message || 'Could not reach the trace service.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="app">
      <header className="bar">
        <div className="route">
          <span>{routeTitle(trace.hops, endpoint)}</span>
          <b>{stats.hops} hops</b>
        </div>
        <TraceInput initial={endpoint} busy={busy} onSubmit={submit} />
      </header>
      {error && (
        <div className="notice" role="alert">
          {error}
        </div>
      )}
      {!isExample && !error && raw.warning && (
        <div className="notice notice-warn" role="status">
          {raw.warning}
        </div>
      )}
      {!isExample && !error && !trace.reached && (
        <div className="notice notice-info" role="status">
          {unreachedMessage(endpoint, trace)}
        </div>
      )}
      {isExample && !error && (
        <div className="notice notice-info">Example trace. Enter an endpoint above to trace your own route.</div>
      )}
      <main className={`main${busy ? ' is-busy' : ''}`}>
        <section className="list" aria-label="Hops">
          <LineDiagram hops={trace.hops} selected={selected} onSelect={setSelected} />
        </section>
        <aside className="side">
          <div className="map-panel">
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
