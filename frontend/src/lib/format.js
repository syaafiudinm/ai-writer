// Format angka & tanggal untuk tampilan (id-ID).

const nf = new Intl.NumberFormat('id-ID');
const compactNf = new Intl.NumberFormat('id-ID', { notation: 'compact', maximumFractionDigits: 1 });

export const formatNumber = (n) => (n == null ? '—' : nf.format(n));

export const formatCompact = (n) => (n == null ? '—' : Math.abs(n) < 1000 ? nf.format(n) : compactNf.format(n));

export function formatUSD(n) {
  if (n == null) return '—';
  const digits = n !== 0 && Math.abs(n) < 0.01 ? 4 : 2;
  return `US$${n.toLocaleString('id-ID', { minimumFractionDigits: digits, maximumFractionDigits: digits })}`;
}

export const formatPercent = (r) =>
  r == null ? '—' : `${(r * 100).toLocaleString('id-ID', { maximumFractionDigits: 1 })}%`;

export function formatMs(ms) {
  if (ms == null) return '—';
  if (ms < 1000) return `${nf.format(ms)} md`;
  return `${(ms / 1000).toLocaleString('id-ID', { maximumFractionDigits: 1 })} dtk`;
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des'];

// Waktu dari server sudah dalam zona TIMEZONE server (mis. +08:00). Bagian
// lokalnya dibaca langsung agar tampilan sama dengan pembagian hari di dashboard,
// apa pun zona waktu browser.
export function formatDateTime(iso) {
  if (!iso) return '—';
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(iso);
  if (!m) return iso;
  const [, y, mo, d, h, mi] = m;
  return `${Number(d)} ${MONTHS[Number(mo) - 1]} ${y}, ${h}.${mi}`;
}

export function formatDate(ymd) {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(ymd || '');
  if (!m) return ymd || '—';
  return `${Number(m[3])} ${MONTHS[Number(m[2]) - 1]} ${m[1]}`;
}

export function formatShortDate(ymd) {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(ymd || '');
  if (!m) return ymd || '';
  return `${Number(m[3])} ${MONTHS[Number(m[2]) - 1]}`;
}

// toYMD memakai tanggal lokal browser (bukan UTC).
export function toYMD(date) {
  const p = (n) => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${p(date.getMonth() + 1)}-${p(date.getDate())}`;
}

export function lastNDays(n, today = new Date()) {
  const from = new Date(today.getFullYear(), today.getMonth(), today.getDate() - (n - 1));
  return { from: toYMD(from), to: toYMD(today) };
}
