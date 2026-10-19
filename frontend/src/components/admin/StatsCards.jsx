import { formatMs, formatNumber, formatPercent, formatUSD } from '../../lib/format.js';
import { STATUS } from './ui.jsx';

function Tile({ label, value, note, hero }) {
  return (
    <div className={`tile${hero ? ' tile--hero' : ''}`}>
      <span className="tile__label">{label}</span>
      <span className="tile__value">{value}</span>
      {note && <span className="tile__note">{note}</span>}
    </div>
  );
}

export default function StatsCards({ summary, pricing }) {
  const s = summary;
  return (
    <div className="tiles">
      <Tile hero label="Draf dibuat" value={formatNumber(s.generations)} note="Semua status" />
      <Tile label="User aktif" value={formatNumber(s.active_users)} />
      <Tile
        label="Token"
        value={formatNumber(s.input_tokens + s.output_tokens)}
        note={`${formatNumber(s.input_tokens)} masuk · ${formatNumber(s.output_tokens)} keluar`}
      />
      <Tile
        label="Estimasi biaya"
        value={pricing.configured ? formatUSD(s.estimated_cost_usd) : '—'}
        note={pricing.configured ? 'Dari harga per juta token' : 'Harga belum diatur di server'}
      />
      <Tile label="Error rate" value={formatPercent(s.error_rate)} note="Error + timeout" />
      <Tile label="Rata-rata durasi" value={formatMs(s.avg_duration_ms)} note="Draf sukses" />
      <Tile label="TTFT p50" value={formatMs(s.p50_ttft_ms)} note="Waktu ke kata pertama" />
    </div>
  );
}

export function StatusBreakdown({ byStatus, total }) {
  const rows = ['success', 'error', 'timeout', 'cancelled'];
  return (
    <section className="card" aria-labelledby="status-title">
      <h2 id="status-title" className="card__title">
        Rincian status
      </h2>
      <div className="table-wrap">
        <table className="table table--compact">
          <thead>
            <tr>
              <th scope="col">Status</th>
              <th scope="col" className="num">
                Jumlah
              </th>
              <th scope="col" className="num">
                Porsi
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((k) => (
              <tr key={k}>
                <td>
                  <span className={`status status--${k}`}>
                    <span aria-hidden="true" className="status__icon">
                      {STATUS[k].icon}
                    </span>
                    {STATUS[k].label}
                  </span>
                </td>
                <td className="num">{formatNumber(byStatus[k])}</td>
                <td className="num">{total ? formatPercent(byStatus[k] / total) : '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
