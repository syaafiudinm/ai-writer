import { useId, useLayoutEffect, useState } from 'react';
import { formatCompact, formatDate, formatNumber, formatShortDate } from '../../lib/format.js';

const HEIGHT = 240;
const M = { top: 22, right: 12, bottom: 30, left: 46 };
const MAX_BAR = 24;
const GAP = 2;

// niceStep membulatkan jarak tick ke 1/2/2,5/5 × 10^n.
export function niceTicks(maxValue, count = 4) {
  if (!maxValue || maxValue <= 0) return { max: count, ticks: Array.from({ length: count + 1 }, (_, i) => i) };
  const raw = maxValue / count;
  const pow = 10 ** Math.floor(Math.log10(raw));
  const f = raw / pow;
  let step = (f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10) * pow;
  if (step < 1) step = 1;
  const max = step * Math.ceil(maxValue / step);
  const ticks = [];
  for (let v = 0; v <= max + step / 2; v += step) ticks.push(Math.round(v * 100) / 100);
  return { max, ticks };
}

function barPath(x, base, w, h) {
  if (h <= 0) return '';
  const r = Math.min(4, w / 2, h);
  const top = base - h;
  return `M${x},${base}V${top + r}Q${x},${top} ${x + r},${top}H${x + w - r}Q${x + w},${top} ${x + w},${top + r}V${base}Z`;
}

// useWidth memakai callback ref agar ukuran diukur ulang saat plot dipasang lagi
// (mis. kembali dari tampilan tabel).
function useWidth() {
  const [el, setEl] = useState(null);
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    if (!el) return undefined;
    setWidth(el.clientWidth);
    const ro = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, [el]);
  return [setEl, width];
}

/**
 * UsageChart: grafik kolom harian satu seri.
 * data: [{date: 'YYYY-MM-DD', value: number, detail?: [[label, text], …]}]
 */
