import { describe, expect, it } from 'vitest';
import { arcPath, arcPoint } from './arc.js';

const omaha = { lat: 41.26, lng: -95.93 };
const chicago = { lat: 41.88, lng: -87.63 };
const london = { lat: 51.51, lng: -0.13 };
const rome = { lat: 41.9, lng: 12.5 };

describe('arcPoint', () => {
  it('starts exactly on the first hop and lands exactly on the second', () => {
    expect(arcPoint(omaha, chicago, 0)).toEqual(omaha);
    expect(arcPoint(omaha, chicago, 1)).toEqual(chicago);
  });

  it('rises above the straight line in between, like a bouncing ball', () => {
    const mid = arcPoint(london, rome, 0.5);
    const straightLat = (london.lat + rome.lat) / 2;
    expect(mid.lat).toBeGreaterThan(straightLat + 1);
  });

  it('is at its highest in the middle and symmetrical', () => {
    const lift = (t) => arcPoint(omaha, chicago, t).lat - (omaha.lat + (chicago.lat - omaha.lat) * t);
    expect(lift(0.5)).toBeGreaterThan(lift(0.25));
    expect(lift(0.25)).toBeCloseTo(lift(0.75), 1);
  });

  it('rises more for a longer hop', () => {
    const lift = (a, b) => arcPoint(a, b, 0.5).lat - (a.lat + b.lat) / 2;
    expect(lift(london, rome)).toBeGreaterThan(lift(omaha, chicago));
  });

  it('bulges to the same side whichever way the hop goes', () => {
    const there = arcPoint(london, rome, 0.5);
    const back = arcPoint(rome, london, 0.5);
    expect(back.lat).toBeCloseTo(there.lat, 6);
    expect(back.lng).toBeCloseTo(there.lng, 6);
  });

  it('bends a straight north-south hop sideways instead of along itself', () => {
    const south = { lat: 10, lng: 20 };
    const north = { lat: 30, lng: 20 };
    const mid = arcPoint(south, north, 0.5);
    expect(mid.lng).toBeGreaterThan(20);
  });

  it('stays put for two hops in the same place', () => {
    expect(arcPoint(omaha, { ...omaha }, 0.5)).toEqual(omaha);
  });

  it('never leaves the map for hops near the poles', () => {
    const p = arcPoint({ lat: 84, lng: 0 }, { lat: 84, lng: 90 }, 0.5);
    expect(Number.isFinite(p.lat)).toBe(true);
    expect(Math.abs(p.lat)).toBeLessThanOrEqual(90);
  });
});

describe('arcPath', () => {
  it('has steps + 1 points from the first hop to the second', () => {
    const path = arcPath(omaha, chicago, 10);
    expect(path).toHaveLength(11);
    expect(path[0]).toEqual(omaha);
    expect(path[10]).toEqual(chicago);
  });
});
