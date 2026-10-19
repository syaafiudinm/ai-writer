import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router';
import { toQuery } from '../../lib/api.js';
import { useApi } from '../../lib/useApi.js';
import { formatDate, formatNumber, formatPercent, lastNDays } from '../../lib/format.js';
import StatsCards, { StatusBreakdown } from '../../components/admin/StatsCards.jsx';
import UsageChart from '../../components/admin/UsageChart.jsx';
import { ErrorAlert } from '../../components/admin/ui.jsx';

const PRESETS = [7, 30, 90];

function RangeFilter({ from, to, days, onPreset, onCustom, disabled }) {
  const [draft, setDraft] = useState({ from, to });
  const [open, setOpen] = useState(!days);

  return (
    <div className="filters" role="group" aria-label="Rentang tanggal">
      <div className="segmented segmented--sm">
        {PRESETS.map((d) => (
          <button
            key={d}
            type="button"
            className={`segmented__btn${days === d ? ' is-checked' : ''}`}
            aria-pressed={days === d}
            onClick={() => {
              setOpen(false);
              onPreset(d);
            }}
            disabled={disabled}
          >
            {d} hari
          </button>
        ))}
        <button
          type="button"
          className={`segmented__btn${!days ? ' is-checked' : ''}`}
          aria-pressed={!days}
          aria-expanded={open}
          onClick={() => {
            setDraft({ from, to });
            setOpen((o) => !o);
          }}
        >
          Pilih tanggal
        </button>
      </div>
      {open && (
        <form
          className="filters__custom"
          onSubmit={(e) => {
            e.preventDefault();
            onCustom(draft.from, draft.to);
          }}
        >
          <label>
            Dari
            <input type="date" value={draft.from} max={draft.to} onChange={(e) => setDraft({ ...draft, from: e.target.value })} required />
          </label>
          <label>
            Sampai
            <input type="date" value={draft.to} min={draft.from} onChange={(e) => setDraft({ ...draft, to: e.target.value })} required />
          </label>
          <button type="submit" className="btn btn--sm btn--ink" disabled={disabled}>
            Terapkan
          </button>
        </form>
      )}
    </div>
  );
}

export default function Dashboard() {
  const [params, setParams] = useSearchParams();
  const days = params.get('from') ? null : Number(params.get('days')) || 30;
  const range = days ? lastNDays(days) : { from: params.get('from'), to: params.get('to') || params.get('from') };

  const { data, error, loading, reload } = useApi(`/api/admin/stats${toQuery(range)}`);

  const genSeries = useMemo(
    () =>
      (data?.daily || []).map((d) => ({
        date: d.date,
        value: d.generations,
        detail: [
          ['User aktif', formatNumber(d.active_users)],
          ['Error rate', formatPercent(d.error_rate)],
        ],
      })),
    [data],
  );
  const tokenSeries = useMemo(
    () =>
      (data?.daily || []).map((d) => ({
        date: d.date,
        value: d.input_tokens + d.output_tokens,
        detail: [
          ['Masuk', formatNumber(d.input_tokens)],
          ['Keluar', formatNumber(d.output_tokens)],
        ],
      })),
    [data],
  );

  return (
    <main className="page">
      <header className="page-head">
        <p className="kicker">Admin</p>
        <h1 className="page-title">Dashboard pemakaian</h1>
        {data && (
          <p className="lede">
            {formatDate(data.range.from)} – {formatDate(data.range.to)} · zona waktu {data.range.timezone}
          </p>
        )}
      </header>

      <RangeFilter
        key={`${range.from}-${range.to}`}
        from={range.from}
        to={range.to}
        days={days}
        disabled={loading}
        onPreset={(d) => setParams({ days: String(d) })}
        onCustom={(from, to) => setParams({ from, to })}
      />

      <ErrorAlert error={error} onRetry={reload} />

      {data ? (
        <div className={`stack${loading ? ' is-refreshing' : ''}`} aria-busy={loading}>
          <StatsCards summary={data.summary} pricing={data.pricing} />
          <div className="chart-grid">
            <UsageChart title="Draf per hari" description="Jumlah permintaan generate, semua status." data={genSeries} unit="draf" />
            <UsageChart title="Token per hari" description="Token masuk + keluar (termasuk thinking)." data={tokenSeries} unit="token" />
          </div>
          <StatusBreakdown byStatus={data.summary.by_status} total={data.summary.generations} />
        </div>
      ) : (
        !error && <p className="muted">Memuat statistik…</p>
      )}
    </main>
  );
}
