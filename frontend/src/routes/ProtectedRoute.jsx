import { Link, Navigate, Outlet, useLocation } from 'react-router';
import { useAuth } from '../context/AuthContext.jsx';

export function Splash({ children = 'Memuat…' }) {
  return (
    <div className="splash" role="status">
      <span className="splash__mark">AI Writer</span>
      <span>{children}</span>
    </div>
  );
}

// ProtectedRoute mengarahkan tamu ke /login; role membatasi halaman admin.
export default function ProtectedRoute({ role }) {
  const { user, loading, loadError, retry } = useAuth();
  const location = useLocation();

  if (loading) return <Splash />;
  if (loadError) {
    return (
      <Splash>
        <span className="alert alert--error">{loadError.message}</span>
        <button type="button" className="btn" onClick={retry}>
          Coba lagi
        </button>
      </Splash>
    );
  }
  if (!user) return <Navigate to="/login" replace state={{ from: location }} />;
  if (role && user.role !== role) {
    return (
      <main className="page page--narrow">
        <section className="card">
          <h1 className="page-title">Tidak ada akses</h1>
          <p>Halaman ini hanya untuk admin.</p>
          <Link className="btn" to="/">
            Kembali ke penulis
          </Link>
        </section>
      </main>
    );
  }
  return <Outlet />;
}
