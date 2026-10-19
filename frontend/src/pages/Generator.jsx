import { useEffect, useRef, useState } from 'react';
import GenerateForm, { DEFAULT_BRIEF, validateBrief } from '../components/GenerateForm.jsx';
import ArticlePreview from '../components/ArticlePreview.jsx';
import CopyButton from '../components/CopyButton.jsx';
import { streamGenerate } from '../lib/sse.js';
import { countMarkers } from '../lib/markers.js';
import { formatNumber } from '../lib/format.js';

const STATUS_TEXT = {
  streaming: 'Menulis…',
  done: 'Selesai',
  stopped: 'Dihentikan',
  error: 'Gagal',
};

export default function Generator() {
  const [brief, setBrief] = useState(DEFAULT_BRIEF);
  const [errors, setErrors] = useState({});
  const [phase, setPhase] = useState('idle'); // idle | streaming | done | stopped | error
  const [text, setText] = useState('');
  const [model, setModel] = useState(null);
  const [result, setResult] = useState(null);
  const [failure, setFailure] = useState(null);
  const controller = useRef(null);
  const articleRef = useRef(null);
  const previewRef = useRef(null);

  // Batalkan stream jika user pindah halaman.
  useEffect(() => () => controller.current?.abort(), []);

  const streaming = phase === 'streaming';

  async function handleSubmit() {
    const clientErrors = validateBrief(brief);
    setErrors(clientErrors);
    if (Object.keys(clientErrors).length) return;

    controller.current?.abort();
    const ac = new AbortController();
    controller.current = ac;

    setPhase('streaming');
    setText('');
    setModel(null);
    setResult(null);
    setFailure(null);
    if (window.matchMedia('(max-width: 960px)').matches) {
      previewRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    try {
      const done = await streamGenerate(brief, {
        signal: ac.signal,
        onMeta: (m) => setModel(m.model),
        onText: (t) => setText((prev) => prev + t),
      });
      setResult(done);
      setPhase('done');
    } catch (err) {
      if (ac.signal.aborted) {
        setPhase('stopped');
        return;
      }
      if (err.fields && Object.keys(err.fields).length) setErrors(err.fields);
      setFailure(err);
      setPhase('error');
    } finally {
      if (controller.current === ac) controller.current = null;
    }
  }

  function handleStop() {
    controller.current?.abort();
  }

  function handleClear() {
    setText('');
    setPhase('idle');
    setResult(null);
    setFailure(null);
    setModel(null);
  }

  const markers = countMarkers(text);
  const hasText = text.length > 0;

  const getRich = () => ({
    text: articleRef.current?.innerText ?? text,
    html: articleRef.current?.innerHTML,
  });
  const getMarkdown = () => ({ text });

  return (
    <main className="page generator">
      <header className="page-head">
        <p className="kicker">Meja tulis</p>
        <h1 className="page-title">Tulis draf berita &amp; artikel</h1>
        <p className="lede">
          Isi brief, lalu periksa draf sebelum terbit. Fakta yang belum ada di brief ditandai{' '}
          <mark className="confirm-marker">[perlu konfirmasi]</mark>.
        </p>
      </header>

      <div className="generator__grid">
        <GenerateForm brief={brief} onChange={setBrief} onSubmit={handleSubmit} busy={streaming} errors={errors} />

        <section className="card preview" ref={previewRef} aria-labelledby="preview-title">
          <div className="preview__bar">
            <div className="preview__label">
              <h2 id="preview-title" className="badge-draft">
                Draf AI
              </h2>
              {phase !== 'idle' && (
                <span className={`status-pill status-pill--${phase}`} role="status">
                  {STATUS_TEXT[phase]}
                </span>
              )}
            </div>
            <div className="preview__actions">
              {streaming ? (
                <button type="button" className="btn btn--sm btn--ink" onClick={handleStop}>
                  ■ Stop
                </button>
              ) : (
                <>
                  <CopyButton getContent={getRich} disabled={!hasText} label="Salin" />
                  <CopyButton getContent={getMarkdown} disabled={!hasText} label="Salin Markdown" />
                  <button type="button" className="btn btn--sm btn--ghost" onClick={handleClear} disabled={!hasText}>
                    Bersihkan
                  </button>
                </>
              )}
            </div>
          </div>

          {failure && (
            <p className="alert alert--error" role="alert">
              {failure.message}
              {hasText && ' Teks yang sudah ditulis tetap ditampilkan di bawah.'}
            </p>
          )}
          {phase === 'stopped' && (
            <p className="alert" role="status">
              Penulisan dihentikan. Draf di bawah belum lengkap.
            </p>
          )}
          {result?.stop_reason === 'max_tokens' && (
            <p className="alert alert--warn" role="status">
              Draf terpotong karena mencapai batas panjang. Coba panjang “Sedang” atau perjelas brief.
            </p>
          )}
          {markers > 0 && !streaming && (
            <p className="alert alert--warn">
              <strong>{markers} penanda</strong> perlu dikonfirmasi sebelum terbit.
            </p>
          )}

          {hasText ? (
            <ArticlePreview ref={articleRef} text={text} streaming={streaming} />
          ) : (
            <div className="preview__empty">
              {streaming ? (
                <p className="typing" role="status">
                  Menyiapkan draf<span aria-hidden="true">…</span>
                </p>
              ) : (
                <>
                  <p className="preview__empty-title">Halaman masih kosong.</p>
                  <p>Draf akan muncul di sini kata demi kata setelah brief dikirim.</p>
                </>
              )}
            </div>
          )}

          {(model || result) && (
            <footer className="preview__meta">
              {model && <span>Model: {model}</span>}
              {result && (
                <span>
                  Token: {formatNumber(result.input_tokens)} masuk · {formatNumber(result.output_tokens)} keluar
                </span>
              )}
            </footer>
          )}
        </section>
      </div>
    </main>
  );
}
