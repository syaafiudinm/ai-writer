import { useState } from 'react';
import { NavLink, Outlet, useNavigate } from 'react-router';
import { useAuth } from '../context/AuthContext.jsx';

const ADMIN_LINKS = [
  { to: '/admin', label: 'Dashboard', end: true },
  { to: '/admin/logs', label: 'Log' },
  { to: '/admin/users', label: 'Pengguna' },
  { to: '/admin/audit', label: 'Audit' },
];

export default function Layout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);

  async function handleLogout() {
    setBusy(true);
    try {
      await logout();
    } catch {
      // Sesi lokal tetap dibersihkan oleh AuthContext.
    } finally {
      setBusy(false);
      navigate('/login', { replace: true });
    }
  }

  return (
    <div className="shell">
      <a className="skip-link" href="#konten">
        Lewati ke konten
      </a>
      <header className="masthead">
        <div className="masthead__inner">
          <NavLink to="/" className="brand">
            <span className="brand__name">AI Writer</span>
            <span className="brand__tag">Ruang redaksi</span>
          </NavLink>
          <nav className="nav" aria-label="Navigasi utama">
            <NavLink to="/" end className="nav__link">
              Tulis
            </NavLink>
            {user?.role === 'admin' &&
              ADMIN_LINKS.map((l) => (
                <NavLink key={l.to} to={l.to} end={l.end} className="nav__link">
                  {l.label}
                </NavLink>
              ))}
          </nav>
          <div className="masthead__user">
            <span className="masthead__name" title={user?.email}>
              {user?.name}
              {user?.role === 'admin' && <span className="tag">admin</span>}
            </span>
            <button type="button" className="btn btn--sm" onClick={handleLogout} disabled={busy}>
              {busy ? 'Keluar…' : 'Keluar'}
            </button>
          </div>
        </div>
      </header>
      <div id="konten" tabIndex={-1}>
        <Outlet />
      </div>
    </div>
  );
}
