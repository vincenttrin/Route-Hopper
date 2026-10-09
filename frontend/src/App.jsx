import { useEffect, useMemo, useRef, useState } from 'react';
import TraceInput from './components/TraceInput.jsx';
import LineDiagram from './components/LineDiagram.jsx';
import HopMap from './components/HopMap.jsx';
import LiveStatus from './components/LiveStatus.jsx';
import GamePanel from './components/GamePanel.jsx';
import GeoNote from './components/GeoNote.jsx';
import Bunny from './components/Bunny.jsx';
import { normalizeTrace, summarize, LOCAL_COLOR } from './lib/trace.js';
import { toRaw } from './lib/stream.js';
import { usePacedReveal, HOP_MS } from './lib/pace.js';
import { tripSummary } from './lib/share.js';
import { parseGuess, judgeRound, unscoreableReason, fromKm, UNITS } from './lib/game.js';
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
  // Every hop found so far. Hops reach the screen later, at the pace set in lib/pace.js.
  const [raw, setRaw] = useState(SAMPLE_TRACE);
  const [isExample, setExample] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [selected, setSelected] = useState(null);
  // Set while hops are arriving: when the trace started and what it is doing besides probing.
  const [live, setLive] = useState(null);
  const [cancelled, setCancelled] = useState(false);
  const [guess, setGuess] = useState('');
  const [unit, setUnit] = useState('km');
  const [guessError, setGuessError] = useState(null);
  // The round being played: set when a trace starts (`guessKm` stays null until the player guesses once the route is shown),
  // cleared by Play again or a trace that fails.
  const [round, setRound] = useState(null);
  const [best, setBest] = useState(null);
  const [shown, setShown] = usePacedReveal(raw.hops.length, SAMPLE_TRACE.hops.length);
  // False from the first hop of a trace until the bunny has landed on the last one.
  const [landed, setLanded] = useState(true);
  const abort = useRef(null);
  const previous = useRef(null);
  const tracing = useRef(false);
  const guessRef = useRef(null);
  const endpointRef = useRef(null);

  // The trace keeps hopping after the last hop is found, until every hop found has been shown.
  const hopping = busy || shown < raw.hops.length;
  // A trace stays partial until its stream finishes and every hop is on screen; it travels with `raw` so a restored trace keeps it.
  const partial = Boolean(raw.partial) || shown < raw.hops.length;
  const view = useMemo(() => ({ ...raw, hops: raw.hops.slice(0, shown) }), [raw, shown]);
  const trace = useMemo(() => normalizeTrace(view, { partial }), [view, partial]);
  const stats = useMemo(() => summarize(trace.hops, trace.networks), [trace]);
  // Where the route was heading, when the destination never answered and has a known location.
  const unreached = !partial && !trace.reached && trace.destination?.located ? trace.destination : null;

  // A round is played once its trace has run to the end; one that was cancelled or cut off is never scored.
  const finished = Boolean(round) && !hopping && !cancelled && !error && !raw.partial;
  const result = useMemo(() => {
    if (!finished) return null;
    const reason = unscoreableReason(stats.exactKm, stats.located);
    if (reason) return { status: 'unscoreable', reason };
    return round.guessKm == null ? null : judgeRound(round.guessKm, stats.exactKm, stats.located);
  }, [finished, round, stats]);
  // `guessing` is a finished, playable route still waiting for the player's guess.
  useEffect(() => {
    if (hopping) {
      setLanded(false);
      return undefined;
    }
    // Let the last hop finish before the game moves on; with reduced motion there is no jump to wait for.
    const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const id = setTimeout(() => setLanded(true), still ? 0 : HOP_MS);
    return () => clearTimeout(id);
  }, [hopping]);
  const status = !round ? 'ready' : hopping || (finished && !landed) ? 'running' : !finished ? 'incomplete' : (result?.status ?? 'guessing');
  // A finished round can be shared once it has its score, or when there is nothing to guess. The example is nobody's trip.
  const shareText = useMemo(
    () => (!isExample && (status === 'scored' || status === 'unscoreable') ? tripSummary({ endpoint, trace, stats, result, unit }) : null),
    [isExample, status, endpoint, trace, stats, result, unit],
  );
  useEffect(() => {
    if (status === 'guessing') guessRef.current?.focus();
  }, [status]);
  useEffect(() => {
    if (result?.status === 'scored') setBest((b) => Math.max(b ?? 0, result.score));
  }, [result]);

  const restore = () => {
    const p = previous.current;
    setRaw(p.raw);
    setShown(p.raw.hops.length);
    setEndpoint(p.endpoint);
    setExample(p.isExample);
    setSelected(null);
    setRound(null);
  };

  const submit = async (value) => {
    abort.current?.abort();
    const ctl = new AbortController();
    abort.current = ctl;
    const current = () => abort.current === ctl && !ctl.signal.aborted;
    previous.current = { raw, endpoint, isExample };
    tracing.current = false;
    const startedAt = Date.now();
    let streamed = null;
    setBusy(true);
    setError(null);
    setGuess('');
    setGuessError(null);
    setCancelled(false);
    setLive(null);
    setRound({ guessKm: null });
    try {
      try {
        await streamTrace(value, 30, ctl.signal, (state) => {
          if (!current()) return;
          // The first event means the server took the request: leave the previous trace behind.
          if (!streamed) {
            tracing.current = true;
            setEndpoint(value);
            setExample(false);
            setSelected(null);
            setShown(0);
            setLive({ startedAt, phase: null });
          }
          streamed = state;
          setRaw({ ...toRaw(state), partial: true });
          // A reset drops the hops found so far; the ones already shown go with them.
          setShown((n) => Math.min(n, state.hops.length));
          setLive((l) => (l && l.phase !== state.phase ? { ...l, phase: state.phase } : l));
        });
        if (!streamed.hops.length) throw new Error('The trace returned no hops.');
        setRaw((r) => ({ ...r, partial: false }));
      } catch (e) {
        const lostBeforeAnyHop = e instanceof StreamInterruptedError && !streamed?.hops.length;
        if (!(e instanceof StreamUnavailableError) && !lostBeforeAnyHop) throw e;
        // Streaming does not work here (an old backend, a proxy in the way): trace in one piece instead.
        streamed = null;
        const data = await runTrace(value, 30, ctl.signal);
        if (!data.hops?.length) throw new Error('The trace returned no hops.');
        if (!current()) return;
        tracing.current = true;
        setRaw(data);
        setShown(0);
        setLive({ startedAt, phase: null });
        setEndpoint(value);
        setExample(false);
        setSelected(null);
      }
    } catch (e) {
      if (!current()) return;
      if (e instanceof StreamInterruptedError) {
        setError(`The connection to the trace service was lost after ${streamed.hops.length} hops. The route so far is shown.`);
      } else {
        setError(e.message || 'Could not reach the trace service.');
      }
      // A trace that never produced a hop leaves the previous one on screen.
      if (!streamed?.hops.length) restore();
    } finally {
      if (abort.current === ctl) setBusy(false);
    }
  };

  // Stops the trace right away: hops found but not yet shown are dropped, so it ends where the bunny is.
  const cancel = () => {
    abort.current?.abort();
    if (!tracing.current || shown === 0) {
      restore();
      return;
    }
    setRaw((r) => ({ ...r, hops: r.hops.slice(0, shown), partial: true }));
    setCancelled(true);
  };

  const submitGuess = () => {
    const parsed = parseGuess(guess, unit);
    if (!parsed.ok) {
      setGuessError(parsed.error);
      guessRef.current?.focus();
      return;
    }
    setGuessError(null);
    setRound({ guessKm: parsed.km });
  };

  const playAgain = () => {
    setRound(null);
    setGuess('');
    setGuessError(null);
    endpointRef.current?.focus();
  };

  const distance = Math.round(fromKm(stats.exactKm, unit)).toLocaleString('en-US');

  return (
    <div className="app">
      <header className="bar">
        <div className="brand">
          <Bunny size={44} className="brand-bunny" />
          <div className="brand-text">
            <span className="brand-name">Route Hopper</span>
            <span className="brand-tag">Follow the bunny along the packet's path</span>
          </div>
        </div>
        <div className="route">
          <span>{routeTitle(trace.hops, endpoint)}</span>
          <b>{stats.hops} hops</b>
        </div>
        <TraceInput initial={endpoint} busy={hopping} endpointRef={endpointRef} onSubmit={submit} onCancel={cancel} />
      </header>
      {error && (
        <div className="notice" role="alert">
          {error}
        </div>
      )}
      {hopping && live && <LiveStatus startedAt={live.startedAt} found={raw.hops.length} shown={shown} probing={busy} phase={live.phase} />}
      {cancelled && !error && (
        <div className="notice notice-info" role="status">
          Trace cancelled after {stats.hops} hops. The bunny stopped where it was, and this round is not scored.
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
        <div className="notice notice-info">Example trace. Enter an endpoint above and press Hop! to send the bunny down your own route, then guess how far its packet travelled.</div>
      )}
      <main className={`main${busy && !live ? ' is-busy' : ''}`}>
        <section className="list" aria-label="Hops">
          <LineDiagram hops={trace.hops} live={hopping} selected={selected} onSelect={setSelected} />
        </section>
        <aside className="side">
          <GamePanel
            status={status}
            unit={unit}
            guess={guess}
            guessError={guessError}
            guessRef={guessRef}
            result={result}
            best={best}
            shareText={shareText}
            onGuess={(v) => {
              setGuess(v);
              setGuessError(null);
            }}
            onUnit={setUnit}
            onSubmitGuess={submitGuess}
            onPlayAgain={playAgain}
          />
          <div className="map-panel">
            <h2>Hop map</h2>
            <HopMap hops={trace.hops} destination={unreached} selected={selected} onSelect={setSelected} />
          </div>
          <div className="stats">
            <div>
              <b>{status === 'running' || status === 'guessing' ? '?' : distance}</b>
              <span>{UNITS[unit].label} between located hops</span>
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
      <GeoNote />
    </div>
  );
}
