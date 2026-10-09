import { UNITS, formatDistance, missLabel, scoreMessage } from '../lib/game.js';

const UNSCOREABLE = {
  'few-hops': 'Fewer than two hops on this route could be placed on the map, so there is no distance to guess. The bunny shrugs.',
  'no-distance': 'The route stayed in one spot, so there is no distance to guess. The bunny shrugs.',
};

/**
 * The game's side of the screen. `status` is one of: ready (no round yet), running (the bunny is
 * hopping along the route), guessing (the route is revealed and the player guesses its distance),
 * scored, unscoreable (the route has no distance to guess) or incomplete (the round ended before the
 * trace finished, so nothing is scored).
 */
export default function GamePanel({ status, unit, guess, guessError, guessRef, result, best, onGuess, onUnit, onSubmitGuess, onPlayAgain }) {
  const submit = (e) => {
    e.preventDefault();
    onSubmitGuess();
  };
  return (
    <section className={`game game-${status}`} aria-label="Distance guessing game" aria-live="polite">
      <h2>Guess the hop distance</h2>
      {status === 'ready' && (
        <p>
          Press <b>Hop!</b> and watch the bunny follow the packet's route. When it gets home, guess how far the packet travelled and
          get a score from 0 to 100. Within 10 km of the real distance is a perfect 100.
        </p>
      )}
      {status === 'running' && <p>The bunny is on the trail. Watch the route; you guess the total distance when it gets home.</p>}
      {status === 'guessing' && (
        <form className="guess-form" onSubmit={submit} noValidate>
          <p>The bunny is home! How far did the packet travel along the whole route?</p>
          <div className={`field-guess${guessError ? ' is-invalid' : ''}`}>
            <label htmlFor="guess" className="sr-only">
              Your guess for the distance the packet travelled
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
              aria-invalid={Boolean(guessError)}
              aria-describedby={guessError ? 'guess-error' : undefined}
            />
            <div className="units" role="group" aria-label="Distance unit">
              {Object.entries(UNITS).map(([key, u]) => (
                <button key={key} type="button" aria-pressed={unit === key} aria-label={u.long} onClick={() => onUnit(key)}>
                  {u.label}
                </button>
              ))}
            </div>
            <button type="submit" className="go">
              Guess
            </button>
          </div>
          {guessError && (
            <p id="guess-error" className="form-error" role="alert">
              {guessError}
            </p>
          )}
        </form>
      )}
      {status === 'scored' && (
        <>
          <div className="score">
            <b>{result.score}</b>
            <span>/ 100</span>
            <i className="meter" role="img" aria-label={`Score ${result.score} out of 100`}>
              <i style={{ width: `${result.score}%` }} />
            </i>
          </div>
          <p className="game-message">{scoreMessage(result.score)}</p>
          <dl className="game-figures">
            <div>
              <dt>Your guess</dt>
              <dd>{formatDistance(result.guessKm, unit)}</dd>
            </div>
            <div>
              <dt>Actual</dt>
              <dd>{formatDistance(result.actualKm, unit)}</dd>
            </div>
          </dl>
          <p className="game-miss">{missLabel(result.guessKm, result.actualKm, unit) ?? 'Right on the carrot!'}</p>
        </>
      )}
      {status === 'unscoreable' && <p>{UNSCOREABLE[result.reason]}</p>}
      {status === 'incomplete' && (
        <p>The trace did not finish, so this round has no score. The bunny will try again whenever you are ready.</p>
      )}
      {best != null && (
        <p className="game-best">
          Session best <b>{best}</b>
        </p>
      )}
      {(status === 'scored' || status === 'unscoreable' || status === 'incomplete') && (
        <button type="button" className="again" onClick={onPlayAgain}>
          Play again
        </button>
      )}
    </section>
  );
}
