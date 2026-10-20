/** Public contract for the unchanged audio-thread JavaScript implementation. */
export function vinylMomentum(initialRate: number, targetRate: number, elapsed: number): { distance: number; done: boolean };

export type VinylScratchCommand =
  | { type: 'load'; channels: Float32Array[] }
  | { type: 'start'; position: number; token?: number }
  | { type: 'coast'; token: number; velocity: number; targetRate: number; shadowPosition?: number; shadowRate?: number }
  | { type: 'hold'; token: number }
  | { type: 'move'; delta: number; position?: number }
  | { type: 'move'; position: number; delta?: number }
  | { type: 'stop' };

export class VinylScratchTransport {
  constructor(sampleRate: number);
  sampleRate: number;
  channels: Float32Array[];
  position: number;
  target: number;
  rate: number;
  gain: number;
  active: boolean;
  token: number;
  smoothing: number;
  coast: {
    initial: number; target: number; frames: number; duration: number;
    distance: number; done: boolean; shadow: number | undefined; shadowRate: number | undefined;
  } | null;
  command(message: VinylScratchCommand): void;
  render(output: Float32Array[]): void;
}
