/** "Oct 1, 2026" for a build date like "2026-10-01" (UTC, so the day never shifts with the time zone), or null if it is not a date. */
export function formatBuildDate(iso) {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso ?? '');
  if (!m) return null;
  const date = new Date(Date.UTC(+m[1], +m[2] - 1, +m[3]));
  // Date.UTC rolls an impossible date over (month 13, day 40); a real one reads back unchanged.
  if (date.getUTCMonth() !== +m[2] - 1 || date.getUTCDate() !== +m[3]) return null;
  return date.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric', timeZone: 'UTC' });
}
