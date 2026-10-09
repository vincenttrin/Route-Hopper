// The guessing game: once the bunny has shown the route, the player guesses its distance and the guess is scored.

export const KM_PER_MILE = 1.609344;

/** A guess further than this many km is rejected as a typo; a route would have to circle the Earth over a dozen times. */
export const MAX_GUESS_KM = 500000;

/** A guess within this many km of the actual distance (over or under) scores a full 100, whatever unit it was typed in. */
export const TOLERANCE_KM = 10;

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
 * Scores a guess from 0 to 100 by how close it is in proportion to the actual distance. A guess within
 * TOLERANCE_KM of the actual distance scores 100. Otherwise:
 *
 *   score = round(100 * min(guess, actual) / max(guess, actual))
 *
 * So 10000 km against an actual 13146 km scores 76, and 13146 km against an actual 10000 km scores 76 too:
 * over- and under-shooting are treated symmetrically. The score stays within 0 to 100. Both distances are in km.
 * Returns null when the inputs cannot be scored: a guess that is not a positive number, or a
 * route that went nowhere (see MIN_SCOREABLE_KM), so the ratio never divides by zero.
 */
export function scoreGuess(guessKm, actualKm) {
  if (!Number.isFinite(guessKm) || guessKm <= 0) return null;
  if (!Number.isFinite(actualKm) || actualKm < MIN_SCOREABLE_KM) return null;
  if (Math.abs(guessKm - actualKm) <= TOLERANCE_KM) return 100;
  return Math.round((100 * Math.min(guessKm, actualKm)) / Math.max(guessKm, actualKm));
}

/**
 * Why a finished trace cannot be played, or null when it can. `locatedCount` is how many hops had a
 * location: the route needs two to have a distance. A route that stayed in one spot has none either.
 */
export function unscoreableReason(actualKm, locatedCount) {
  if (locatedCount < 2) return 'few-hops';
  if (!Number.isFinite(actualKm) || actualKm < MIN_SCOREABLE_KM) return 'no-distance';
  return null;
}

/**
 * Judges a finished round once the player has guessed. Returns `{ status: 'scored', score, guessKm, actualKm }`
 * or `{ status: 'unscoreable', reason: 'few-hops' | 'no-distance' }`.
 */
export function judgeRound(guessKm, actualKm, locatedCount) {
  const reason = unscoreableReason(actualKm, locatedCount);
  const score = reason ? null : scoreGuess(guessKm, actualKm);
  if (score === null) return { status: 'unscoreable', reason: reason ?? 'no-distance' };
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
