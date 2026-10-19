import { formatDateTime, formatMs, formatNumber, formatUSD } from '../../lib/format.js';
import { StatusBadge } from './ui.jsx';

const LANG = { id: 'ID', en: 'EN' };

export default function LogTable({ items, showCost }) {
  if (!items.length) return <p className="empty">Tidak ada log untuk filter ini.</p>;
  return (
    <div className="table-wrap">
      <table className="table">
        <thead>
          <tr>
            <th scope="col">Waktu</th>
            <th scope="col">Penulis</th>
            <th scope="col">Brief</th>
            <th scope="col">Status</th>
            <th scope="col" className="num">
              Token
            </th>
            <th scope="col" className="num">
              TTFT
            </th>
            <th scope="col" className="num">
              Durasi
            </th>
            {showCost && (
              <th scope="col" className="num">
                Biaya
              </th>
            )}
          </tr>
        </thead>
        <tbody>
          {items.map((l) => (
            <tr key={l.id}>
              <td className="nowrap">{formatDateTime(l.created_at)}</td>
              <td>
                <span className="cell-main">{l.user.name}</span>
                <span className="cell-sub">{l.user.email}</span>
              </td>
              <td className="cell-topic">
                <span className="cell-main" title={l.topic}>
                  {l.topic}
                </span>
                <span className="cell-sub">
                  {l.type} · {l.tone} · {l.length} · {LANG[l.language] || l.language}
                </span>
              </td>
              <td>
                <StatusBadge status={l.status} />
                {l.error_code && <span className="cell-sub mono">{l.error_code}</span>}
              </td>
              <td className="num nowrap">
                <span className="cell-main">{formatNumber(l.input_tokens + l.output_tokens)}</span>
                <span className="cell-sub">
                  {formatNumber(l.input_tokens)} / {formatNumber(l.output_tokens)}
                </span>
              </td>
              <td className="num nowrap">{formatMs(l.ttft_ms)}</td>
              <td className="num nowrap">{formatMs(l.duration_ms)}</td>
              {showCost && <td className="num nowrap">{formatUSD(l.estimated_cost_usd)}</td>}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
