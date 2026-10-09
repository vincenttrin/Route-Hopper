import { useEffect, useRef, useState } from 'react';
import { appLink, shareTrip } from '../lib/share.js';

const NOTES = {
  shared: 'Shared! The bunny is proud.',
  copied: 'Copied to your clipboard. Paste it to a friend!',
};

/** Shares `text` (see lib/share.js): the share sheet where the browser has one, else the clipboard. */
export default function ShareButton({ text }) {
  const [outcome, setOutcome] = useState(null);
  const area = useRef(null);

  // A new round has new words to share.
  useEffect(() => setOutcome(null), [text]);
  useEffect(() => {
    if (outcome === 'failed') area.current?.select();
  }, [outcome]);

  const share = async () => {
    const result = await shareTrip({ title: 'Route Hopper', text, url: appLink(window.location) });
    setOutcome(result === 'cancelled' ? null : result);
  };

  return (
    <div className="share">
      <button type="button" className="share-btn" onClick={share}>
        Share trip
      </button>
      {NOTES[outcome] && (
        <p className="share-note" role="status">
          {NOTES[outcome]}
        </p>
      )}
      {outcome === 'failed' && (
        <>
          <p className="share-note" role="status">
            Could not copy automatically. Copy the text below and send it to a friend.
          </p>
          <textarea ref={area} className="share-text" readOnly rows={4} value={text} aria-label="Trip summary to share" />
        </>
      )}
    </div>
  );
}
