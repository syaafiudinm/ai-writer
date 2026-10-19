import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { api, setUnauthenticatedHandler } from '../lib/api.js';

const AuthContext = createContext(null);

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(null);

  const loadMe = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      setUser(await api.get('/api/auth/me'));
    } catch (err) {
      setUser(null);
      // 401 = belum login (normal); selain itu server bermasalah.
      if (err.status !== 401 && err.status !== 403) setLoadError(err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    setUnauthenticatedHandler(() => setUser(null));
    loadMe();
    return () => setUnauthenticatedHandler(null);
  }, [loadMe]);

  const login = useCallback(async (email, password) => {
    const u = await api.post('/api/auth/login', { email, password });
    setUser(u);
    return u;
  }, []);

  const logout = useCallback(async () => {
    try {
      await api.post('/api/auth/logout');
    } finally {
      setUser(null);
    }
  }, []);

  const value = useMemo(
    () => ({ user, loading, loadError, retry: loadMe, login, logout }),
    [user, loading, loadError, loadMe, login, logout],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth harus dipakai di dalam AuthProvider');
  return ctx;
}
