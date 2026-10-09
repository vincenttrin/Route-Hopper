import { useEffect, useRef } from 'react';
import L from 'leaflet';
import { bunnyMarkup } from './Bunny.jsx';
import { HOP_MS } from '../lib/pace.js';
import { arcPath, arcPoint } from '../lib/arc.js';

const INK = '#2f2a35';
const PAPER = '#fdf6ea';
const DEST = '#f28a1f';
const SELECTED = '#d6336c';

const BUNNY_SIZE = 44;
// Static markup built from our own artwork, never from trace data.
const BUNNY_ICON = L.divIcon({
  className: 'bunny-marker',
  html: `<div class="bunny-map">${bunnyMarkup(BUNNY_SIZE)}</div>`,
  iconSize: [BUNNY_SIZE, BUNNY_SIZE],
  iconAnchor: [BUNNY_SIZE / 2, BUNNY_SIZE + 6],
});

const prefersReducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches;
const ease = (t) => (t < 0.5 ? 2 * t * t : 1 - (-2 * t + 2) ** 2 / 2);

function tooltipFor(h) {
  const root = document.createElement('div');
  const num = document.createElement('b');
  num.textContent = String(h.n).padStart(2, '0');
  root.append(num, ` ${h.city || h.hostname || h.ip}`, document.createElement('br'), `${h.ip} - ${h.rtt != null ? h.rtt.toFixed(1) + ' ms' : 'no rtt'}`);
  return root;
}

function destinationTooltip(d) {
  const root = document.createElement('div');
  const label = document.createElement('b');
  label.textContent = 'Destination';
  root.append(label, ` ${d.city || d.ip}`, document.createElement('br'), `${d.ip} - no reply`);
  return root;
}

/** `destination` is only passed when no probe reached it; it is drawn as unconfirmed. */
export default function HopMap({ hops, destination, selected, onSelect }) {
  const el = useRef(null);
  const map = useRef(null);
  const layer = useRef(null);
  const markers = useRef(new Map());
  // The bunny rides on the newest located hop: its marker, the running glide, and how many hops it has seen.
  const bunny = useRef({ marker: null, frame: 0, count: 0, last: null });
  const onSelectRef = useRef(onSelect);

  useEffect(() => {
    onSelectRef.current = onSelect;
  }, [onSelect]);

  useEffect(() => {
    map.current = L.map(el.current, { zoomControl: true, attributionControl: true, worldCopyJump: true }).setView([30, -20], 2);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
      maxZoom: 18,
      attribution: '&copy; OpenStreetMap contributors | IP geolocation by <a href="https://db-ip.com" target="_blank" rel="noopener noreferrer">DB-IP</a>',
      className: 'tiles-muted',
    }).addTo(map.current);
    layer.current = L.layerGroup().addTo(map.current);
    const resize = new ResizeObserver(() => map.current?.invalidateSize());
    resize.observe(el.current);
    const rider = bunny.current;
    return () => {
      resize.disconnect();
      cancelAnimationFrame(rider.frame);
      rider.marker = null;
      map.current.remove();
      map.current = null;
    };
  }, []);

  useEffect(() => {
    const group = layer.current;
    group.clearLayers();
    markers.current.clear();
    const located = hops.filter((h) => h.located);
    for (let i = 1; i < located.length; i++) {
      const a = located[i - 1];
      const b = located[i];
      L.polyline(arcPath(a, b), { color: b.color, weight: 5, opacity: 0.95, lineCap: 'round' }).addTo(group);
    }
    located.forEach((h) => {
      const style = h.destination
        ? { radius: 8, color: INK, weight: 3, fillColor: DEST, fillOpacity: 1 }
        : { radius: 6, color: INK, weight: 3, fillColor: PAPER, fillOpacity: 1 };
      const m = L.circleMarker([h.lat, h.lng], style).addTo(group);
      m.bindTooltip(tooltipFor(h), { direction: 'top', offset: [0, -6] });
      m.on('click', () => onSelectRef.current(h.n));
      markers.current.set(h.n, m);
    });
    // Fit the arcs too, not just the hops, so the bounce is never cut off at the edge of the map.
    const points = located.map((h) => [h.lat, h.lng]);
    for (let i = 1; i < located.length; i++) points.push(...arcPath(located[i - 1], located[i], 6).map((p) => [p.lat, p.lng]));
    if (destination) {
      const to = [destination.lat, destination.lng];
      if (located.length) {
        const from = located[located.length - 1];
        L.polyline([[from.lat, from.lng], to], { color: INK, weight: 3, opacity: 0.7, dashArray: '2 8', lineCap: 'round' }).addTo(group);
      }
      L.circleMarker(to, { radius: 8, color: INK, weight: 3, dashArray: '3 3', fillColor: DEST, fillOpacity: 0.35 })
        .bindTooltip(destinationTooltip(destination), { direction: 'top', offset: [0, -6] })
        .addTo(group);
      points.push(to);
    }
    if (points.length) {
      map.current.fitBounds(L.latLngBounds(points), { padding: [40, 40], maxZoom: 6 });
    }
  }, [hops, destination]);

  useEffect(() => {
    const rider = bunny.current;
    const last = hops.findLast((h) => h.located);
    const wasCount = rider.count;
    const wasLast = rider.last;
    rider.count = hops.length;
    rider.last = last;
    if (!last) {
      cancelAnimationFrame(rider.frame);
      rider.marker?.remove();
      rider.marker = null;
      return;
    }
    if (rider.marker && wasLast && hops.length === wasCount && wasLast.lat === last.lat && wasLast.lng === last.lng) return;
    const to = L.latLng(last.lat, last.lng);
    if (!rider.marker) {
      rider.marker = L.marker(to, { icon: BUNNY_ICON, interactive: false, keyboard: false, zIndexOffset: 1000 }).addTo(map.current);
      return;
    }
    cancelAnimationFrame(rider.frame);
    const from = rider.marker.getLatLng();
    // Only the next hop of the same trace is a hop; anything else (a new trace, an old one restored) is a jump.
    if (hops.length !== wasCount + 1 || from.equals(to) || prefersReducedMotion()) {
      rider.marker.setLatLng(to);
      return;
    }
    const sprite = rider.marker.getElement()?.firstElementChild;
    if (sprite) {
      sprite.classList.remove('is-leaping');
      void sprite.offsetWidth;
      sprite.classList.add('is-leaping');
    }
    const t0 = performance.now();
    const step = (now) => {
      const t = Math.min(1, (now - t0) / HOP_MS);
      // The same arc the route line is drawn along, so the bunny bounces from hop to hop on it.
      rider.marker.setLatLng(arcPoint(from, to, ease(t)));
      if (t < 1) rider.frame = requestAnimationFrame(step);
    };
    rider.frame = requestAnimationFrame(step);
  }, [hops]);

  useEffect(() => {
    markers.current.forEach((m, n) => {
      const on = n === selected;
      m.setStyle({ weight: on ? 5 : 3, color: on ? SELECTED : INK });
      if (on) {
        m.bringToFront();
        map.current.panTo(m.getLatLng());
      }
    });
  }, [selected, hops]);

  return <div ref={el} className="hop-map" role="application" aria-label="Map of the packet route" />;
}
