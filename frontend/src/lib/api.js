// Klien fetch untuk API Go. Semua error dibalas server sebagai {error, code, fields}.

export class ApiError extends Error {
  constructor(status, body) {
    super(body?.error || `Permintaan gagal (HTTP ${status}).`);
    this.name = 'ApiError';
    this.status = status;
    this.code = body?.code || 'http_error';
    this.fields = body?.fields || {};
  }
}

let onUnauthenticated = null;

// Dipanggil AuthContext agar 401 dari mana pun langsung mengeluarkan user.
export function setUnauthenticatedHandler(fn) {
  onUnauthenticated = fn;
}

export function notifyUnauthenticated() {
  onUnauthenticated?.();
}

export function buildHeaders(method, hasBody) {
  const headers = { Accept: 'application/json' };
  // Server menolak request pengubah data tanpa header ini (proteksi CSRF).
  if (method !== 'GET') headers['X-Requested-With'] = 'fetch';
  if (hasBody) headers['Content-Type'] = 'application/json';
  return headers;
}

export async function readJSON(res) {
  try {
    return await res.json();
  } catch {
    return null;
  }
}

export const networkError = () =>
  new ApiError(0, { error: 'Tidak bisa terhubung ke server. Periksa koneksi lalu coba lagi.', code: 'network_error' });

export async function request(method, path, body, { signal } = {}) {
  let res;
  try {
    res = await fetch(path, {
      method,
      headers: buildHeaders(method, body !== undefined),
      body: body === undefined ? undefined : JSON.stringify(body),
      credentials: 'same-origin',
      signal,
    });
  } catch (err) {
    if (err?.name === 'AbortError') throw err;
    throw networkError();
  }
  if (res.status === 204) return null;
  const data = await readJSON(res);
  if (!res.ok) {
    if (res.status === 401 && path !== '/api/auth/login') notifyUnauthenticated();
    throw new ApiError(res.status, data);
  }
  return data;
}

export const api = {
  get: (path, opts) => request('GET', path, undefined, opts),
  post: (path, body, opts) => request('POST', path, body, opts),
  patch: (path, body, opts) => request('PATCH', path, body, opts),
};

// toQuery membuang nilai kosong agar URL tetap bersih.
export function toQuery(params) {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') q.set(k, String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : '';
}
