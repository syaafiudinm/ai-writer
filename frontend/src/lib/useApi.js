import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from './api.js';

// useApi mengambil GET path dan menyimpan data sebelumnya selama memuat ulang,
// sehingga tampilan tidak berkedip saat filter berubah.
export function useApi(path) {
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(true);
  const [nonce, setNonce] = useState(0);
  const current = useRef(null);

  useEffect(() => {
    if (!path) return undefined;
    const ac = new AbortController();
    current.current = ac;
    setLoading(true);
    setError(null);
    api
      .get(path, { signal: ac.signal })
      .then((d) => {
        if (!ac.signal.aborted) setData(d);
      })
      .catch((err) => {
        if (!ac.signal.aborted) setError(err);
      })
      .finally(() => {
        if (!ac.signal.aborted) setLoading(false);
      });
    return () => ac.abort();
  }, [path, nonce]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  return { data, error, loading, reload, setData };
}
