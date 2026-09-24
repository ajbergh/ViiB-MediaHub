import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { describe, expect, it } from 'vitest';

type Transport = {
  port: { onmessage: ((event: { data: Record<string, unknown> }) => void) | null; postMessage: (message: unknown) => void };
  position: number;
  playing: boolean;
  process: (inputs: unknown[], outputs: Float32Array[][]) => boolean;
};

function createTransport() {
  class WorkletBase {
    port = { onmessage: null as ((event: { data: Record<string, unknown> }) => void) | null, postMessage: (message: unknown) => messages.push(message as Record<string, unknown>) };
  }
  const messages: Record<string, unknown>[] = [];
  let Processor: (new () => Transport) | undefined;
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
  const render = (frames: number) => {
    const outputs = Array.from({ length: 4 }, () => [new Float32Array(frames), new Float32Array(frames)]);
    processor.process([], outputs);
    return outputs;
  };
  const configure = (frames: number, sampleRate = 48000, channels = 2, generation = 1) =>
    command({ type: 'configure', sampleRate, channels, frames, generation });
  const addFrames = (startFrame: number, frameCount: number, sample: (frame: number, bus: number, channel: number) => number, channels = 2, generation = 1) => {
    const samples = new Float32Array(frameCount * 4 * channels);
    for (let frame = 0; frame < frameCount; frame++) {
      for (let bus = 0; bus < 4; bus++) {
        for (let channel = 0; channel < channels; channel++) {
          samples[(frame * 4 + bus) * channels + channel] = sample(startFrame + frame, bus, channel);
        }
      }
    }
    command({ type: 'frames', startFrame, frameCount, channels, data: samples.buffer, generation });
  };
  return { processor, command, render, configure, addFrames, messages };
}

describe('stem transport worklet', () => {
  it('reconstructs four synthetic stereo buses at unity gain on one aligned clock', () => {
    const transport = createTransport();
    transport.configure(3);
    transport.addFrames(0, 3, (frame, bus, channel) => 100 * bus + 10 * channel + frame);
    transport.command({ type: 'play', generation: 1 });

    const outputs = transport.render(3);
    for (let bus = 0; bus < 4; bus++) {
      for (let channel = 0; channel < 2; channel++) {
        for (let frame = 0; frame < 3; frame++) {
          expect(outputs[bus][channel][frame]).toBe(100 * bus + 10 * channel + frame);
        }
      }
    }
    expect(transport.processor.position).toBe(3);
  });

  it('starts, pauses without advancing, and resumes from the same position', () => {
    const transport = createTransport();
    transport.configure(8);
    transport.addFrames(0, 8, frame => frame + 1);

    transport.command({ type: 'play', generation: 1 });
    expect(transport.render(2)[0][0]).toEqual(new Float32Array([1, 2]));
    transport.command({ type: 'pause', generation: 1 });
    expect(transport.render(3)[0][0]).toEqual(new Float32Array(3));
    expect(transport.processor.position).toBe(2);
    transport.command({ type: 'play', generation: 1 });
    expect(transport.render(2)[0][0]).toEqual(new Float32Array([3, 4]));
    expect(transport.processor.position).toBe(4);
  });

  it('resamples package frames onto the AudioContext clock before applying tempo', () => {
    const transport = createTransport();
    transport.configure(4, 44100, 1);
    transport.addFrames(0, 4, frame => frame, 1);
    transport.command({ type: 'play', rate: 1.2, generation: 1 });

    const outputs = transport.render(2);
    expect(outputs[0][0][0]).toBe(0);
    expect(outputs[0][0][1]).toBeCloseTo((44100 / 48000) * 1.2);
    expect(transport.processor.position).toBeCloseTo(2 * (44100 / 48000) * 1.2);
  });

  it('wraps at loop boundaries, interpolates across the loop seam, and bounds position reports', () => {
    const transport = createTransport();
    transport.configure(8);
    transport.addFrames(0, 8, frame => frame);
    transport.command({ type: 'loop', startFrame: 1, endFrame: 3, enabled: true, generation: 1 });
    transport.command({ type: 'seek', frame: 2.5, generation: 1 });
    transport.command({ type: 'play', generation: 1 });

    const loopOutput = transport.render(4)[0][0];
    expect(loopOutput[0]).toBeCloseTo(1.5);
    expect(loopOutput[1]).toBeCloseTo(1.5);
    expect(loopOutput[2]).toBeCloseTo(1.5);
    expect(loopOutput[3]).toBeCloseTo(1.5);
    expect(transport.processor.position).toBeCloseTo(2.5);

    transport.command({ type: 'seek', frame: 2, generation: 1 });
    expect(transport.render(3)[0][0]).toEqual(new Float32Array([2, 1, 2]));

    transport.command({ type: 'seek', frame: 1, generation: 1 });
    transport.messages.length = 0;
    transport.render(8193);
    const reports = transport.messages.filter(message => message.type === 'position');
    expect(reports).toHaveLength(2);
    expect(reports.every(message => Number(message.frame) >= 1 && Number(message.frame) <= 3)).toBe(true);
  });

  it('stops at end of stream and emits one ended message', () => {
    const transport = createTransport();
    transport.configure(2);
    transport.addFrames(0, 2, frame => frame + 1);
    transport.command({ type: 'play', generation: 1 });

    expect(transport.render(4)[0][0]).toEqual(new Float32Array([1, 2, 0, 0]));
    expect(transport.processor.position).toBe(2);
    expect(transport.processor.playing).toBe(false);
    expect(transport.messages.filter(message => message.type === 'ended')).toEqual([{ type: 'ended', frame: 2 }]);
  });

  it('rejects stale-generation frames and zero-fills an underrun before recovering', () => {
    const transport = createTransport();
    transport.configure(8, 48000, 2, 2);
    transport.addFrames(0, 2, () => 9, 2, 1);
    transport.command({ type: 'play', generation: 2 });
    expect(transport.render(2)[0][0]).toEqual(new Float32Array(2));
    expect(transport.messages.filter(message => message.type === 'underrun')).toHaveLength(1);

    transport.addFrames(2, 2, () => 0.75, 2, 2);
    expect(transport.render(2)[0][0]).toEqual(new Float32Array([0.75, 0.75]));
    expect(transport.messages.filter(message => message.type === 'underrun')).toHaveLength(1);
  });
});
