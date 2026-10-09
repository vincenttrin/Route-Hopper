import { useState } from 'react';
import { UNITS } from '../lib/game.js';

/**
 * The endpoint to trace plus the player's guess for how far the packet will travel. The guess is
 * owned by the parent, which checks it when the form is submitted and passes `guessError` back.
 */
export default function TraceInput({ initial, busy, guess, unit, guessError, guessRef, onGuess, onUnit, onSubmit, onCancel }) {
  const [value, setValue] = useState(initial);
  const submit = (e) => {
    e.preventDefault();
    const v = value.trim();
    if (v && !busy) onSubmit(v);
  };
  return (
    <form className="trace-form" onSubmit={submit} noValidate>
      <label htmlFor="endpoint" className="sr-only">
        Endpoint
      </label>
      <input
        id="endpoint"
        className="field-endpoint"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        placeholder="example.com or 93.184.216.34"
        autoComplete="off"
        spellCheck="false"
      />
      <div className={`field-guess${guessError ? ' is-invalid' : ''}`}>
        <label htmlFor="guess" className="sr-only">
          Your guess for the distance the packet travels
        </label>
        <input
          id="guess"
          ref={guessRef}
          value={guess}
          onChange={(e) => onGuess(e.target.value)}
          placeholder="Your guess"
          inputMode="decimal"
          autoComplete="off"
          spellCheck="false"
          readOnly={busy}
          aria-invalid={Boolean(guessError)}
          aria-describedby={guessError ? 'guess-error' : undefined}
        />
        <div className="units" role="group" aria-label="Distance unit">
          {Object.entries(UNITS).map(([key, u]) => (
            <button key={key} type="button" aria-pressed={unit === key} aria-label={u.long} disabled={busy} onClick={() => onUnit(key)}>
              {u.label}
            </button>
          ))}
        </div>
      </div>
      {/* Distinct keys: React must remount rather than flip this one node from button to submit mid-click, or the click that cancels also submits the form. */}
      {busy ? (
        <button key="cancel" type="button" className="go" onClick={onCancel}>
          Cancel
        </button>
      ) : (
        <button key="trace" type="submit" className="go">Hop!</button>
      )}
      {guessError && (
        <p id="guess-error" className="form-error" role="alert">
          {guessError}
        </p>
      )}
    </form>
  );
}
