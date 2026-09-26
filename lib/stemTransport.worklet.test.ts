import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { describe, expect, it } from 'vitest';

type Transport = {
  port: { onmessage: ((event: { data: Record<string, unknown> }) => void) | null; postMessage: (message: unknown) => void };
  position: number;
  playing: boolean;
  wetMix: number;
  process: (inputs: unknown[], outputs: Float32Array[][]) => boolean;
};

/** The real Signalsmith core, captured through the same prelude the browser loader uses. */
function loadStretchCore(sampleRate: number) {
  const prelude = readFileSync(new URL('./stemStretchModule.ts', import.meta.url), 'utf8').match(/STRETCH_PRELUDE = `([\s\S]*?)`;/)?.[1];
  const source = readFileSync(new URL('../node_modules/signalsmith-stretch/SignalsmithStretch.mjs', import.meta.url), 'utf8');
  // Scripts cannot contain the package's closing ES export; the loader runs it as a module.
  const script = source.replace(/export default _export;\s*$/, '');
  if (!prelude || script === source) throw new Error('signalsmith-stretch layout changed; re-check stemStretchModule.ts');
  const context: Record<string, unknown> = { sampleRate };
  runInNewContext(prelude + script, context);
  return context.__viibStretchCore;
}

function createTransport({ sampleRate = 48000, core }: { sampleRate?: number; core?: unknown } = {}) {
  class WorkletBase {
    port = { onmessage: null as ((event: { data: Record<string, unknown> }) => void) | null, postMessage: (message: unknown) => messages.push(message as Record<string, unknown>) };
  }
  const messages: Record<string, unknown>[] = [];
  let Processor: (new () => Transport) | undefined;
  const source = readFileSync(new URL('./stemTransport.worklet.js', import.meta.url), 'utf8');
  runInNewContext(source, {
    AudioWorkletProcessor: WorkletBase,
    registerProcessor: (_name: string, processor: typeof Processor) => { Processor = processor; },
    globalThis: { sampleRate, __viibStretchCore: core },
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
  const configure = (frames: number, packageRate = sampleRate, channels = 2, generation = 1) =>
    command({ type: 'configure', sampleRate: packageRate, channels, frames, generation });
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

  it('reports when no stretch core is available', () => {
    expect(createTransport().messages).toContainEqual({ type: 'stretch', ready: false });
  });
});

const RATE = 44100;
const sine = (frequency: number) => (frame: number) => 0.5 * Math.sin(2 * Math.PI * frequency * frame / RATE);

/** Cycles per second from rising zero crossings. */
function frequencyOf(samples: Float32Array) {
  let crossings = 0;
  let first = -1;
  let last = -1;
  for (let i = 1; i < samples.length; i++) {
    if (samples[i - 1] < 0 && samples[i] >= 0) {
      crossings++;
      if (first < 0) first = i;
      last = i;
    }
  }
  return (crossings - 1) / ((last - first) / RATE);
}

async function keyLockTransport(frames = RATE * 3) {
  const transport = createTransport({ sampleRate: RATE, core: loadStretchCore(RATE) });
  transport.configure(frames);
  // Bus 1 is bus 0 inverted: they only cancel if the buses stay phase-coherent.
  transport.addFrames(0, frames, (frame, bus) => (bus === 0 ? 1 : bus === 1 ? -1 : 0) * sine(440)(frame));
  for (let attempt = 0; attempt < 200 && !transport.messages.some(message => message.type === 'stretch'); attempt++) {
    transport.render(128);
    await new Promise(resolve => setTimeout(resolve, 5));
  }
  expect(transport.messages).toContainEqual({ type: 'stretch', ready: true });
  return transport;
}

function renderSeconds(transport: ReturnType<typeof createTransport>, seconds: number) {
  const blocks = Math.round(seconds * RATE / 128);
  const bus0 = new Float32Array(blocks * 128);
  const bus1 = new Float32Array(blocks * 128);
  for (let block = 0; block < blocks; block++) {
    const outputs = transport.render(128);
    bus0.set(outputs[0][0], block * 128);
    bus1.set(outputs[1][0], block * 128);
  }
  return { bus0, bus1 };
}

describe('stem transport key lock', () => {
  it('keeps the original pitch at a changed tempo while the clock still follows tempo', async () => {
    const transport = await keyLockTransport();
    transport.command({ type: 'keyLock', enabled: true });
    transport.command({ type: 'play', rate: 1.12, generation: 1 });
    const { bus0, bus1 } = renderSeconds(transport, 0.6);
    const tail = Math.round(RATE * 0.3);

    expect(transport.processor.wetMix).toBeGreaterThan(0.99);
    expect(Math.abs(frequencyOf(bus0.subarray(tail)) - 440)).toBeLessThan(4);
    expect(transport.processor.position).toBeCloseTo(Math.round(0.6 * RATE / 128) * 128 * 1.12, 0);
    let residual = 0;
    for (let i = tail; i < bus0.length; i++) residual = Math.max(residual, Math.abs(bus0[i] + bus1[i]));
    expect(residual).toBeLessThan(0.001);
  });

  it('plays varispeed pitch without key lock and bypasses the stretcher at unity tempo', async () => {
    const transport = await keyLockTransport();
    transport.command({ type: 'play', rate: 1.12, generation: 1 });
    const varispeed = renderSeconds(transport, 0.4).bus0.subarray(Math.round(RATE * 0.1));
    expect(Math.abs(frequencyOf(varispeed) - 440 * 1.12)).toBeLessThan(4);

    transport.command({ type: 'keyLock', enabled: true });
    transport.command({ type: 'rate', rate: 1, generation: 1 });
    renderSeconds(transport, 0.2);
    expect(transport.processor.wetMix).toBe(0);
  });

  it('plays dry immediately after a seek, then crossfades back to key lock', async () => {
    const transport = await keyLockTransport();
    transport.command({ type: 'keyLock', enabled: true });
    transport.command({ type: 'play', rate: 0.9, generation: 1 });
    renderSeconds(transport, 0.3);
    expect(transport.processor.wetMix).toBeGreaterThan(0.99);

    transport.command({ type: 'seek', frame: RATE * 2, generation: 1 });
    transport.render(128);
    expect(transport.processor.wetMix).toBe(0);
    renderSeconds(transport, 0.2);
    expect(transport.processor.wetMix).toBeGreaterThan(0.99);
  });
});

describe('stem transport key lock alignment', () => {
  const CLICK = Math.round(RATE * 0.3);
  const click = (frame: number) => { const k = ((frame % CLICK) + CLICK) % CLICK; return k < 48 ? 0.8 * (1 - k / 48) : 0; };
  const onsets = (samples: number[]) => {
    const found: number[] = [];
    for (let i = 1; i < samples.length; i++) {
      if (Math.abs(samples[i]) > 0.2 && Math.abs(samples[i - 1]) <= 0.2 && (!found.length || i - found[found.length - 1] > 2000)) found.push(i);
    }
    return found;
  };

  async function clickTransport() {
    const frames = RATE * 12;
    const transport = createTransport({ sampleRate: RATE, core: loadStretchCore(RATE) });
    transport.configure(frames);
    transport.addFrames(0, frames, (frame, bus) => (bus === 0 ? click(frame) : 0));
    for (let attempt = 0; attempt < 200 && !transport.messages.some(message => message.type === 'stretch'); attempt++) {
      transport.render(128);
      await new Promise(resolve => setTimeout(resolve, 5));
    }
    return transport;
  }

  /** Key-locked output next to the dry output the same clock would have produced. */
  function play(transport: ReturnType<typeof createTransport>, rate: number, seconds: number, loop?: { start: number; end: number }) {
    transport.command({ type: 'keyLock', enabled: true });
    transport.command({ type: 'seek', frame: RATE * 5, generation: 1 });
    if (loop) transport.command({ type: 'loop', startFrame: loop.start, endFrame: loop.end, enabled: true, generation: 1 });
    transport.command({ type: 'play', rate, generation: 1 });
    const wet: number[] = [];
    const dry: number[] = [];
    for (let block = 0; block < Math.round(seconds * RATE / 128); block++) {
      let position = transport.processor.position;
      for (let i = 0; i < 128; i++, position += rate) {
        if (loop && position >= loop.end) position -= loop.end - loop.start;
        dry.push(click(Math.floor(position)));
      }
      wet.push(...transport.render(128)[0][0]);
    }
    // Compare once the stretcher has warmed up and crossfaded in.
    const settled = (i: number) => i > RATE * 0.25;
    const dryOnsets = onsets(dry).filter(settled);
    return onsets(wet).filter(settled).map(at => at - dryOnsets.reduce((best, x) => (Math.abs(x - at) < Math.abs(best - at) ? x : best)));
  }

  it.each([0.92, 1.06])('lands transients within a millisecond of the dry clock at %sx', async rate => {
    const transport = await clickTransport();
    const offsets = play(transport, rate, 1.5);
    expect(transport.processor.wetMix).toBe(1);
    expect(offsets.length).toBeGreaterThanOrEqual(3);
    for (const offset of offsets) expect(Math.abs(offset)).toBeLessThan(RATE / 1000);
  });

  it('stays aligned through repeated loop wraps without re-priming', async () => {
    const transport = await clickTransport();
    const offsets = play(transport, 1.02, 2, { start: RATE * 5, end: RATE * 5 + Math.round(RATE * 0.5) });
    expect(offsets.length).toBeGreaterThanOrEqual(5);
    for (const offset of offsets) expect(Math.abs(offset)).toBeLessThan(RATE / 1000);
  });
});

describe('stem transport scratch', () => {
  const scratchOf = (transport: ReturnType<typeof createTransport>) =>
    (command: Record<string, unknown>) => transport.command({ type: 'scratch', command });

  it('reads all four stem buses backwards and forwards and settles silently', () => {
    const transport = createTransport();
    transport.configure(48000);
    transport.addFrames(0, 48000, (frame, bus) => (bus + 1) * frame / 48000);
    const scratch = scratchOf(transport);

    scratch({ type: 'start', position: 0.5, token: 1 });
    expect(transport.render(4800)[0][0].every(value => value === 0)).toBe(true);
    scratch({ type: 'move', delta: 0.2 });
    const forward = transport.render(960);
    expect(forward[0][0][900]).toBeGreaterThan(forward[0][0][400]);
    expect(forward[3][0][900]).toBeCloseTo(4 * forward[0][0][900], 5);
    scratch({ type: 'move', delta: -0.4 });
    const backward = transport.render(960);
    expect(backward[0][0][900]).toBeLessThan(backward[0][0][400]);
    transport.render(9600);
    expect(transport.render(480)[0][0].every(value => Math.abs(value) < 0.00001)).toBe(true);
  });

  it('coasts from a flick to the deck tempo and reports the settled position', () => {
    const transport = createTransport();
    transport.configure(48000 * 4);
    transport.addFrames(0, 48000 * 4, frame => frame / 48000);
    const scratch = scratchOf(transport);
    scratch({ type: 'start', position: 1, token: 1 });
    scratch({ type: 'coast', velocity: -3, targetRate: 1, token: 2 });
    transport.render(48000 * 2);

    const events = transport.messages.filter(message => message.type === 'scratch').map(message => message.event as Record<string, unknown>);
    const settled = events.find(event => event.type === 'settled');
    expect(settled).toMatchObject({ token: 2 });
    expect(Number(settled!.rate)).toBeCloseTo(1, 1);
  });

  it('keeps ten seconds of history behind the playhead and reports evictions', () => {
    const transport = createTransport();
    const frames = 48000 * 14;
    transport.configure(frames);
    for (let start = 0; start < frames; start += 48000) transport.addFrames(start, 48000, frame => 1 + frame / 48000);
    transport.command({ type: 'play', generation: 1 });
    transport.render(48000 * 13);

    const evictions = transport.messages.filter(message => message.type === 'evicted').map(message => Number(message.frame));
    expect(evictions.length).toBeGreaterThan(0);
    expect(Math.max(...evictions)).toBeGreaterThan(48000 * 2);
    expect(Math.max(...evictions)).toBeLessThanOrEqual(48000 * 3);
    const scratch = scratchOf(transport);
    scratch({ type: 'start', position: 4, token: 1 });
    scratch({ type: 'move', delta: -0.5 });
    expect(transport.render(4800)[0][0].some(value => value > 3)).toBe(true);
  });
});
