import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { describe, expect, it } from 'vitest';

describe('stem transport worklet', () => {
  it('resamples package frames onto the AudioContext clock before applying tempo', () => {
    class WorkletBase {
      port = { onmessage: null as ((event: { data: Record<string, unknown> }) => void) | null, postMessage: () => {} };
    }
    let Processor: (new () => { port: WorkletBase['port']; process: (inputs: unknown[], outputs: Float32Array[][]) => boolean }) | undefined;
    const source = readFileSync(new URL('./stemTransport.worklet.js', import.meta.url), 'utf8');
    runInNewContext(source, {
      AudioWorkletProcessor: WorkletBase,
      registerProcessor: (_name: string, processor: typeof Processor) => { Processor = processor; },
      globalThis: { sampleRate: 48000 },
      Float32Array,
      Map,
      Math,
    });
    if (!Processor) throw new Error('Worklet processor was not registered');

    const processor = new Processor();
    const command = (data: Record<string, unknown>) => processor.port.onmessage?.({ data });
    command({ type: 'configure', sampleRate: 44100, channels: 1, frames: 4, generation: 1 });
    const samples = new Float32Array(4 * 4);
    for (let frame = 0; frame < 4; frame++) for (let bus = 0; bus < 4; bus++) samples[frame * 4 + bus] = frame;
    command({ type: 'frames', startFrame: 0, frameCount: 4, channels: 1, data: samples.buffer, generation: 1 });
    command({ type: 'play', rate: 1.2, generation: 1 });

    const outputs = Array.from({ length: 4 }, () => [new Float32Array(2), new Float32Array(2)]);
    processor.process([], outputs);
    expect(outputs[0][0][0]).toBe(0);
    expect(outputs[0][0][1]).toBeCloseTo((44100 / 48000) * 1.2);
  });
});
