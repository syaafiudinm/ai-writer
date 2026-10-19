import { useSearchParams } from 'react-router';
import { toQuery } from '../../lib/api.js';
import { useApi } from '../../lib/useApi.js';
import LogTable from '../../components/admin/LogTable.jsx';
import { ErrorAlert, Pagination, STATUS } from '../../components/admin/ui.jsx';

const FILTER_KEYS = ['user_id', 'status', 'type', 'from', 'to'];

export default function Logs() {
  const [params, setParams] = useSearchParams();
  const filter = Object.fromEntries(FILTER_KEYS.map((k) => [k, params.get(k) || '']));
  const page = Number(params.get('page')) || 1;

  const { data, error, loading, reload } = useApi(`/api/admin/logs${toQuery({ ...filter, page })}`);
  const users = useApi('/api/admin/users');
  // Estimasi biaya hanya berarti jika harga diatur; cek lewat data log itu sendiri.
  const showCost = data?.items?.some((l) => l.estimated_cost_usd > 0);

  const update = (patch) => {
    const next = { ...filter, ...patch };
    const clean = Object.fromEntries(Object.entries(next).filter(([, v]) => v));
    setParams(clean);
  };

  const hasFilter = FILTER_KEYS.some((k) => filter[k]);

  return (
    <main className="page">
      <header className="page-head">
        <p className="kicker">Admin</p>
        <h1 className="page-title">Log generate</h1>
        <p className="lede">Metadata setiap permintaan. Isi fakta dan draf tidak pernah disimpan.</p>
      </header>

      <div className="filters" role="group" aria-label="Filter log">
        <label className="filter">
          Penulis
          <select value={filter.user_id} onChange={(e) => update({ user_id: e.target.value })}>
            <option value="">Semua</option>
            {users.data?.items.map((u) => (
              <option key={u.id} value={u.id}>
                {u.name}
              </option>
            ))}
          </select>
        </label>
        <label className="filter">
          Status
          <select value={filter.status} onChange={(e) => update({ status: e.target.value })}>
            <option value="">Semua</option>
            {Object.entries(STATUS).map(([k, s]) => (
              <option key={k} value={k}>
                {s.label}
              </option>
            ))}
          </select>
        </label>
        <label className="filter">
          Jenis
          <select value={filter.type} onChange={(e) => update({ type: e.target.value })}>
            <option value="">Semua</option>
            <option value="berita">Berita</option>
            <option value="artikel">Artikel</option>
          </select>
        </label>
        <label className="filter">
          Dari
          <input type="date" value={filter.from} max={filter.to || undefined} onChange={(e) => update({ from: e.target.value })} />
        </label>
        <label className="filter">
          Sampai
          <input type="date" value={filter.to} min={filter.from || undefined} onChange={(e) => update({ to: e.target.value })} />
        </label>
        {hasFilter && (
          <button type="button" className="btn btn--sm btn--ghost" onClick={() => setParams({})}>
            Reset filter
          </button>
        )}
      </div>

      <ErrorAlert error={error} onRetry={reload} />

      {data ? (
        <section className={`card card--flush${loading ? ' is-refreshing' : ''}`} aria-busy={loading}>
          <LogTable items={data.items} showCost={showCost} />
          <Pagination
            page={data.page}
            totalPages={data.total_pages}
            total={data.total}
            disabled={loading}
            onPage={(p) => setParams({ ...Object.fromEntries(params), page: String(p) })}
          />
        </section>
      ) : (
        !error && <p className="muted">Memuat log…</p>
      )}
    </main>
  );
}
