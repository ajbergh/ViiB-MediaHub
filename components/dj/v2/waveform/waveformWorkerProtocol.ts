import type { MainWaveformFrame } from './canvasWaveformRenderer';

export type WaveformWorkerRequest =
  | { type: 'state'; generation: number; frame: Partial<Omit<MainWaveformFrame, 'position'>> }
  | { type: 'draw'; generation: number; position: number; width: number; height: number; ratio: number };
export type WaveformWorkerResponse =
  | { type: 'ready' }
  | { type: 'frame'; generation: number; bitmap: ImageBitmap }
  | { type: 'error'; message: string };
