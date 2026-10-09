import { useEffect, useRef } from 'react';
import L from 'leaflet';

const INK = '#15181a';
const PAPER = '#f3f4f2';
const DEST = '#f4b400';

function tooltipFor(h) {
  const root = document.createElement('div');
  const num = document.createElement('b');
  num.textContent = String(h.n).padStart(2, '0');
  root.append(num, ` ${h.city || h.hostname || h.ip}`, document.createElement('br'), `${h.ip} - ${h.rtt != null ? h.rtt.toFixed(1) + ' ms' : 'no rtt'}`);
  return root;
}

export default function HopMap({ hops, selected, onSelect }) {
  const el = useRef(null);
  const map = useRef(null);
  const layer = useRef(null);
  const markers = useRef(new Map());
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
    return () => {
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
      L.polyline([[a.lat, a.lng], [b.lat, b.lng]], { color: b.color, weight: 5, opacity: 0.95, lineCap: 'round' }).addTo(group);
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
    if (located.length) {
      map.current.fitBounds(L.latLngBounds(located.map((h) => [h.lat, h.lng])), { padding: [40, 40], maxZoom: 6 });
    }
  }, [hops]);

  useEffect(() => {
    markers.current.forEach((m, n) => {
      const on = n === selected;
      m.setStyle({ weight: on ? 5 : 3, color: on ? '#d83b2a' : INK });
      if (on) {
        m.bringToFront();
        map.current.panTo(m.getLatLng());
      }
    });
  }, [selected, hops]);

  return <div ref={el} className="hop-map" role="application" aria-label="Map of the packet route" />;
}
