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
      {/* Distinct keys: React must remount rather than flip this one node from button to submit mid-click, or the click that cancels also submits the form. */}
      {busy ? (
        <button key="cancel" type="button" onClick={onCancel}>
          Cancel
        </button>
      ) : (
        <button key="trace" type="submit">Trace</button>
      )}
    </form>
  );
}
