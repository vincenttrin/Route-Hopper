import { LOCAL_COLOR } from '../lib/trace.js';

const ROW = 58;
const LANE_X = [22, 56];
const MAX_RTT_BAR = 92;

function Track({ hops }) {
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
              <circle cx={cx} cy={cy} r="11" className="st-dest" />
              <circle cx={cx} cy={cy} r="5" className="st-dest-core" />
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
    </svg>
  );
}

export default function LineDiagram({ hops, selected, onSelect }) {
  const maxRtt = Math.max(1, ...hops.map((h) => h.rtt || 0));
  return (
    <ol className="line-list" style={{ '--row': `${ROW}px` }}>
      <Track hops={hops} />
      {hops.map((h) => {
        const pad = String(h.n).padStart(2, '0');
        const title = h.network ? h.city || h.hostname || h.ip : 'No reply';
        const meta = h.network
          ? [`HOP ${pad}`, h.hostname, h.ip].filter(Boolean).join(' - ')
          : `HOP ${pad} - * * * - probe timed out, route continues`;
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
                      {h.network}
                    </span>
                  ) : (
                    <span className="chip chip-unknown">Unknown</span>
                  )}
                  {h.network && !h.located && <span className="chip chip-outline">Not located</span>}
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
