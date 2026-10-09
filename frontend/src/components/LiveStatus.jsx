import { useEffect, useState } from 'react';

/**
 * Progress line for a trace that is still running: the bunny's next stop and the time spent.
 * `found` hops have been discovered, `shown` of them are on screen; the rest are waiting their turn.
 */
export default function LiveStatus({ startedAt, found, shown, probing, phase }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 250);
    return () => clearInterval(id);
  }, []);
  const seconds = Math.max(0, (now - startedAt) / 1000);
  const waiting = found - shown;
  let what;
  if (phase === 'icmp') what = 'The trail went cold, so the bunny is sniffing again with ICMP probes';
  else if (probing) what = `The bunny is sniffing out hop ${found + 1}`;
  else what = 'The trail is found, the bunny is hopping home';
  const queue = waiting > 0 ? `, ${waiting} more ${waiting === 1 ? 'hop' : 'hops'} waiting` : '';
  return (
    <div className="notice notice-info" role="status" aria-live="off">
      {what} - {shown} {shown === 1 ? 'hop' : 'hops'} hopped{queue} - {seconds.toFixed(1)} s
    </div>
  );
}
