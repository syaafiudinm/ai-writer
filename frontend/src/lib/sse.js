// Membaca stream POST /api/generate. EventSource tidak bisa POST, jadi
// body dibaca manual lewat fetch + ReadableStream.

import { ApiError, buildHeaders, networkError, notifyUnauthenticated, readJSON } from './api.js';

// createSSEParser menerima potongan teks sembarang dan memanggil onEvent
// untuk setiap event lengkap ({event, data}). data sudah di-parse dari JSON.
export function createSSEParser(onEvent) {
  let buffer = '';
  const dispatch = (block) => {
    let event = 'message';
    const data = [];
    for (const line of block.split('\n')) {
      if (line === '' || line.startsWith(':')) continue;
      const i = line.indexOf(':');
      const field = i === -1 ? line : line.slice(0, i);
      let value = i === -1 ? '' : line.slice(i + 1);
      if (value.startsWith(' ')) value = value.slice(1);
      if (field === 'event') event = value;
      else if (field === 'data') data.push(value);
    }
    if (data.length === 0) return;
    let parsed;
    try {
      parsed = JSON.parse(data.join('\n'));
    } catch {
      return;
    }
    onEvent({ event, data: parsed });
  };
  return {
    push(chunk) {
      buffer += chunk.replace(/\r\n?/g, '\n');
      let idx;
      while ((idx = buffer.indexOf('\n\n')) !== -1) {
        const block = buffer.slice(0, idx);
        buffer = buffer.slice(idx + 2);
        dispatch(block);
      }
    },
  };
}

/**
 * streamGenerate mengirim brief dan memanggil callback selama teks mengalir.
 * Selesai normal → resolve dengan data event `done`.
 * Error sebelum teks pertama (JSON) atau event `error` → reject ApiError.
 * Dibatalkan lewat signal → reject AbortError.
 */
export async function streamGenerate(brief, { signal, onMeta, onText } = {}) {
  let res;
  try {
    res = await fetch('/api/generate', {
      method: 'POST',
      headers: { ...buildHeaders('POST', true), Accept: 'text/event-stream' },
      body: JSON.stringify(brief),
      credentials: 'same-origin',
      signal,
    });
  } catch (err) {
    if (err?.name === 'AbortError') throw err;
    throw networkError();
  }

  const type = res.headers.get('Content-Type') || '';
  if (!res.ok || !type.includes('text/event-stream')) {
    const body = await readJSON(res);
    if (res.status === 401) notifyUnauthenticated();
    throw new ApiError(res.ok ? 502 : res.status, body);
  }

  let done = null;
  let failure = null;
  const parser = createSSEParser(({ event, data }) => {
    if (event === 'meta') onMeta?.(data);
    else if (event === 'message' && typeof data.text === 'string') onText?.(data.text);
    else if (event === 'done') done = data;
    else if (event === 'error') failure = new ApiError(200, data);
  });

  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
  try {
    for (;;) {
      const { value, done: finished } = await reader.read();
      if (finished) break;
      parser.push(value);
      if (failure) throw failure;
    }
  } catch (err) {
    if (err instanceof ApiError || err?.name === 'AbortError') throw err;
    throw new ApiError(0, { error: 'Koneksi terputus saat draf sedang ditulis.', code: 'stream_interrupted' });
  } finally {
    reader.releaseLock?.();
  }
  if (failure) throw failure;
  if (!done) {
    throw new ApiError(0, { error: 'Koneksi terputus sebelum draf selesai.', code: 'stream_interrupted' });
  }
  return done;
}
