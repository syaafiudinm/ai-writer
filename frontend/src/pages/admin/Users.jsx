import { useState } from 'react';
import { api } from '../../lib/api.js';
import { useApi } from '../../lib/useApi.js';
import { formatDateTime } from '../../lib/format.js';
import { useAuth } from '../../context/AuthContext.jsx';
import { ErrorAlert, Modal } from '../../components/admin/ui.jsx';

function randomPassword() {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789';
  const buf = new Uint32Array(14);
  crypto.getRandomValues(buf);
  return Array.from(buf, (n) => chars[n % chars.length]).join('');
}

function FieldError({ id, msg }) {
  return msg ? (
    <p id={id} className="field__error">
      {msg}
    </p>
  ) : null;
}

function PasswordField({ id, value, onChange, error, label = 'Password' }) {
  return (
    <div className="field">
      <div className="field__row">
        <label htmlFor={id}>{label}</label>
        <button type="button" className="link-btn" onClick={() => onChange(randomPassword())}>
          Buat acak
        </button>
      </div>
      <input
        id={id}
        type="text"
        autoComplete="new-password"
        spellCheck={false}
        className="mono"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={!!error}
        aria-describedby={`${id}-hint${error ? ` ${id}-err` : ''}`}
        required
      />
      <p id={`${id}-hint`} className="hint">
        Minimal 8 karakter. Sampaikan ke pemilik akun lewat jalur aman.
      </p>
      <FieldError id={`${id}-err`} msg={error} />
    </div>
  );
}

function CreateUserForm({ onCreated, onCancel }) {
  const [form, setForm] = useState({ name: '', email: '', password: '', role: 'user' });
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const f = error?.fields || {};
  const set = (k) => (v) => setForm({ ...form, [k]: v });

  async function submit(e) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      onCreated(await api.post('/api/admin/users', form));
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} noValidate>
      {error && !Object.keys(f).length && error.code !== 'email_taken' && (
        <p className="alert alert--error" role="alert">
          {error.message}
        </p>
      )}
      <div className="field">
        <label htmlFor="new-name">Nama</label>
        <input id="new-name" value={form.name} onChange={(e) => set('name')(e.target.value)} aria-invalid={!!f.name} required autoFocus />
        <FieldError msg={f.name} />
      </div>
      <div className="field">
        <label htmlFor="new-email">Email</label>
        <input
          id="new-email"
          type="email"
          autoComplete="off"
          value={form.email}
          onChange={(e) => set('email')(e.target.value)}
          aria-invalid={!!(f.email || error?.code === 'email_taken')}
          required
        />
        <FieldError msg={f.email || (error?.code === 'email_taken' ? error.message : null)} />
      </div>
      <PasswordField id="new-password" value={form.password} onChange={set('password')} error={f.password} />
      <fieldset className="field">
        <legend>Peran</legend>
        <div className="segmented">
          {['user', 'admin'].map((r) => (
            <label key={r} className={`segmented__opt${form.role === r ? ' is-checked' : ''}`}>
              <input type="radio" name="new-role" value={r} checked={form.role === r} onChange={() => set('role')(r)} />
              <span className="segmented__label">{r === 'admin' ? 'Admin' : 'Penulis'}</span>
            </label>
          ))}
        </div>
      </fieldset>
      <div className="modal__actions">
        <button type="button" className="btn btn--ghost" onClick={onCancel}>
          Batal
        </button>
        <button type="submit" className="btn btn--ink" disabled={busy}>
          {busy ? 'Menyimpan…' : 'Buat akun'}
        </button>
      </div>
    </form>
  );
}

function ResetPasswordForm({ user, onDone, onCancel }) {
  const [password, setPassword] = useState('');
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);

  async function submit(e) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      onDone(await api.patch(`/api/admin/users/${user.id}`, { password }));
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} noValidate>
      <p>
        Password baru untuk <strong>{user.name}</strong>. Semua sesi aktifnya akan dicabut.
      </p>
      {error && !error.fields.password && (
        <p className="alert alert--error" role="alert">
          {error.message}
        </p>
      )}
      <PasswordField id="reset-password" label="Password baru" value={password} onChange={setPassword} error={error?.fields.password} />
      <div className="modal__actions">
        <button type="button" className="btn btn--ghost" onClick={onCancel}>
          Batal
        </button>
        <button type="submit" className="btn btn--ink" disabled={busy}>
          {busy ? 'Menyimpan…' : 'Simpan password'}
        </button>
      </div>
    </form>
  );
}

