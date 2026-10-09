import { useEffect, useState } from 'react';

/** Progress line for a trace that is still running: the hop being probed and the time spent. */
export default function LiveStatus({ startedAt, hops, phase }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 250);
    return () => clearInterval(id);
  }, []);
  const seconds = Math.max(0, (now - startedAt) / 1000);
  const next = (hops[hops.length - 1]?.n ?? 0) + 1;
  const what = phase === 'icmp' ? 'Destination did not answer, retrying with ICMP probes' : `Probing hop ${next}`;
  return (
    <div className="notice notice-info" role="status" aria-live="off">
      Tracing... {what} - {hops.length} {hops.length === 1 ? 'hop' : 'hops'} found - {seconds.toFixed(1)} s
    </div>
  );
}
