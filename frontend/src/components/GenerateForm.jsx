import { useId } from 'react';

export const DEFAULT_BRIEF = {
  type: 'berita',
  topic: '',
  keywords: '',
  tone: 'formal',
  length: 'sedang',
  language: 'id',
  facts: '',
};

const TYPES = [
  { value: 'berita', label: 'Berita', hint: 'Piramida terbalik, 5W+1H, tanpa opini' },
  { value: 'artikel', label: 'Artikel', hint: 'Bersubjudul, mengalir, bisa beropini' },
];
const TONES = [
  { value: 'formal', label: 'Formal' },
  { value: 'santai', label: 'Santai' },
  { value: 'persuasif', label: 'Persuasif' },
  { value: 'edukatif', label: 'Edukatif' },
];
const LENGTHS = [
  { value: 'pendek', label: 'Pendek', hint: '±400 kata' },
  { value: 'sedang', label: 'Sedang', hint: '±800 kata' },
  { value: 'panjang', label: 'Panjang', hint: '±1.400 kata' },
];
const LANGUAGES = [
  { value: 'id', label: 'Indonesia' },
  { value: 'en', label: 'English' },
];

export const LIMITS = { topic: [5, 500], keywords: 200, facts: 4000 };

// validateBrief meniru validasi server agar kesalahan terlihat sebelum request.
export function validateBrief(b) {
  const errs = {};
  const topic = [...b.topic.trim()].length;
  if (topic < LIMITS.topic[0] || topic > LIMITS.topic[1]) errs.topic = 'Topik wajib diisi (5–500 karakter).';
  if ([...b.keywords.trim()].length > LIMITS.keywords) errs.keywords = 'Kata kunci maksimal 200 karakter.';
  if ([...b.facts.trim()].length > LIMITS.facts) errs.facts = 'Fakta maksimal 4.000 karakter.';
  return errs;
}

function Segmented({ name, legend, options, value, onChange, disabled, error }) {
  return (
    <fieldset className="field" disabled={disabled}>
      <legend>{legend}</legend>
      <div className="segmented" role="radiogroup">
        {options.map((o) => (
          <label key={o.value} className={`segmented__opt${value === o.value ? ' is-checked' : ''}`}>
            <input
              type="radio"
              name={name}
              value={o.value}
              checked={value === o.value}
              onChange={() => onChange(o.value)}
            />
            <span className="segmented__label">{o.label}</span>
            {o.hint && <span className="segmented__hint">{o.hint}</span>}
          </label>
        ))}
      </div>
      {error && <p className="field__error">{error}</p>}
    </fieldset>
  );
}

function Counter({ value, max, id }) {
  const n = [...value].length;
  return (
    <span id={id} className={`counter${n > max ? ' is-over' : ''}`}>
      {n.toLocaleString('id-ID')}/{max.toLocaleString('id-ID')}
    </span>
  );
}

export default function GenerateForm({ brief, onChange, onSubmit, busy, errors = {} }) {
  const uid = useId();
  const set = (key) => (v) => onChange({ ...brief, [key]: v });
  const setFromEvent = (key) => (e) => onChange({ ...brief, [key]: e.target.value });

  const describe = (key, extra) => [errors[key] ? `${uid}-${key}-err` : null, extra].filter(Boolean).join(' ') || undefined;

  return (
    <form
      className="card brief"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit();
      }}
      noValidate
    >
      <h2 className="card__title">Brief</h2>

      <Segmented
        name="type"
        legend="Jenis tulisan"
        options={TYPES}
        value={brief.type}
        onChange={set('type')}
        disabled={busy}
        error={errors.type}
      />

      <div className="field">
        <div className="field__row">
          <label htmlFor={`${uid}-topic`}>Topik</label>
          <Counter value={brief.topic} max={LIMITS.topic[1]} id={`${uid}-topic-count`} />
        </div>
        <textarea
          id={`${uid}-topic`}
          rows={3}
          value={brief.topic}
          onChange={setFromEvent('topic')}
          disabled={busy}
          placeholder="Contoh: Pemkot Makassar membuka 12 taman baru di kawasan pesisir"
          aria-invalid={!!errors.topic}
          aria-describedby={describe('topic', `${uid}-topic-count`)}
        />
        {errors.topic && (
          <p id={`${uid}-topic-err`} className="field__error">
            {errors.topic}
          </p>
        )}
      </div>

      <div className="field">
        <div className="field__row">
          <label htmlFor={`${uid}-keywords`}>
            Kata kunci <span className="optional">opsional</span>
          </label>
          <Counter value={brief.keywords} max={LIMITS.keywords} id={`${uid}-kw-count`} />
        </div>
        <input
          id={`${uid}-keywords`}
          type="text"
          value={brief.keywords}
          onChange={setFromEvent('keywords')}
          disabled={busy}
          placeholder="Pisahkan dengan koma"
          aria-invalid={!!errors.keywords}
          aria-describedby={describe('keywords', `${uid}-kw-count`)}
        />
        {errors.keywords && (
          <p id={`${uid}-keywords-err`} className="field__error">
            {errors.keywords}
          </p>
        )}
      </div>

      <div className="field-grid">
        <div className="field">
          <label htmlFor={`${uid}-tone`}>Gaya bahasa</label>
          <select
            id={`${uid}-tone`}
            value={brief.tone}
            onChange={setFromEvent('tone')}
            disabled={busy}
            aria-invalid={!!errors.tone}
          >
            {TONES.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
          {errors.tone && <p className="field__error">{errors.tone}</p>}
        </div>
        <Segmented
          name="language"
          legend="Bahasa"
          options={LANGUAGES}
          value={brief.language}
          onChange={set('language')}
          disabled={busy}
          error={errors.language}
        />
      </div>

      <Segmented
        name="length"
        legend="Panjang"
        options={LENGTHS}
        value={brief.length}
        onChange={set('length')}
        disabled={busy}
        error={errors.length}
      />

      <div className="field">
        <div className="field__row">
          <label htmlFor={`${uid}-facts`}>
            Fakta &amp; data <span className="optional">opsional</span>
          </label>
          <Counter value={brief.facts} max={LIMITS.facts} id={`${uid}-facts-count`} />
        </div>
        <textarea
          id={`${uid}-facts`}
          rows={7}
          value={brief.facts}
          onChange={setFromEvent('facts')}
          disabled={busy}
          placeholder={'Satu fakta per baris: nama, jabatan, angka, tanggal, kutipan.\nYang tidak ada di sini akan ditandai [perlu konfirmasi].'}
          aria-invalid={!!errors.facts}
          aria-describedby={describe('facts', `${uid}-facts-count ${uid}-facts-hint`)}
        />
        <p id={`${uid}-facts-hint`} className="hint">
          Fakta hanya dipakai untuk draf ini dan tidak disimpan di server.
        </p>
        {errors.facts && (
          <p id={`${uid}-facts-err`} className="field__error">
            {errors.facts}
          </p>
        )}
      </div>

      <button type="submit" className="btn btn--ink btn--block btn--lg" disabled={busy}>
        {busy ? 'Sedang menulis…' : 'Tulis draf'}
      </button>
    </form>
  );
}
