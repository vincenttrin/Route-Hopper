// The guessing game: parse a distance guess, score it against the traced route, and word the result.

export const KM_PER_MILE = 1.609344;

/** A guess further than this many km is rejected as a typo; a route would have to circle the Earth over a dozen times. */
export const MAX_GUESS_KM = 500000;

/**
 * Relative error at which the score reaches 0: a guess that is off by this share of the actual
 * distance (or more) scores nothing. 1 means "off by the whole distance", so guessing 0 or double
 * the actual distance scores 0.
 */
export const ZERO_SCORE_ERROR = 1;

/** Below this many km the route did not go anywhere, so there is no distance to compare a guess with. */
export const MIN_SCOREABLE_KM = 1;

export const UNITS = {
  km: { label: 'km', long: 'kilometres', perKm: 1 },
  mi: { label: 'mi', long: 'miles', perKm: 1 / KM_PER_MILE },
};

export const toKm = (value, unit) => value / UNITS[unit].perKm;
export const fromKm = (km, unit) => km * UNITS[unit].perKm;

export function formatDistance(km, unit) {
  return `${Math.round(fromKm(km, unit)).toLocaleString('en-US')} ${UNITS[unit].label}`;
}

/**
 * Reads the guess a player typed. Digits with optional thousands commas and a decimal part are
 * accepted ("8000", "8,000", "7.5"). Returns `{ ok: true, km }` (always in km, whatever the unit
 * typed) or `{ ok: false, error }` with a message that can be shown as is.
 */
export function parseGuess(text, unit = 'km') {
  const raw = String(text ?? '').trim();
  if (!raw) return { ok: false, error: 'Guess a distance first.' };
  if (!UNITS[unit]) return { ok: false, error: 'Pick km or miles.' };
  const cleaned = raw.replace(/,/g, '');
  if (!/^(\d+\.?\d*|\.\d+)$/.test(cleaned)) return { ok: false, error: 'Use digits only, like 8000.' };
  const value = Number(cleaned);
  if (!Number.isFinite(value) || value <= 0) return { ok: false, error: 'The guess has to be more than 0.' };
  const km = toKm(value, unit);
  if (km > MAX_GUESS_KM) return { ok: false, error: `That is too far to be a route. Stay under ${formatDistance(MAX_GUESS_KM, unit)}.` };
  return { ok: true, km };
}

/**
 * Scores a guess from 0 to 100 by its relative error `e = |guess - actual| / actual`:
 *
 *   score = 100 * (1 - e / ZERO_SCORE_ERROR) ^ 2, rounded, and 0 once e reaches ZERO_SCORE_ERROR
 *
 * An exact guess scores 100. The score falls smoothly (no cliff) and is gentlest at first: 5% off
 * is 90, 10% off is 81, 25% off is 56, 50% off is 25, and 100% off or more is 0. Over- and
 * under-shooting by the same distance score the same. Both distances are in the same unit (km).
 * Returns null when the inputs cannot be scored: a guess that is not a positive number, or a
 * route that went nowhere (see MIN_SCOREABLE_KM).
 */
export function scoreGuess(guessKm, actualKm) {
  if (!Number.isFinite(guessKm) || guessKm <= 0) return null;
  if (!Number.isFinite(actualKm) || actualKm < MIN_SCOREABLE_KM) return null;
  const error = Math.abs(guessKm - actualKm) / actualKm;
  if (error >= ZERO_SCORE_ERROR) return 0;
  return Math.round(100 * (1 - error / ZERO_SCORE_ERROR) ** 2);
}

/**
 * Judges a finished round. `locatedCount` is how many hops had a location: the route needs two to
 * have a distance. Returns `{ status: 'scored', score, guessKm, actualKm }` or
 * `{ status: 'unscoreable', reason: 'few-hops' | 'no-distance' }`.
 */
export function judgeRound(guessKm, actualKm, locatedCount) {
  if (locatedCount < 2) return { status: 'unscoreable', reason: 'few-hops' };
  const score = scoreGuess(guessKm, actualKm);
  if (score === null) return { status: 'unscoreable', reason: 'no-distance' };
  return { status: 'scored', score, guessKm, actualKm };
}

/** A line for the result panel. */
export function scoreMessage(score) {
  if (score >= 95) return 'Perfect hop! The bunny bows to you.';
  if (score >= 80) return 'Hoppy days! Nearly spot on.';
  if (score >= 55) return 'Nice bounding! Only a few burrows off.';
  if (score >= 25) return 'Not bad for a bunny. The trail was a fair bit different.';
  if (score > 0) return 'Whoops, a wild leap! The bunny is dizzy.';
  return 'Lost in the clover. Way off this time.';
}

/** Which way the guess missed, in words, or null when it is within a rounding error of the actual distance. */
export function missLabel(guessKm, actualKm, unit) {
  const diff = guessKm - actualKm;
  if (Math.round(Math.abs(fromKm(diff, unit))) === 0) return null;
  return `${formatDistance(Math.abs(diff), unit)} too ${diff > 0 ? 'far' : 'short'}`;
}
