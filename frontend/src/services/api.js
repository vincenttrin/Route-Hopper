export async function runTrace(endpoint, maxHops = 30, signal) {
  const res = await fetch('/api/trace', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ endpoint, maxHops }),
    signal,
  });
  if (!res.ok) {
    const text = (await res.text()).trim();
    throw new Error(text || `Trace failed (HTTP ${res.status})`);
  }
  return res.json();
}
