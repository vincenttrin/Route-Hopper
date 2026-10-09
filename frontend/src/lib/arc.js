// The bunny's hop path: an arc between two hops, bulging up the map like a bouncing ball. Drawn only for
// show; distances are always measured along the straight great-circle legs (see routeDistanceKm).

const R = 6378137;
const MAX_LAT = 85.0511;
const rad = (d) => (d * Math.PI) / 180;
const deg = (r) => (r * 180) / Math.PI;

// Web Mercator, the projection of the map tiles, so "up" on the screen is "up" for the arc too.
const project = ({ lat, lng }) => ({ x: R * rad(lng), y: R * Math.log(Math.tan(Math.PI / 4 + rad(Math.min(MAX_LAT, Math.max(-MAX_LAT, lat))) / 2)) });
const unproject = ({ x, y }) => ({ lat: deg(2 * Math.atan(Math.exp(y / R)) - Math.PI / 2), lng: deg(x / R) });

/** How high the arc rises at its middle, as a share of the straight distance between the hops. */
export const ARC_HEIGHT = 0.25;

/**
 * The point a share `t` (0 to 1) of the way along the arc from `a` to `b`, both `{ lat, lng }`. t = 0 is
 * exactly `a` and t = 1 exactly `b`; in between the path rises to ARC_HEIGHT of the distance and comes back down.
 * The arc bulges to the screen-up side of the straight line (to the right of it for a straight north-south hop).
 */
export function arcPoint(a, b, t) {
  if (t <= 0) return { lat: a.lat, lng: a.lng };
  if (t >= 1) return { lat: b.lat, lng: b.lng };
  const p = project(a);
  const q = project(b);
  const dx = q.x - p.x;
  const dy = q.y - p.y;
  const len = Math.hypot(dx, dy);
  if (len === 0) return { lat: a.lat, lng: a.lng };
  let nx = -dy / len;
  let ny = dx / len;
  if (ny < 0 || (ny === 0 && nx < 0)) {
    nx = -nx;
    ny = -ny;
  }
  const lift = len * ARC_HEIGHT * 4 * t * (1 - t);
  return unproject({ x: p.x + dx * t + nx * lift, y: p.y + dy * t + ny * lift });
}

/** The arc from `a` to `b` as `steps + 1` points, for drawing a line. */
export function arcPath(a, b, steps = 24) {
  return Array.from({ length: steps + 1 }, (_, i) => arcPoint(a, b, i / steps));
}
