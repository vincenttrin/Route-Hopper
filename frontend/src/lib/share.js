// The share message: a short, game-style card about a finished trip, in the spirit of daily puzzle
// shares (a title, the score, a line of emoji tiles, the link). It is built to fit in a text message.
import { formatDistance, MIN_SCOREABLE_KM } from './game.js';
import { LOCAL } from './trace.js';

/** The host the player asked about, without scheme, credentials, path or query ("https://a:b@Example.com/x?t=1" gives "example.com"). */
export function endpointLabel(endpoint) {
  const text = String(endpoint ?? '').trim();
  if (!text) return 'somewhere';
  try {
    const url = new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(text) ? text : `http://${text}`);
    return url.hostname.replace(/^\[|\]$/g, '') || text;
  } catch {
    return text.split(/[/?#]/)[0] || text;
  }
}

// One tile per network crossed, in the order the route reaches them.
const NETWORK_TILES = ['🟪', '🟦', '🟩', '🟨', '🟫', '🟥'];
const LOCAL_TILE = '⬜';
const SILENT_TILE = '⬛';
const DEST_TILE = '🥕';
// A text message should stay short even for a 30-hop route.
const MAX_TILES = 20;

/** The hop line: a bunny, one tile per hop coloured by network (white for the local network, black for no reply), and a carrot at the end. */
export function hopLine(hops, reached = true) {
  const tiles = new Map();
  const out = [];
  for (const h of hops) {
    let tile = SILENT_TILE;
    if (h.network === LOCAL) tile = LOCAL_TILE;
    else if (h.network) {
      if (!tiles.has(h.network)) tiles.set(h.network, NETWORK_TILES[tiles.size % NETWORK_TILES.length]);
      tile = tiles.get(h.network);
    }
    for (let i = 0; i < (h.span ?? 1); i++) out.push(tile);
  }
  const shown = out.length > MAX_TILES ? [...out.slice(0, MAX_TILES - 1), '…'] : out;
  return `🐇${shown.join('')}${reached ? DEST_TILE : ''}`;
}

/**
 * The text to share for a finished trip, a few short lines:
 *
 *   🐰 Route Hopper: example.com
 *   Score 87/100
 *   Guessed 5,000 km, actual 7,432 km
 *   🐇⬜🟪🟦🟦🟩🥕
 *
 * `trace` and `stats` are the normalized trace and its summary; `result` is the judged round (null or
 * unscoreable when there was nothing to guess, which shows the trip's facts instead of a score).
 */
export function tripSummary({ endpoint, trace, stats, result, unit = 'km' }) {
  const lines = [`🐰 Route Hopper: ${endpointLabel(endpoint)}`];
  const hasDistance = stats.located >= 2 && stats.exactKm >= MIN_SCOREABLE_KM;
  if (result?.status === 'scored') {
    lines.push(`Score ${result.score}/100`);
    lines.push(`Guessed ${formatDistance(result.guessKm, unit)}, actual ${formatDistance(stats.exactKm, unit)}`);
  } else {
    const facts = [`${stats.hops} ${stats.hops === 1 ? 'hop' : 'hops'}`];
    if (hasDistance) facts.push(formatDistance(stats.exactKm, unit));
    lines.push(facts.join(', '));
  }
  lines.push(hopLine(trace.hops, trace.reached !== false));
  return lines.join('\n');
}

/** The link to put in a share, or null for an address nobody else can open. */
export function appLink(location) {
  if (!location || !/^https?:$/.test(location.protocol)) return null;
  if (/^(localhost|127\.|\[?::1\]?$|0\.0\.0\.0)/.test(location.hostname)) return null;
  return `${location.origin}${location.pathname}`;
}

function copyBySelection(text, doc) {
  if (!doc?.body || !doc.execCommand) return false;
  const area = doc.createElement('textarea');
  area.value = text;
  area.setAttribute('readonly', '');
  area.style.position = 'fixed';
  area.style.opacity = '0';
  doc.body.appendChild(area);
  try {
    area.select();
    return doc.execCommand('copy');
  } catch {
    return false;
  } finally {
    area.remove();
  }
}

/**
 * Shares `text` with the Web Share API where the browser has it, else copies it to the clipboard.
 * Resolves with 'shared', 'copied', 'cancelled' (the player closed the share sheet) or 'failed'
 * (nothing could be done: the caller should show the text to copy by hand).
 */
export async function shareTrip({ title, text, url }, { nav = globalThis.navigator, doc = globalThis.document } = {}) {
  const data = { title, text, ...(url ? { url } : {}) };
  if (nav?.share && (!nav.canShare || nav.canShare(data))) {
    try {
      await nav.share(data);
      return 'shared';
    } catch (e) {
      if (e?.name === 'AbortError') return 'cancelled';
      // The browser refused to share (not allowed here, no targets): fall back to the clipboard.
    }
  }
  const full = url ? `${text}\n${url}` : text;
  if (nav?.clipboard?.writeText) {
    try {
      await nav.clipboard.writeText(full);
      return 'copied';
    } catch {
      // Denied or not a secure context: try the old way.
    }
  }
  return copyBySelection(full, doc) ? 'copied' : 'failed';
}