function ConfirmForm({ message, confirmLabel, onConfirm, onCancel }) {
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  async function confirm() {
    setBusy(true);
    setError(null);
    try {
      await onConfirm();
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  }
  return (
    <div>
      <p>{message}</p>
      {error && (
        <p className="alert alert--error" role="alert">
          {error.message}
        </p>
      )}
      <div className="modal__actions">
        <button type="button" className="btn btn--ghost" onClick={onCancel}>
          Batal
        </button>
        <button type="button" className="btn btn--ink" onClick={confirm} disabled={busy} autoFocus>
          {busy ? 'Menyimpan…' : confirmLabel}
        </button>
      </div>
    </div>
  );
}

export default function Users() {
  const { user: me } = useAuth();
  const { data, error, loading, reload, setData } = useApi('/api/admin/users');
  const [dialog, setDialog] = useState(null); // {kind, user}
  const [notice, setNotice] = useState(null);

  const close = () => setDialog(null);
  const replace = (u) => setData((d) => ({ items: d.items.map((x) => (x.id === u.id ? u : x)) }));

  async function patch(u, body, msg) {
    const updated = await api.patch(`/api/admin/users/${u.id}`, body);
    replace(updated);
    setNotice(msg);
    close();
  }

  const items = data?.items || [];
  const admins = items.filter((u) => u.role === 'admin' && u.is_active).length;

  let modal = null;
  if (dialog?.kind === 'create') {
    modal = (
      <CreateUserForm
        onCancel={close}
        onCreated={(u) => {
          setData((d) => ({ items: [...d.items, u] }));
          setNotice(`Akun ${u.name} dibuat.`);
          close();
        }}
      />
    );
  } else if (dialog?.kind === 'password') {
    modal = (
      <ResetPasswordForm
        user={dialog.user}
        onCancel={close}
        onDone={(u) => {
          replace(u);
          setNotice(`Password ${u.name} direset.`);
          close();
        }}
      />
    );
  } else if (dialog?.kind === 'role') {
    const u = dialog.user;
    const to = u.role === 'admin' ? 'user' : 'admin';
    modal = (
      <ConfirmForm
        message={
          to === 'admin'
            ? `Jadikan ${u.name} admin? Admin bisa melihat semua log dan mengelola akun.`
            : `Turunkan ${u.name} menjadi penulis biasa?`
        }
        confirmLabel={to === 'admin' ? 'Jadikan admin' : 'Turunkan'}
        onCancel={close}
        onConfirm={() => patch(u, { role: to }, `Peran ${u.name} diubah ke ${to === 'admin' ? 'admin' : 'penulis'}.`)}
      />
    );
  } else if (dialog?.kind === 'active') {
    const u = dialog.user;
    modal = (
      <ConfirmForm
        message={
          u.is_active
            ? `Nonaktifkan ${u.name}? Semua sesinya langsung dicabut dan ia tidak bisa login.`
            : `Aktifkan kembali ${u.name}?`
        }
        confirmLabel={u.is_active ? 'Nonaktifkan' : 'Aktifkan'}
        onCancel={close}
        onConfirm={() =>
          patch(u, { is_active: !u.is_active }, `${u.name} ${u.is_active ? 'dinonaktifkan' : 'diaktifkan'}.`)
        }
      />
    );
  }

  const TITLES = { create: 'Akun baru', password: 'Reset password', role: 'Ubah peran', active: 'Status akun' };

  return (
    <main className="page">
      <header className="page-head page-head--row">
        <div>
          <p className="kicker">Admin</p>
          <h1 className="page-title">Pengguna</h1>
          <p className="lede">Akun hanya dibuat oleh admin. Minimal satu admin aktif harus tersisa.</p>
        </div>
        <button type="button" className="btn btn--ink" onClick={() => setDialog({ kind: 'create' })}>
          + Akun baru
        </button>
      </header>

      {notice && (
        <p className="alert alert--ok" role="status">
          {notice}
          <button type="button" className="link-btn" onClick={() => setNotice(null)}>
            Tutup
          </button>
        </p>
      )}
      <ErrorAlert error={error} onRetry={reload} />

      {data ? (
        <section className={`card card--flush${loading ? ' is-refreshing' : ''}`}>
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th scope="col">Nama</th>
                  <th scope="col">Peran</th>
                  <th scope="col">Status</th>
                  <th scope="col">Login terakhir</th>
                  <th scope="col">Dibuat</th>
                  <th scope="col">
                    <span className="sr-only">Aksi</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {items.map((u) => {
                  const self = u.id === me?.id;
                  const lastAdmin = u.role === 'admin' && u.is_active && admins <= 1;
                  const lockReason = self ? 'Tidak bisa mengubah akun sendiri' : lastAdmin ? 'Admin aktif terakhir' : undefined;
                  return (
                    <tr key={u.id} className={u.is_active ? undefined : 'is-inactive'}>
                      <td>
                        <span className="cell-main">
                          {u.name} {self && <span className="tag">Anda</span>}
                        </span>
                        <span className="cell-sub">{u.email}</span>
                      </td>
                      <td>{u.role === 'admin' ? <span className="tag tag--ink">admin</span> : 'penulis'}</td>
                      <td>
                        <span className={`status status--${u.is_active ? 'success' : 'cancelled'}`}>
                          <span aria-hidden="true" className="status__icon">
                            {u.is_active ? '●' : '○'}
                          </span>
                          {u.is_active ? 'Aktif' : 'Nonaktif'}
                        </span>
                      </td>
                      <td className="nowrap">{formatDateTime(u.last_login_at)}</td>
                      <td className="nowrap">{formatDateTime(u.created_at)}</td>
                      <td>
                        <div className="row-actions">
                          <button
                            type="button"
                            className="btn btn--xs"
                            onClick={() => setDialog({ kind: 'role', user: u })}
                            disabled={u.role === 'admin' && !!lockReason}
                            title={u.role === 'admin' ? lockReason : undefined}
                          >
                            {u.role === 'admin' ? 'Turunkan' : 'Jadikan admin'}
                          </button>
                          <button
                            type="button"
                            className="btn btn--xs"
                            onClick={() => setDialog({ kind: 'active', user: u })}
                            disabled={u.is_active && !!lockReason}
                            title={u.is_active ? lockReason : undefined}
                          >
                            {u.is_active ? 'Nonaktifkan' : 'Aktifkan'}
                          </button>
                          <button type="button" className="btn btn--xs" onClick={() => setDialog({ kind: 'password', user: u })}>
                            Reset password
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>
      ) : (
        !error && <p className="muted">Memuat pengguna…</p>
      )}

      <Modal open={!!dialog} title={dialog ? TITLES[dialog.kind] : ''} onClose={close}>
        {modal}
      </Modal>
    </main>
  );
}
