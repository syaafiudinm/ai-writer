import { Link, Route, Routes } from 'react-router';
import Layout from './components/Layout.jsx';
import ProtectedRoute from './routes/ProtectedRoute.jsx';
import Login from './pages/Login.jsx';
import Generator from './pages/Generator.jsx';
import Dashboard from './pages/admin/Dashboard.jsx';
import Logs from './pages/admin/Logs.jsx';
import Users from './pages/admin/Users.jsx';
import Audit from './pages/admin/Audit.jsx';

function NotFound() {
  return (
    <main className="page page--narrow">
      <section className="card">
        <h1 className="page-title">Halaman tidak ditemukan</h1>
        <p>Alamat yang dibuka tidak ada.</p>
        <Link className="btn" to="/">
          Ke halaman utama
        </Link>
      </section>
    </main>
  );
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route element={<ProtectedRoute />}>
        <Route element={<Layout />}>
          <Route index element={<Generator />} />
          <Route path="admin" element={<ProtectedRoute role="admin" />}>
            <Route index element={<Dashboard />} />
            <Route path="logs" element={<Logs />} />
            <Route path="users" element={<Users />} />
            <Route path="audit" element={<Audit />} />
          </Route>
          <Route path="*" element={<NotFound />} />
        </Route>
      </Route>
    </Routes>
  );
}
