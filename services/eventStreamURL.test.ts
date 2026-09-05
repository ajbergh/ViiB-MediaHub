import { afterEach, describe, expect, it, vi } from 'vitest';
import { getEventStreamURL, resetEventStreamURLCacheForTests } from './eventStreamURL';

afterEach(() => {
  resetEventStreamURLCacheForTests();
  vi.unstubAllGlobals();
});

describe('getEventStreamURL', () => {
  it('keeps a relative URL outside the native app', async () => {
    await expect(getEventStreamURL('/api/library/events')).resolves.toBe('/api/library/events');
  });

  it('uses Wails loopback server for native event streams', async () => {
    vi.stubGlobal('window', {
      go: { main: { App: { GetServerURL: vi.fn().mockResolvedValue('http://127.0.0.1:34115/') } } },
    });

    await expect(getEventStreamURL('/api/v2/library/events?since=12'))
      .resolves.toBe('http://127.0.0.1:34115/api/v2/library/events?since=12');
  });
});
