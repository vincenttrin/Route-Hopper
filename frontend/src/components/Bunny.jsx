// The mascot and the carrot at the end of the route. Drawn with the app's ink and paper colours; no outside assets.

const INK = '#2f2a35';
const FUR = '#fffaf0';
const PINK = '#f4a6b8';
const CARROT = '#f28a1f';
const LEAF = '#3d9a58';

const LINE = { stroke: INK, strokeWidth: 2, strokeLinejoin: 'round', strokeLinecap: 'round' };

// The bunny facing right, in a 48 x 48 box: ears up, body low, ready to jump. As data so it can
// be drawn by React and also written out as plain markup for the Leaflet map marker.
const BUNNY = [
  ['ellipse', { cx: 29, cy: 9.5, rx: 3, ry: 8.5, fill: FUR, transform: 'rotate(-12 29 9.5)', ...LINE }],
  ['ellipse', { cx: 29, cy: 10, rx: 1.2, ry: 5.5, fill: PINK, transform: 'rotate(-12 29 9.5)' }],
  ['ellipse', { cx: 22, cy: 35, rx: 15, ry: 9, fill: FUR, ...LINE }],
  ['circle', { cx: 6.5, cy: 33, r: 4.5, fill: FUR, ...LINE }],
  ['ellipse', { cx: 16, cy: 42, rx: 7, ry: 3.6, fill: FUR, ...LINE }],
  ['circle', { cx: 34, cy: 26, r: 9, fill: FUR, ...LINE }],
  ['ellipse', { cx: 37, cy: 9, rx: 3, ry: 8.5, fill: FUR, transform: 'rotate(8 37 9)', ...LINE }],
  ['ellipse', { cx: 37, cy: 9.5, rx: 1.2, ry: 5.5, fill: PINK, transform: 'rotate(8 37 9)' }],
  ['ellipse', { cx: 33, cy: 42.5, rx: 4.4, ry: 2.8, fill: FUR, ...LINE }],
  ['circle', { cx: 37.5, cy: 24.5, r: 1.7, fill: INK }],
  ['circle', { cx: 43, cy: 28, r: 1.7, fill: PINK, stroke: INK, strokeWidth: 1 }],
  ['ellipse', { cx: 33, cy: 30.5, rx: 2.6, ry: 1.6, fill: PINK, opacity: 0.7 }],
];

export function BunnyArt() {
  return (
    <>
      {BUNNY.map(([Tag, attrs], i) => (
        <Tag key={i} {...attrs} />
      ))}
    </>
  );
}

/** The bunny as an SVG string, for places that take markup instead of elements. Built from the constants above only. */
export function bunnyMarkup(size) {
  const kebab = (k) => k.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`);
  const shapes = BUNNY.map(([tag, attrs]) => `<${tag} ${Object.entries(attrs).map(([k, v]) => `${kebab(k)}="${v}"`).join(' ')}/>`);
  return `<svg viewBox="0 0 48 48" width="${size}" height="${size}" aria-hidden="true" focusable="false">${shapes.join('')}</svg>`;
}

export default function Bunny({ size = 40, className }) {
  return (
    <svg className={className} viewBox="0 0 48 48" width={size} height={size} aria-hidden="true" focusable="false">
      <BunnyArt />
    </svg>
  );
}

/** A carrot in a 24 x 32 box: where the route ends. */
export function CarrotArt() {
  const line = { stroke: INK, strokeWidth: 1.8, strokeLinejoin: 'round', strokeLinecap: 'round' };
  return (
    <>
      <path d="M12 9 C8 5 6 3 5 1 C9 1 11 4 12 8 C12 4 13 1 14 0 C16 3 14 6 12.5 9 C15 6 18 4 20 4 C19 7 16 9 13 10 Z" fill={LEAF} {...line} strokeWidth="1.4" />
      <path d="M12 9 C17 9 19 13 17 18 L12.8 30.5 C12.5 31.3 11.5 31.3 11.2 30.5 L7 18 C5 13 7 9 12 9 Z" fill={CARROT} {...line} />
      <path d="M9 15 H12 M13 20 H16 M10 24 H12.5" stroke={INK} strokeWidth="1.3" strokeLinecap="round" fill="none" opacity=".55" />
    </>
  );
}

export function Carrot({ size = 24, className }) {
  return (
    <svg className={className} viewBox="0 0 24 32" width={size} height={(size * 32) / 24} aria-hidden="true" focusable="false">
      <CarrotArt />
    </svg>
  );
}
