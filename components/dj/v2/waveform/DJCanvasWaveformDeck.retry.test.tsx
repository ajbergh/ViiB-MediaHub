// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
const mocks = vi.hoisted(() => ({ metadata: vi.fn(), load: vi.fn(), state: { spotifySessionGeneration: 1, djDeckA: { track: { id: 'retry' } }, djDeckB: { track: undefined } } }));
vi.mock('../../../../store', () => ({ useStore: Object.assign((select: any) => select(mocks.state), { getState: () => mocks.state }) }));
vi.mock('../../../../services/api', () => ({ api: { getTrackAnalysisFeature: mocks.metadata } }));
vi.mock('../../../../services/localThreeBand', () => ({ loadLocalThreeBand: mocks.load }));
vi.mock('../../../../services/audioReadDiagnostics', () => ({
  reportAudioReadFailure: vi.fn(),
  withSharedAudioMetadataRead: (_key: string, read: () => Promise<any>) => read(),
}));
vi.mock('../../../../hooks/useWaveformScratch', () => ({ useWaveformScratch: () => ({ className: '', onPointerDown: vi.fn() }) }));
vi.mock('../../../../hooks/useDJAudioEngine', () => ({ useDJAudioEngineActions: () => ({ seek: vi.fn() }) }));
vi.mock('../../../../lib/djAudio', () => ({ getDJAudioEngine: () => ({ isScratching: () => false, initialized: false, getPosition: () => 0 }) }));
import { DJCanvasWaveformDeck } from './DJCanvasWaveformDeck';

it('retries scrolling-lane metadata and local-band reads when its lane retry generation advances', async () => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  mocks.metadata.mockResolvedValue({ sourceFingerprint: 'retry-fp' });
  mocks.load.mockRejectedValueOnce(new Error('transient read failure')).mockResolvedValueOnce({
    sourceFingerprint: 'retry-fp',
    overview: { frames: 100, sampleRate: 10, low: [1], mid: [0.5], high: [0.25] },
  });
  const host = document.createElement('div');
  const root = createRoot(host);
  try {
    await act(async () => root.render(<DJCanvasWaveformDeck deck="A" visibleSeconds={10} colorMode="rgb" localBands retryGeneration={0} />));
    expect(mocks.load).toHaveBeenCalledTimes(1);
    await act(async () => root.render(<DJCanvasWaveformDeck deck="A" visibleSeconds={10} colorMode="rgb" localBands retryGeneration={1} />));
    expect(mocks.load).toHaveBeenCalledTimes(2);
  } finally {
    await act(async () => root.unmount());
    vi.restoreAllMocks();
  }
});
