import { useState } from 'react';
import { Navigate, useLocation, useNavigate } from 'react-router';
import { useAuth } from '../context/AuthContext.jsx';
import { Splash } from '../routes/ProtectedRoute.jsx';

export default function Login() {
  const { user, loading, login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const from = location.state?.from?.pathname || '/';

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState(null);
  const [submitting, setSubmitting] = useState(false);

  if (loading) return <Splash />;
  if (user) return <Navigate to={from} replace />;

  async function handleSubmit(e) {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await login(email.trim(), password);
      navigate(from, { replace: true });
    } catch (err) {
      setError(err);
      setSubmitting(false);
    }
  }

  const fields = error?.fields || {};

  return (
    <main className="login">
      <div className="login__masthead">
        <p className="login__kicker">Edisi Redaksi</p>
        <h1 className="login__title">AI Writer</h1>
        <p className="login__lede">Draf berita dan artikel, ditulis cepat, diperiksa manusia.</p>
      </div>

      <form className="card login__card" onSubmit={handleSubmit} noValidate>
        <h2 className="card__title">Masuk</h2>

        {error && !Object.keys(fields).length && (
          <p className="alert alert--error" role="alert">
            {error.message}
          </p>
        )}

        <div className="field">
          <label htmlFor="email">Email</label>
          <input
            id="email"
            type="email"
            autoComplete="username"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            aria-invalid={!!fields.email}
            aria-describedby={fields.email ? 'email-err' : undefined}
            required
            autoFocus
          />
          {fields.email && (
            <p id="email-err" className="field__error">
              {fields.email}
            </p>
          )}
        </div>

        <div className="field">
          <label htmlFor="password">Password</label>
          <input
            id="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            aria-invalid={!!fields.password}
            aria-describedby={fields.password ? 'password-err' : undefined}
            required
          />
          {fields.password && (
            <p id="password-err" className="field__error">
              {fields.password}
            </p>
          )}
        </div>

        <button type="submit" className="btn btn--ink btn--block" disabled={submitting}>
          {submitting ? 'Memeriksa…' : 'Masuk'}
        </button>
        <p className="hint">Akun dibuat oleh admin. Lupa password? Hubungi admin.</p>
      </form>
    </main>
  );
}
