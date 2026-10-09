import { describe, expect, it } from 'vitest';
import {
  KM_PER_MILE,
  MAX_GUESS_KM,
  toKm,
  MIN_SCOREABLE_KM,
  formatDistance,
  judgeRound,
  missLabel,
  parseGuess,
  scoreGuess,
  scoreMessage,
  unscoreableReason,
} from './game.js';

describe('scoreGuess', () => {
  it('gives 100 for an exact guess', () => {
    expect(scoreGuess(8000, 8000)).toBe(100);
  });

  it('gives 100 within 10 km either side, and exactly at the 10 km boundary', () => {
    expect(scoreGuess(1005, 1000)).toBe(100);
    expect(scoreGuess(995, 1000)).toBe(100);
    expect(scoreGuess(1010, 1000)).toBe(100);
    expect(scoreGuess(990, 1000)).toBe(100);
  });

  it('drops continuously just past the 10 km boundary', () => {
    expect(scoreGuess(1011, 1000)).toBe(100); // 0.1% past rounds to 100
    expect(scoreGuess(1015, 1000)).toBe(99);
    expect(scoreGuess(985, 1000)).toBe(99);
    expect(scoreGuess(1020, 1000)).toBe(98);
  });

  it('follows the documented curve past the tolerance: 100 * (1 - error)^2', () => {
    expect(scoreGuess(1060, 1000)).toBe(90); // 5% past the tolerance
    expect(scoreGuess(890, 1000)).toBe(81); // 10% past, under
    expect(scoreGuess(1260, 1000)).toBe(56); // 25% past
    expect(scoreGuess(490, 1000)).toBe(25); // 50% past, under
  });

  it('applies the tolerance in km, not in the display unit', () => {
    const actualKm = 1000;
    expect(scoreGuess(toKm(6.2, 'mi') + actualKm, actualKm)).toBe(100); // about 10 km
  });

  it('scores over- and under-shooting by the same distance alike', () => {
    expect(scoreGuess(1300, 1000)).toBe(scoreGuess(700, 1000));
  });

  it('is 0 at the error threshold and beyond it', () => {
    expect(scoreGuess(2010, 1000)).toBe(0);
    expect(scoreGuess(5000, 1000)).toBe(0);
    expect(scoreGuess(1, 1000)).toBe(0);
  });

  it('is never negative, even for a huge error', () => {
    expect(scoreGuess(MAX_GUESS_KM, 1000)).toBe(0);
    expect(scoreGuess(1e12, 1.5)).toBe(0);
    expect(scoreGuess(1, 400000)).toBe(0);
  });

  it('never rises as the guess moves away from the actual distance', () => {
    let last = Infinity;
    for (let guess = 1000; guess <= 2200; guess += 10) {
      const score = scoreGuess(guess, 1000);
      expect(score).toBeLessThanOrEqual(last);
      expect(score).toBeGreaterThanOrEqual(0);
      expect(score).toBeLessThanOrEqual(100);
      last = score;
    }
  });

  it('returns null when there is nothing to score', () => {
    expect(scoreGuess(0, 1000)).toBeNull();
    expect(scoreGuess(-5, 1000)).toBeNull();
    expect(scoreGuess(NaN, 1000)).toBeNull();
    expect(scoreGuess(100, 0)).toBeNull();
    expect(scoreGuess(100, MIN_SCOREABLE_KM / 2)).toBeNull();
    expect(scoreGuess(100, NaN)).toBeNull();
  });
});

describe('parseGuess', () => {
  it('reads plain and comma-grouped numbers', () => {
    expect(parseGuess('8000')).toEqual({ ok: true, km: 8000 });
    expect(parseGuess(' 8,000 ')).toEqual({ ok: true, km: 8000 });
    expect(parseGuess('7.5')).toEqual({ ok: true, km: 7.5 });
  });

  it('converts miles to km', () => {
    const { ok, km } = parseGuess('1000', 'mi');
    expect(ok).toBe(true);
    expect(km).toBeCloseTo(1000 * KM_PER_MILE, 6);
  });

  it('rejects empty and blank guesses', () => {
    expect(parseGuess('').ok).toBe(false);
    expect(parseGuess('   ').ok).toBe(false);
    expect(parseGuess(undefined).ok).toBe(false);
    expect(parseGuess(null).ok).toBe(false);
  });

  it('rejects text, signs and exponents', () => {
    for (const bad of ['abc', '12km', '-5', '+5', '1e3', '1.2.3', '.', 'Infinity', 'NaN']) {
      expect(parseGuess(bad).ok, bad).toBe(false);
    }
  });

  it('rejects zero and absurdly long routes', () => {
    expect(parseGuess('0').ok).toBe(false);
    expect(parseGuess('0.0').ok).toBe(false);
    expect(parseGuess(String(MAX_GUESS_KM + 1)).ok).toBe(false);
    expect(parseGuess(String(MAX_GUESS_KM)).ok).toBe(true);
    expect(parseGuess('1e308').ok).toBe(false);
  });

  it('rejects an unknown unit', () => {
    expect(parseGuess('100', 'furlongs').ok).toBe(false);
  });

  it('explains every rejection', () => {
    expect(parseGuess('').error).toMatch(/guess/i);
    expect(parseGuess('abc').error).toMatch(/digits/i);
    expect(parseGuess('0').error).toMatch(/more than 0/i);
  });
});

describe('judgeRound', () => {
  it('scores a route with a distance', () => {
    expect(judgeRound(890, 1000, 5)).toEqual({ status: 'scored', score: 81, guessKm: 890, actualKm: 1000 });
  });

  it('does not score a route with fewer than two located hops', () => {
    expect(judgeRound(900, 0, 0)).toEqual({ status: 'unscoreable', reason: 'few-hops' });
    expect(judgeRound(900, 0, 1)).toEqual({ status: 'unscoreable', reason: 'few-hops' });
  });

  it('does not score a route that stayed in one place', () => {
    expect(judgeRound(900, 0, 3)).toEqual({ status: 'unscoreable', reason: 'no-distance' });
  });
});

describe('unscoreableReason', () => {
  it('lets a route with a distance be played', () => {
    expect(unscoreableReason(1000, 5)).toBeNull();
    expect(unscoreableReason(MIN_SCOREABLE_KM, 2)).toBeNull();
  });

  it('needs at least two located hops', () => {
    expect(unscoreableReason(0, 0)).toBe('few-hops');
    expect(unscoreableReason(0, 1)).toBe('few-hops');
  });

  it('needs the route to have gone somewhere', () => {
    expect(unscoreableReason(0, 3)).toBe('no-distance');
    expect(unscoreableReason(MIN_SCOREABLE_KM / 2, 3)).toBe('no-distance');
    expect(unscoreableReason(NaN, 3)).toBe('no-distance');
  });
});

describe('words', () => {
  it('has a message for every score', () => {
    for (const score of [0, 1, 24, 25, 54, 55, 79, 80, 94, 95, 100]) {
      expect(scoreMessage(score).length).toBeGreaterThan(0);
    }
    expect(scoreMessage(100)).not.toBe(scoreMessage(0));
  });

  it('formats distances in the chosen unit', () => {
    expect(formatDistance(1000, 'km')).toBe('1,000 km');
    expect(formatDistance(1609.344, 'mi')).toBe('1,000 mi');
  });

  it('says which way a guess missed', () => {
    expect(missLabel(1200, 1000, 'km')).toBe('200 km too far');
    expect(missLabel(800, 1000, 'km')).toBe('200 km too short');
    expect(missLabel(1000.2, 1000, 'km')).toBeNull();
  });
});
