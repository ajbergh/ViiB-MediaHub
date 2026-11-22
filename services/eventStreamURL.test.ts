/** Tests and fixtures for event Stream URL behavior. */

import { afterEach, describe, expect, it, vi } from 'vitest';
import { getEventStreamURL, resetEventStreamURLCacheForTests } from './eventStreamURL';

afterEach(() => {
  resetEventStreamURLCacheForTests();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
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
  it('recovers from a transient binding failure before starting either stream', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    const getURL = vi.fn().mockRejectedValueOnce(new Error('binding unavailable'))
      .mockResolvedValue('http://127.0.0.1:34115/');
    vi.stubGlobal('window', {go: {main: {App: {GetServerURL: getURL}}}});

    await expect(Promise.all([
      getEventStreamURL('/api/library/events'),
      getEventStreamURL('/api/spotify/downloads/events'),
    ])).resolves.toEqual([
      'http://127.0.0.1:34115/api/library/events',
      'http://127.0.0.1:34115/api/spotify/downloads/events',
    ]);
    expect(getURL).toHaveBeenCalledTimes(2);
    await expect(getEventStreamURL('/api/library/events'))
      .resolves.toBe('http://127.0.0.1:34115/api/library/events');
    await expect(getEventStreamURL('/api/spotify/downloads/events'))
      .resolves.toBe('http://127.0.0.1:34115/api/spotify/downloads/events');
    expect(getURL).toHaveBeenCalledTimes(2);
  });

  it('does not cache an empty or non-loopback binding result', async () => {
    const getURL = vi.fn().mockResolvedValueOnce('').mockResolvedValueOnce('https://example.com')
      .mockResolvedValue('http://127.0.0.1:34115/');
    vi.stubGlobal('window', {go: {main: {App: {GetServerURL: getURL}}}});

    await expect(getEventStreamURL('/api/library/events')).resolves.toBe('/api/library/events');
    await expect(getEventStreamURL('/api/library/events'))
      .resolves.toBe('http://127.0.0.1:34115/api/library/events');
  });

});
