import { useEffect, useRef } from 'react';
import { LOCAL, LOCAL_COLOR } from '../lib/trace.js';
import { BunnyArt, CarrotArt } from './Bunny.jsx';

const ROW = 58;
const LANE_X = [22, 56];
const MAX_RTT_BAR = 92;
const BUNNY = 32;

function Track({ hops }) {
  const last = hops.length - 1;
  const y = (i) => i * ROW + ROW / 2;
  const x = (i) => LANE_X[hops[i].lane];
  const segments = [];
  for (let i = 0; i < hops.length - 1; i++) {
    const a = hops[i];
    const b = hops[i + 1];
    const jog = Math.abs(x(i + 1) - x(i));
    const d =
      jog === 0
        ? `M${x(i)} ${y(i)}L${x(i + 1)} ${y(i + 1)}`
        : `M${x(i)} ${y(i)}L${x(i)} ${y(i + 1) - jog}L${x(i + 1)} ${y(i + 1)}`;
    const gap = !a.network || !b.network;
    segments.push(
      <path
        key={i}
        d={d}
        fill="none"
        stroke={b.color || a.color || LOCAL_COLOR}
        strokeWidth="10"
        strokeDasharray={gap ? '3 6' : undefined}
      />,
    );
  }
  return (
    <svg className="track" viewBox={`0 0 90 ${hops.length * ROW}`} width="90" height={hops.length * ROW} aria-hidden="true">
      {segments}
      {hops.map((h, i) => {
        const cx = x(i);
        const cy = y(i);
        if (!h.network) {
          return <circle key={i} cx={cx} cy={cy} r="6" className="st st-miss" />;
        }
        if (h.destination) {
          return (
            <g key={i}>
              <circle cx={cx} cy={cy} r="13" className="st-dest" />
              <g transform={`translate(${cx - 9} ${cy - 12}) scale(.75)`}>
                <CarrotArt />
              </g>
            </g>
          );
        }
        if (h.change) {
          return (
            <g key={i}>
              <circle cx={cx} cy={cy} r="11" className="st st-change" />
              <circle cx={cx} cy={cy} r="3.5" className="st-dot" />
            </g>
          );
        }
        return <circle key={i} cx={cx} cy={cy} r="6.5" className="st st-plain" />;
      })}
      {last >= 0 && (
        // The bunny waits on the newest hop and glides to the next one. Its first position never glides, so a new trace does not drag it across the old one.
        <g className={`bunny${last === 0 ? ' bunny-snap' : ''}`} style={{ transform: `translate(${x(last) - BUNNY / 2}px, ${y(last) - BUNNY - 11}px)` }}>
          <g key={last} className="bunny-leap">
            <g transform={`scale(${BUNNY / 48})`}>
              <BunnyArt />
            </g>
          </g>
        </g>
      )}
    </svg>
  );
}

const NARROW = '(max-width: 899px)';

export default function LineDiagram({ hops, live = false, selected, onSelect }) {
  const maxRtt = Math.max(1, ...hops.map((h) => h.rtt || 0));
  const list = useRef(null);
  // Follow the bunny down a long route. On a narrow screen the list sits below the map, so leave the page where it is.
  useEffect(() => {
    if (!live || window.matchMedia(NARROW).matches) return;
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    list.current?.lastElementChild?.scrollIntoView?.({ block: 'nearest', behavior: reduced ? 'auto' : 'smooth' });
  }, [live, hops.length]);
  return (
    <ol className={`line-list${live ? ' is-live' : ''}`} ref={list} style={{ '--row': `${ROW}px` }}>
      <Track hops={hops} />
      {hops.map((h) => {
        const pad = String(h.n).padStart(2, '0');
        const title = h.hidden ? 'Hidden start' : h.network ? h.city || h.hostname || h.ip : 'No reply';
        const ends = !live && h === hops[hops.length - 1];
        const silentMeta =
          h.span > 1
            ? `HOPS ${pad}-${String(h.n + h.span - 1).padStart(2, '0')} - * * * - no reply, the trace ends here`
            : `HOP ${pad} - * * * - ${ends ? 'no reply, the trace ends here' : 'probe timed out, route continues'}`;
        const meta = h.hidden
          ? `HOP ${pad} - hidden for privacy`
          : h.network
            ? [`HOP ${pad}`, h.hostname, h.ip].filter(Boolean).join(' - ')
            : silentMeta;
        const selectable = h.located;
        return (
          <li key={h.n} className={`row${selected === h.n ? ' is-selected' : ''}${h.network ? '' : ' is-miss'}`}>
            <button
              type="button"
              className="row-btn"
              disabled={!selectable}
              aria-pressed={selected === h.n}
              onClick={() => onSelect(h.n)}
            >
              <span className="track-gap" />
              <span className="row-main">
                <span className="city">
                  {title}
                  {h.network ? (
                    <span className="chip" style={{ background: h.color }}>
                      {h.hidden ? 'Private' : h.network}
                    </span>
                  ) : (
                    <span className="chip chip-unknown">Unknown</span>
                  )}
                  {h.network && h.network !== LOCAL && !h.located && <span className="chip chip-outline">Not located</span>}
                </span>
                <span className="meta">{meta}</span>
              </span>
              <span className="rtt">
                {h.rtt != null ? (
                  <>
                    {h.rtt.toFixed(1)}
                    <small>ms</small>
                    <i style={{ width: Math.max(6, (h.rtt / maxRtt) * MAX_RTT_BAR), background: h.color }} />
                  </>
                ) : (
                  '-'
                )}
              </span>
            </button>
          </li>
        );
      })}
    </ol>
  );
}
