import { useEffect, useRef, useState } from 'react';

async function writeClipboard({ text, html }) {
  if (html && window.ClipboardItem && navigator.clipboard?.write) {
    await navigator.clipboard.write([
      new ClipboardItem({
        'text/plain': new Blob([text], { type: 'text/plain' }),
        'text/html': new Blob([html], { type: 'text/html' }),
      }),
    ]);
    return;
  }
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  // Fallback untuk konteks non-HTTPS.
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.setAttribute('readonly', '');
  ta.style.position = 'fixed';
  ta.style.opacity = '0';
  document.body.appendChild(ta);
  ta.select();
  const ok = document.execCommand('copy');
  ta.remove();
  if (!ok) throw new Error('copy gagal');
}

// CopyButton menyalin hasil getContent() → {text, html?}.
export default function CopyButton({ getContent, label = 'Salin', disabled, className = 'btn btn--sm' }) {
  const [state, setState] = useState('idle');
  const timer = useRef(null);
  useEffect(() => () => clearTimeout(timer.current), []);

  async function handleClick() {
    try {
      await writeClipboard(getContent());
      setState('done');
    } catch {
      setState('error');
    }
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setState('idle'), 2000);
  }

  return (
    <button type="button" className={className} onClick={handleClick} disabled={disabled}>
      <span aria-live="polite">{state === 'done' ? 'Tersalin ✓' : state === 'error' ? 'Gagal menyalin' : label}</span>
    </button>
  );
}
