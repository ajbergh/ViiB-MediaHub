// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ load: vi.fn(), metadata: vi.fn(), seek: vi.fn(), state: { djDeckA: { track: { id: 'a' }, duration: 10 } } }));
vi.mock('../../../store', () => ({ useStore: Object.assign((select: any) => select(mocks.state), { getState: () => mocks.state }) }));
vi.mock('../../../hooks/useDJAudioEngine', () => ({ useDJAudioEngineActions: () => ({ seek: mocks.seek }) }));
vi.mock('../../../services/api', () => ({ api: { getTrackAnalysisFeature: mocks.metadata } }));
vi.mock('../../../services/trackThreeBand', () => ({ loadTrackThreeBand: mocks.load }));
import { DJDeckOverview } from './DJDeckOverview';
it('shows current Spotify bands first and falls back to amplitude for mismatched or unavailable bands', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 vi.spyOn(HTMLCanvasElement.prototype,'getContext').mockReturnValue(null);
 mocks.metadata.mockResolvedValue({ sourceFingerprint: 'fp' });
 mocks.load.mockResolvedValue({ data: { source: 'spotify', sourceFingerprint: 'fp', durationSeconds: 10, low: [1], mid: [0.5], high: [0.25] } });
 const host = document.createElement('div'); const root = createRoot(host);
 try {
  await act(async () => root.render(<DJDeckOverview deck="A" localBands />));
  expect(host.textContent).toContain('Spotify bands - Low / Mid / High');
  mocks.state.djDeckA.duration = 20;
  await act(async () => root.render(<DJDeckOverview deck="A" localBands visibleSeconds={8} />));
  expect(host.textContent).toContain('different timeline');
  mocks.state.djDeckA.track = { id: 'replacement' };
  mocks.load.mockResolvedValue({ data: null });
  await act(async () => root.render(<DJDeckOverview deck="A" localBands visibleSeconds={10} />));
  expect(host.textContent).toContain('Amplitude overview');
 } finally { await act(async () => root.unmount()); vi.restoreAllMocks(); }
});

it('draws all three lanes and preserves playback-time seeking and cue markers', async () => {
 const ctx = { setTransform: vi.fn(), fillRect: vi.fn(), beginPath: vi.fn(), rect: vi.fn(), fill: vi.fn(), drawImage: vi.fn(), strokeRect: vi.fn(), fillText: vi.fn(), fillStyle: '', strokeStyle: '', lineWidth: 1, font: '' };
 vi.spyOn(HTMLCanvasElement.prototype,'getContext').mockReturnValue(ctx as any);
 vi.spyOn(HTMLCanvasElement.prototype,'clientWidth','get').mockReturnValue(100);
 vi.spyOn(HTMLCanvasElement.prototype,'clientHeight','get').mockReturnValue(30);
 vi.spyOn(HTMLCanvasElement.prototype,'getBoundingClientRect').mockReturnValue({ left: 0, width: 100 } as DOMRect);
 Object.assign(mocks.state.djDeckA,{ track: { id: 'draw' }, duration: 10, position: 2, waveformPeaks: null, loop: { start: 0,end: 0,enabled: false }, hotCues: [{ position: 5,color: 'cue',origin: 'manual' }] });
 mocks.metadata.mockResolvedValue({ sourceFingerprint: 'fp' });
 mocks.load.mockResolvedValue({ data: { source: 'spotify', sourceFingerprint: 'fp', durationSeconds: 10, low: [1], mid: [0.5], high: [0.25] } });
 const host = document.createElement('div'); const root = createRoot(host);
 try {
  await act(async () => root.render(<DJDeckOverview deck="A" localBands visibleSeconds={4} />));
  expect(ctx.rect.mock.calls.some(call => call[1] < 10)).toBe(true);
  expect(ctx.rect.mock.calls.some(call => call[1] >= 10 && call[1] < 20)).toBe(true);
  expect(ctx.rect.mock.calls.some(call => call[1] >= 20)).toBe(true);
  expect(ctx.fillRect).toHaveBeenCalledWith(50,0,2,30);
  await act(async () => host.querySelector('canvas')!.dispatchEvent(new MouseEvent('click',{ bubbles:true,clientX:75 })));
  expect(mocks.seek).toHaveBeenCalledWith('A',7.5);
 } finally { await act(async () => root.unmount()); vi.restoreAllMocks(); }
});

it('retries a transient provider/local band read failure from an accessible action', async () => {
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
 vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
 mocks.state.djDeckA.track = { id: 'retry' };
 mocks.state.djDeckA.duration = 10;
 mocks.metadata.mockResolvedValue({ sourceFingerprint: 'retry-fp' });
 mocks.load.mockResolvedValueOnce({ data: null, localError: new Error('transient read failure') }).mockResolvedValueOnce({
  data: { source: 'local', sourceFingerprint: 'retry-fp', durationSeconds: 10, low: [1], mid: [0.5], high: [0.25] },
 });
 const host = document.createElement('div');
 const root = createRoot(host);
 try {
  await act(async () => root.render(<DJDeckOverview deck="A" localBands />));
  expect(host.textContent).toContain('Amplitude overview remains available.');
  const retry = host.querySelector('button');
  expect(retry?.textContent).toBe('Retry three-band');
  await act(async () => retry?.click());
  expect(host.textContent).toContain('Local bands - Low / Mid / High');
  expect(mocks.load).toHaveBeenCalledTimes(2);
 } finally {
  await act(async () => root.unmount());
  vi.restoreAllMocks();
 }
});