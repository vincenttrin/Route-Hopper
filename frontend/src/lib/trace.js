const PRIVATE_IP = /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|127\.|169\.254\.)/;
export const LOCAL = 'Local network';

export const LOCAL_COLOR = '#7a6f6a';
// Every colour reads at 4.5:1 or better under white chip text. Orange is left out: it marks the destination.
const PALETTE = ['#b0306a', '#1f6fb5', '#277a4c', '#7a4fb5', '#0f7f82', '#8a5a2b'];

function networkOf(hop) {
  if (!hop.ip) return null;
  if (PRIVATE_IP.test(hop.ip)) return LOCAL;
  if (hop.org) return hop.org;
  const parts = (hop.hostname || '').split('.');
  if (parts.length >= 2) return parts.slice(-2).join('.');
  return 'Unknown network';
}

const clean = (v) => (v && v !== '*' ? v : null);
const hasGeo = (h) => Number.isFinite(h.lat) && Number.isFinite(h.lng) && !(h.lat === 0 && h.lng === 0);

/**
 * Turns the API response into display-ready hops with network, colour and lane.
 * `reached` is false when the destination never answered; the silent hops at the
 * end of such a trace are folded into one row (`span` counts the hops it covers).
 * With `partial` the trace is unfinished (still running, cancelled or cut off): nothing is folded, since more hops may
 * follow, and a hop only counts as the destination if it has that address.
 */
export function normalizeTrace(raw, { partial = false } = {}) {
  const colors = new Map();
  let lane = 0;
  let prevNet = null;
  const hops = raw.hops.map((h, i) => {
    const ip = clean(h.ip);
    const hop = {
      n: h.hopNumber ?? i + 1,
      ip,
      hostname: clean(h.hostname),
      city: clean(h.city),
      country: clean(h.country),
      org: h.org,
      lat: h.lat,
      lng: h.lng,
      rtt: ip && Number.isFinite(h.rtt) ? h.rtt : null,
      span: 1,
    };
    hop.located = Boolean(ip) && hasGeo(hop);
    hop.network = networkOf(hop);
    if (hop.network) {
      if (prevNet && hop.network !== prevNet) lane = 1 - lane;
      hop.change = prevNet === null || hop.network !== prevNet;
      prevNet = hop.network;
      if (!colors.has(hop.network)) {
        colors.set(hop.network, hop.network === LOCAL ? LOCAL_COLOR : PALETTE[colors.size % PALETTE.length]);
      }
    }
    hop.lane = lane;
    hop.color = hop.network ? colors.get(hop.network) : null;
    return hop;
  });
  // A no-reply hop borrows the lane of the hop before it, which is already set above.
  const d = raw.destination;
  let tail = hops.length;
  while (tail > 0 && !hops[tail - 1].ip) tail--;
  const lastReply = tail > 0 ? hops[tail - 1].n : null;
  let reached;
  if (partial) {
    const dest = d?.ip && hops.find((h) => h.ip === d.ip);
    if (dest) dest.destination = true;
    reached = Boolean(dest);
  } else {
    reached = raw.reached !== false;
    if (hops.length - tail > 1) {
      hops[tail].span = hops.length - tail;
      hops.length = tail + 1;
    }
    const last = hops[hops.length - 1];
    if (last && reached) last.destination = true;
  }
  return {
    hops,
    networks: [...colors].map(([name, color]) => ({ name, color })),
    reached,
    lastReply,
    destination: d?.ip ? { ip: d.ip, city: clean(d.city), lat: d.lat, lng: d.lng, located: hasGeo(d) } : null,
  };
}

const rad = (d) => (d * Math.PI) / 180;

export function distanceKm(a, b) {
  const s =
    Math.sin(rad(b.lat - a.lat) / 2) ** 2 +
    Math.cos(rad(a.lat)) * Math.cos(rad(b.lat)) * Math.sin(rad(b.lng - a.lng) / 2) ** 2;
  return 6371 * 2 * Math.asin(Math.sqrt(s));
}

/** Total great-circle distance, in km, along the route: hops without a location are skipped, the rest are joined in order. */
export function routeDistanceKm(hops) {
  const located = hops.filter((h) => h.located);
  let km = 0;
  for (let i = 1; i < located.length; i++) km += distanceKm(located[i - 1], located[i]);
  return km;
}

export function summarize(hops, networks) {
  const km = routeDistanceKm(hops);
  const last = hops[hops.length - 1];
  return {
    hops: hops.reduce((n, h) => n + h.span, 0),
    networks: networks.filter((n) => n.name !== LOCAL).length,
    km: Math.round(km),
    exactKm: km,
    located: hops.filter((h) => h.located).length,
    // Only the destination's own reply is a latency to it.
    rtt: last?.destination ? last.rtt : null,
  };
}

export function placeLabel(hop) {
  return hop.city || hop.hostname || hop.ip || 'No reply';
}
