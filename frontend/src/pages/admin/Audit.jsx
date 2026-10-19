import { useSearchParams } from 'react-router';
import { toQuery } from '../../lib/api.js';
import { useApi } from '../../lib/useApi.js';
import { formatDateTime } from '../../lib/format.js';
import { ErrorAlert, Pagination } from '../../components/admin/ui.jsx';

export const ACTIONS = {
  login_success: 'Login berhasil',
  login_failed: 'Login gagal',
  login_rate_limited: 'Login diblokir sementara',
  logout: 'Logout',
  user_created: 'User dibuat',
  user_role_changed: 'Peran diubah',
  user_disabled: 'User dinonaktifkan',
  user_enabled: 'User diaktifkan',
  user_password_reset: 'Password direset',
};

function Person({ who, fallback }) {
  if (!who) return <span className="muted">{fallback}</span>;
  return (
    <>
      <span className="cell-main">{who.name}</span>
      <span className="cell-sub">{who.email}</span>
    </>
  );
}

function describe(e) {
  const m = e.metadata || {};
  switch (e.action) {
    case 'user_role_changed':
      return `${m.from} → ${m.to}`;
    case 'user_created':
      return `peran ${m.role}`;
    case 'login_failed':
    case 'login_rate_limited':
      return m.email ? `email: ${m.email}` : '';
    default:
      return Object.keys(m).length ? JSON.stringify(m) : '';
  }
}

export default function Audit() {
  const [params, setParams] = useSearchParams();
  const action = params.get('action') || '';
  const page = Number(params.get('page')) || 1;
  const { data, error, loading, reload } = useApi(`/api/admin/audit${toQuery({ action, page })}`);

  return (
    <main className="page">
      <header className="page-head">
        <p className="kicker">Admin</p>
        <h1 className="page-title">Jejak audit</h1>
        <p className="lede">Login dan semua perubahan akun, terbaru di atas.</p>
      </header>

      <div className="filters" role="group" aria-label="Filter audit">
        <label className="filter">
          Aksi
          <select value={action} onChange={(e) => setParams(e.target.value ? { action: e.target.value } : {})}>
            <option value="">Semua aksi</option>
            {Object.entries(ACTIONS).map(([k, label]) => (
              <option key={k} value={k}>
                {label}
              </option>
            ))}
          </select>
        </label>
      </div>

      <ErrorAlert error={error} onRetry={reload} />

      {data ? (
        <section className={`card card--flush${loading ? ' is-refreshing' : ''}`} aria-busy={loading}>
          {data.items.length ? (
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th scope="col">Waktu</th>
                    <th scope="col">Aksi</th>
                    <th scope="col">Pelaku</th>
                    <th scope="col">Target</th>
                    <th scope="col">Detail</th>
                    <th scope="col">IP</th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((e) => (
                    <tr key={e.id}>
                      <td className="nowrap">{formatDateTime(e.created_at)}</td>
                      <td>
                        <span className={`action action--${e.action}`}>{ACTIONS[e.action] || e.action}</span>
                      </td>
                      <td>
                        <Person who={e.actor} fallback="Tidak dikenal" />
                      </td>
                      <td>
                        <Person who={e.target} fallback="—" />
                      </td>
                      <td className="cell-detail">{describe(e)}</td>
                      <td className="mono nowrap">{e.ip || '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p className="empty">Belum ada catatan audit.</p>
          )}
          <Pagination
            page={data.page}
            totalPages={data.total_pages}
            total={data.total}
            disabled={loading}
            onPage={(p) => setParams({ ...(action ? { action } : {}), page: String(p) })}
          />
        </section>
      ) : (
        !error && <p className="muted">Memuat audit…</p>
      )}
    </main>
  );
}
