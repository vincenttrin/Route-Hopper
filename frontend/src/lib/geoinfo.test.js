import { describe, expect, it } from 'vitest';
import { formatBuildDate } from './geoinfo.js';

describe('formatBuildDate', () => {
  it('formats the build date without shifting the day', () => {
    expect(formatBuildDate('2026-10-01')).toBe('Oct 1, 2026');
    expect(formatBuildDate('2026-01-31')).toBe('Jan 31, 2026');
  });

  it('is null for anything else', () => {
    for (const v of [undefined, null, '', 'yesterday', '2026-13-40', '2026-10']) expect(formatBuildDate(v)).toBeNull();
  });
});
