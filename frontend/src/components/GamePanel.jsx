import { formatDistance, missLabel, scoreMessage } from '../lib/game.js';

const UNSCOREABLE = {
  'few-hops': 'Fewer than two hops on this route could be placed on the map, so there is no distance to score. The bunny shrugs.',
  'no-distance': 'The route stayed in one spot, so there is no distance to score. The bunny shrugs.',
};

/**
 * The game's side of the screen. `status` is one of: ready (no round yet), running (guess locked in,
 * trace under way), scored, unscoreable (the route has no distance to compare with) or incomplete
 * (the round ended before the trace finished, so nothing is scored).
 */
export default function GamePanel({ status, unit, round, result, best, onPlayAgain }) {
  return (
    <section className={`game game-${status}`} aria-label="Distance guessing game" aria-live="polite">
      <h2>Guess the hop distance</h2>
      {status === 'ready' && (
        <p>
          How far will the packet travel? Guess the total distance along its route, then press <b>Hop!</b> The bunny follows the trail
          and scores your guess from 0 to 100.
        </p>
      )}
      {status === 'running' && (
        <>
          <p className="game-guess">
            Your guess <b>{formatDistance(round.guessKm, unit)}</b>
          </p>
          <p>The bunny is on the trail. The real distance stays secret until it gets home.</p>
        </>
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
