import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';

afterEach(() => vi.unstubAllGlobals());

describe('source-bound key edits', () => {
  it('preserves tonic zero and sends the loaded media revision when saving', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      if (headers.get('If-Match') !== '"source-v1"') return new Response('source changed', { status: 412 });
      return new Response(JSON.stringify({ keyTonic: 0, keyMode: 'major', keySource: 'manual', sourceFingerprint: 'source-v1' }), { status: 200 });
    });
    vi.stubGlobal('fetch', fetchMock);
    const saved = await api.updateTrackKey('song / 1', { tonic: 0, mode: 'major' }, 'source-v1');
    expect(saved.keyTonic).toBe(0);
    expect(saved.sourceFingerprint).toBe('source-v1');
    expect(String(fetchMock.mock.calls[0][0])).toContain('song%20%2F%201/key');
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({ tonic: 0, mode: 'major' });
  });

  it('passes source rejection back to the editor on reset', async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify({ error: 'song source changed' }), { status: 412 }));
    vi.stubGlobal('fetch', fetchMock);
    await expect(api.resetTrackKey('song', 'source-v1')).rejects.toThrow();
    expect(fetchMock.mock.calls[0][1]?.method).toBe('DELETE');
    expect(new Headers(fetchMock.mock.calls[0][1]?.headers).get('If-Match')).toBe('"source-v1"');
  });
});
