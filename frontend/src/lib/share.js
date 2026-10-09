// The share summary: a friendly, bunny-themed text about a finished trip that can be sent to a friend.
// It says where the packet went and how far, never where it started: the server withholds the route's
// start (see backend/handlers/redact.go) and nothing here reads hop addresses or host names.
import { formatDistance, MIN_SCOREABLE_KM } from './game.js';

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

function countryName(code) {
  try {
    return new Intl.DisplayNames(['en'], { type: 'region' }).of(code) || code;
  } catch {
    return code;
  }
}

function list(items) {
  try {
    return new Intl.ListFormat('en', { style: 'long', type: 'conjunction' }).format(items);
  } catch {
    return items.join(', ');
  }
}

const plural = (n, one, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

/** Countries the route passed through, by name, in the order it reached them. */
export function countriesOf(hops) {
  const codes = [];
  for (const h of hops) {
    if (h.located && h.country && !codes.includes(h.country)) codes.push(h.country);
  }
  return codes.map(countryName);
}

/**
 * The text to share for a finished trip. `trace` and `stats` are the normalized trace and its summary;
 * `result` is the judged round (null when the player did not guess). Only coarse facts are used: the
 * destination the player typed, hop and network counts, the total distance, the countries passed
 * through, and the guess and score.
 */
export function tripSummary({ endpoint, trace, stats, result, unit = 'km' }) {
  const target = trace.hops.find((h) => h.destination);
  const city = trace.destination?.city || target?.city;
  const country = target?.country ? countryName(target.country) : null;
  const place = [city, country].filter(Boolean).join(', ');
  const lines = [`🐰 I followed a packet to ${endpointLabel(endpoint)}${place ? ` (${place})` : ''}!`];

  let trip = `It hopped ${plural(stats.hops, 'time')}`;
  if (stats.located >= 2 && stats.exactKm >= MIN_SCOREABLE_KM) trip += ` and travelled about ${formatDistance(stats.exactKm, unit)}`;
  if (stats.networks > 0) trip += ` across ${plural(stats.networks, 'network')}`;
  const countries = countriesOf(trace.hops);
  if (countries.length) trip += `, passing through ${list(countries)}`;
  lines.push(`${trip}.`);

  if (result?.status === 'scored') {
    lines.push(`I guessed ${formatDistance(result.guessKm, unit)} and scored ${result.score}/100. Can you beat my bunny score? 🥕`);
  } else {
    lines.push('Think you can guess how far it went? 🥕');
  }
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
