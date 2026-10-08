const PRIVATE_IP = /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|127\.|169\.254\.)/;
const LOCAL = 'Local network';

export const LOCAL_COLOR = '#6d7479';
const PALETTE = ['#d83b2a', '#1769c2', '#1f9a4b', '#7b3fb0', '#e07a00', '#0f8b8d'];

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

/** Turns the API response into display-ready hops with network, colour and lane. */
export function normalizeTrace(raw) {
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
      org: h.org,
      lat: h.lat,
      lng: h.lng,
      rtt: ip && Number.isFinite(h.rtt) ? h.rtt : null,
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
  const last = hops[hops.length - 1];
  if (last) last.destination = true;
  return { hops, networks: [...colors].map(([name, color]) => ({ name, color })) };
}

const rad = (d) => (d * Math.PI) / 180;

export function distanceKm(a, b) {
  const s =
    Math.sin(rad(b.lat - a.lat) / 2) ** 2 +
    Math.cos(rad(a.lat)) * Math.cos(rad(b.lat)) * Math.sin(rad(b.lng - a.lng) / 2) ** 2;
  return 6371 * 2 * Math.asin(Math.sqrt(s));
}

export function summarize(hops, networks) {
  const located = hops.filter((h) => h.located);
  let km = 0;
  for (let i = 1; i < located.length; i++) km += distanceKm(located[i - 1], located[i]);
  const last = [...hops].reverse().find((h) => h.rtt != null);
  return {
    hops: hops.length,
    networks: networks.filter((n) => n.name !== LOCAL).length,
    km: Math.round(km),
    rtt: last ? last.rtt : null,
  };
}

export function placeLabel(hop) {
  return hop.city || hop.hostname || hop.ip || 'No reply';
}