export default function UsageChart({ title, description, data, unit, valueFormat = formatNumber }) {
  const uid = useId();
  const [wrapRef, width] = useWidth();
  const [active, setActive] = useState(null);
  const [view, setView] = useState('chart');

  const n = data.length;
  const values = data.map((d) => d.value);
  const total = values.reduce((a, b) => a + b, 0);
  const peakIdx = n ? values.indexOf(Math.max(...values)) : -1;
  const { max, ticks } = niceTicks(peakIdx >= 0 ? values[peakIdx] : 0);

  const innerW = Math.max(0, width - M.left - M.right);
  const innerH = HEIGHT - M.top - M.bottom;
  const base = M.top + innerH;
  const band = n ? innerW / n : 0;
  const barW = Math.max(1, Math.min(MAX_BAR, band - GAP));
  const y = (v) => (v / max) * innerH;
  const barX = (i) => M.left + band * i + (band - barW) / 2;

  const labelEvery = Math.max(1, Math.ceil(n / Math.max(1, Math.floor(innerW / 64))));
  const showLabel = (i) => (n - 1 - i) % labelEvery === 0;

  const indexFromPointer = (e) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const px = e.clientX - rect.left - M.left;
    if (px < 0 || px > innerW) return null;
    return Math.min(n - 1, Math.max(0, Math.floor(px / band)));
  };

  function onKeyDown(e) {
    if (!n) return;
    const keys = { ArrowLeft: -1, ArrowRight: 1 };
    if (e.key in keys) {
      e.preventDefault();
      setActive((a) => Math.min(n - 1, Math.max(0, (a ?? n - 1) + keys[e.key])));
    } else if (e.key === 'Home') {
      e.preventDefault();
      setActive(0);
    } else if (e.key === 'End') {
      e.preventDefault();
      setActive(n - 1);
    } else if (e.key === 'Escape') {
      setActive(null);
    }
  }

  const activeItem = active != null && active < n ? data[active] : null;
  const tipLeft = activeItem ? Math.min(Math.max(barX(active) + barW / 2, 80), width - 80) : 0;
  const tipTop = activeItem ? Math.max(base - y(activeItem.value) - 10, 64) : 0;

  const summary =
    n === 0
      ? `${title}: tidak ada data.`
      : `${title}, ${n} hari. Total ${valueFormat(total)}${unit ? ` ${unit}` : ''}. Tertinggi ${valueFormat(values[peakIdx])} pada ${formatDate(data[peakIdx].date)}. Gunakan panah kiri/kanan untuk membaca per hari.`;

  return (
    <figure className="card chart" aria-labelledby={`${uid}-title`}>
      <div className="chart__head">
        <div>
          <figcaption id={`${uid}-title`} className="card__title">
            {title}
          </figcaption>
          {description && <p className="chart__desc">{description}</p>}
        </div>
        <div className="segmented segmented--sm" role="group" aria-label="Tampilan">
          <button
            type="button"
            className={`segmented__btn${view === 'chart' ? ' is-checked' : ''}`}
            aria-pressed={view === 'chart'}
            onClick={() => setView('chart')}
          >
            Grafik
          </button>
          <button
            type="button"
            className={`segmented__btn${view === 'table' ? ' is-checked' : ''}`}
            aria-pressed={view === 'table'}
            onClick={() => setView('table')}
          >
            Tabel
          </button>
        </div>
      </div>

      {view === 'chart' ? (
        <div className="chart__plot" ref={wrapRef}>
          {width > 0 && (
            <svg
              width={width}
              height={HEIGHT}
              role="img"
              aria-label={summary}
              tabIndex={0}
              onPointerMove={(e) => setActive(indexFromPointer(e))}
              onPointerLeave={() => setActive(null)}
              onKeyDown={onKeyDown}
              onFocus={() => setActive((a) => a ?? (n ? n - 1 : null))}
              onBlur={() => setActive(null)}
            >
              {ticks.map((t) => (
                <g key={t}>
                  <line className="chart__grid" x1={M.left} x2={width - M.right} y1={base - y(t)} y2={base - y(t)} />
                  <text className="chart__tick" x={M.left - 8} y={base - y(t)} dy="0.32em" textAnchor="end">
                    {formatCompact(t)}
                  </text>
                </g>
              ))}

              {active != null && (
                <rect className="chart__band" x={M.left + band * active} y={M.top} width={band} height={innerH} />
              )}

              {data.map((d, i) => (
                <path
                  key={d.date}
                  className={`chart__bar${i === active ? ' is-active' : ''}`}
                  d={barPath(barX(i), base, barW, y(d.value))}
                />
              ))}

              <line className="chart__axis" x1={M.left} x2={width - M.right} y1={base} y2={base} />

              {data.map((d, i) =>
                showLabel(i) ? (
                  <text
                    key={d.date}
                    className="chart__tick"
                    x={i === n - 1 ? barX(i) + barW : barX(i) + barW / 2}
                    y={base + 18}
                    textAnchor={i === n - 1 ? 'end' : 'middle'}
                  >
                    {formatShortDate(d.date)}
                  </text>
                ) : null,
              )}

              {peakIdx >= 0 && values[peakIdx] > 0 && active == null && (
                <text
                  className="chart__label"
                  x={Math.min(Math.max(barX(peakIdx) + barW / 2, M.left + 16), width - M.right - 16)}
                  y={base - y(values[peakIdx]) - 6}
                  textAnchor="middle"
                >
                  {formatCompact(values[peakIdx])}
                </text>
              )}
            </svg>
          )}

          {activeItem && (
            <div className="chart__tip" style={{ left: tipLeft, top: tipTop }} aria-live="polite">
              <strong className="chart__tip-value">
                {valueFormat(activeItem.value)}
                {unit ? ` ${unit}` : ''}
              </strong>
              <span className="chart__tip-date">{formatDate(activeItem.date)}</span>
              {activeItem.detail?.map(([k, v]) => (
                <span key={k} className="chart__tip-row">
                  {k}: {v}
                </span>
              ))}
            </div>
          )}
        </div>
      ) : (
        <div className="table-wrap chart__table">
          <table className="table table--compact">
            <thead>
              <tr>
                <th scope="col">Tanggal</th>
                <th scope="col" className="num">
                  {unit ? unit[0].toUpperCase() + unit.slice(1) : 'Nilai'}
                </th>
                {data[0]?.detail?.map(([k]) => (
                  <th key={k} scope="col" className="num">
                    {k}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.map((d) => (
                <tr key={d.date}>
                  <td>{formatDate(d.date)}</td>
                  <td className="num">{valueFormat(d.value)}</td>
                  {d.detail?.map(([k, v]) => (
                    <td key={k} className="num">
                      {v}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </figure>
  );
}
