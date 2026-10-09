import { useState } from 'react';

export default function TraceInput({ initial, busy, onSubmit }) {
  const [value, setValue] = useState(initial);
  const submit = (e) => {
    e.preventDefault();
    const v = value.trim();
    if (v) onSubmit(v);
  };
  return (
    <form className="trace-form" onSubmit={submit}>
      <label htmlFor="endpoint" className="sr-only">
        Endpoint
      </label>
      <input
        id="endpoint"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        placeholder="example.com or 93.184.216.34"
        autoComplete="off"
        spellCheck="false"
      />
      <button type="submit" disabled={busy}>
        {busy ? 'Tracing' : 'Trace'}
      </button>
    </form>
  );
}
