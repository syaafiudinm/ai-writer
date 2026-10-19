import { afterEach, describe, expect, it, vi } from 'vitest';
import { createSSEParser, streamGenerate } from './sse.js';

function sseResponse(chunks, init = {}) {
  const enc = new TextEncoder();
  const body = new ReadableStream({
    start(c) {
      for (const ch of chunks) c.enqueue(enc.encode(ch));
      c.close();
    },
  });
  return new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream; charset=utf-8' }, ...init });
}

afterEach(() => vi.unstubAllGlobals());

describe('createSSEParser', () => {
  it('menyusun event yang terpotong di tengah chunk dan CRLF', () => {
    const events = [];
    const p = createSSEParser((e) => events.push(e));
    p.push('event: meta\ndata: {"model":"m"}\n\nda');
    p.push('ta: {"text":"Halo "}\r\n\r\ndata: {"text":"dunia"}\n');
    expect(events).toHaveLength(2);
    p.push('\nevent: done\ndata: {"stop_reason":"stop"}\n\n');
    expect(events).toEqual([
      { event: 'meta', data: { model: 'm' } },
      { event: 'message', data: { text: 'Halo ' } },
      { event: 'message', data: { text: 'dunia' } },
      { event: 'done', data: { stop_reason: 'stop' } },
    ]);
  });

  it('mengabaikan komentar dan data yang bukan JSON', () => {
    const events = [];
    const p = createSSEParser((e) => events.push(e));
    p.push(': ping\n\ndata: bukan-json\n\ndata: {"text":"ok"}\n\n');
    expect(events).toEqual([{ event: 'message', data: { text: 'ok' } }]);
  });
});

describe('streamGenerate', () => {
  it('mengirim header CSRF dan mengalirkan teks sampai done', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      sseResponse([
        'event: meta\ndata: {"model":"gemini"}\n\n',
        'data: {"text":"# Judul"}\n\ndata: {"text":"\\n\\nIsi"}\n\n',
        'event: done\ndata: {"stop_reason":"stop","input_tokens":10,"output_tokens":20}\n\n',
      ]),
    );
    vi.stubGlobal('fetch', fetchMock);
    let text = '';
    let model = null;
    const done = await streamGenerate({ type: 'berita' }, { onMeta: (m) => (model = m.model), onText: (t) => (text += t) });
    expect(text).toBe('# Judul\n\nIsi');
    expect(model).toBe('gemini');
    expect(done.output_tokens).toBe(20);
    const [, init] = fetchMock.mock.calls[0];
    expect(init.headers['X-Requested-With']).toBe('fetch');
    expect(init.method).toBe('POST');
  });

  it('error JSON sebelum stream → ApiError dengan fields', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: 'Topik wajib diisi.', code: 'invalid_input', fields: { topic: 'x' } }), {
          status: 400,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    );
    await expect(streamGenerate({})).rejects.toMatchObject({ status: 400, code: 'invalid_input', fields: { topic: 'x' } });
  });

  it('event error di tengah stream → reject dengan kode server, teks sebelumnya tetap terkirim', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        sseResponse(['data: {"text":"awal"}\n\n', 'event: error\ndata: {"error":"Waktu habis","code":"timeout"}\n\n']),
      ),
    );
    let text = '';
    await expect(streamGenerate({}, { onText: (t) => (text += t) })).rejects.toMatchObject({ code: 'timeout' });
    expect(text).toBe('awal');
  });

  it('stream berakhir tanpa done → stream_interrupted', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(sseResponse(['data: {"text":"a"}\n\n'])));
    await expect(streamGenerate({})).rejects.toMatchObject({ code: 'stream_interrupted' });
  });
});
