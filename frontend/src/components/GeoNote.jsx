import { useEffect, useState } from 'react';
import { getGeoInfo } from '../services/api.js';
import { formatBuildDate } from '../lib/geoinfo.js';

// While the backend has no database (first start, still downloading) ask again now and then.
const RECHECK_MS = 30000;

/** Footer note about the location database: whose it is, and when it was last updated. Also carries the DB-IP attribution. */
export default function GeoNote() {
  const [info, setInfo] = useState(null);
  const available = info?.available;

  useEffect(() => {
    const ctl = new AbortController();
    const load = () => getGeoInfo(ctl.signal).then((next) => !ctl.signal.aborted && setInfo(next));
    load();
    if (available) return () => ctl.abort();
    const id = setInterval(load, RECHECK_MS);
    return () => {
      clearInterval(id);
      ctl.abort();
    };
  }, [available]);

  if (!info) return null;
  if (!info.available) {
    return (
      <footer className="geo-note" role="status">
        The location database is not loaded yet, so hops cannot be placed on the map. The server downloads it by itself; try again in a few minutes.
      </footer>
    );
  }
  const updated = formatBuildDate(info.updated);
  const dbip = info.provider?.startsWith('DB-IP');
  return (
    <footer className="geo-note">
      IP geolocation by{' '}
      {dbip ? (
        <a href="https://db-ip.com" target="_blank" rel="noopener noreferrer">
          DB-IP
        </a>
      ) : (
        info.provider || 'a GeoIP database'
      )}
      {dbip ? ' (Lite database)' : ''}
      {updated ? `, database last updated ${updated}` : ''}.
    </footer>
  );
}
