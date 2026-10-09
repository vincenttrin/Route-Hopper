import { useState } from 'react';

export default function TraceInput({ initial, busy, onSubmit, onCancel }) {
  const [value, setValue] = useState(initial);
  const submit = (e) => {
    e.preventDefault();
    const v = value.trim();
    if (v && !busy) onSubmit(v);
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
      {busy ? (
        <button type="button" onClick={onCancel}>
          Cancel
        </button>
      ) : (
        <button type="submit">Trace</button>
      )}
    </form>
  );
}
