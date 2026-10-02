// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ state: {} as any, getChanges: vi.fn() }));
vi.mock('../store', () => ({ useStore: Object.assign(
  (selector: any) => selector(mocks.state),
  { getState: () => mocks.state, subscribe: () => () => {}, setState: (patch: any) => Object.assign(mocks.state, patch) },
) }));
vi.mock('../lib/smartMix', () => ({ generateSmartMixes: () => [] }));
vi.mock('../services/eventStreamURL', () => ({ getEventStreamURL: async (url: string) => url }));
vi.mock('../services/libraryV2', () => ({
  LibraryResnapshotRequired: class extends Error {},
  libraryV2: { getSnapshot: async () => ({ songs: [], revision: 1 }), getChanges: mocks.getChanges, eventURL: () => '/revisions' },
}));
import LibraryEventListener from './LibraryEventListener';

it('holds scan loading until the completion event has synchronized the catalog', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  const sources: FakeEventSource[] = [];
  class FakeEventSource {
    static CLOSED = 2;
    readyState = 1;
    onmessage: ((event: MessageEvent) => void) | null = null;
    onerror = null;
    onopen = null;
    constructor(public url: string) { sources.push(this); }
    addEventListener() {}
    close() {}
  }
  vi.stubGlobal('EventSource', FakeEventSource);
  let finish!: (value: any) => void;
  mocks.getChanges.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
  mocks.state = {
    backendAvailable: true, songs: [], isScanning: true,
    setScanning: (value: boolean) => { mocks.state.isScanning = value; },
    setScanProgress: vi.fn(), refreshLibrary: vi.fn().mockResolvedValue(undefined),
    setEnrichmentStatus: vi.fn(), addLog: vi.fn(),
  };
  const root = createRoot(document.createElement('div'));
  try {
    await act(async () => root.render(<LibraryEventListener />));
    const legacy = sources.find(source => source.url === '/api/library/events')!;
    await act(async () => legacy.onmessage!(new MessageEvent('message', { data: JSON.stringify({ type: 'scan_complete', message: '' }) })));
    expect(mocks.state.isScanning).toBe(true);
    const song = { id: 'song', title: 'Track', artist: 'Artist', album: 'Album', duration: 1, url: '/song', addedAt: 1 };
    await act(async () => finish({ revision: 2, changes: [{ songId: 'song', operation: 'upsert', revision: 2 }], songs: [song] }));
    expect(mocks.state.songs).toHaveLength(1);
    expect(mocks.state.isScanning).toBe(false);
  } finally { await act(async () => root.unmount()); vi.unstubAllGlobals(); }
});
