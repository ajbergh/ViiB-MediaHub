import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DJFourChannelAudio, type DJAudioSource } from './djFourChannelAudio';

// A steady-state signal model checks complete paths, including splitter and
// merger port indices. This does not simulate OS drivers or browser rendering.
class Node {
  channelCount = 2;
  channelCountMode = 'max';
  channelInterpretation = 'speakers';
  maxChannelCount = 4;
  gain = {
    value: 1,
    cancelScheduledValues: vi.fn(),
    setTargetAtTime: vi.fn((value: number) => { this.gain.value = value; }),
  };
  incoming: { source: Node; output: number; input: number }[] = [];
  outgoing: Node[] = [];
  constructor(readonly kind: 'gain' | 'splitter' | 'merger' | 'destination') {}
  connect(target: Node, output = 0, input = 0) {
    target.incoming.push({ source: this, output, input });
    this.outgoing.push(target);
    return target;
  }
  disconnect() {
    for (const target of this.outgoing) target.incoming = target.incoming.filter(edge => edge.source !== this);
    this.outgoing = [];
  }
  signal(values: Map<Node, number[]>): number[] {
    if (values.has(this)) return values.get(this)!;
    const sum = Array(this.kind === 'merger' || this.kind === 'destination' ? 4 : 2).fill(0);
    for (const { source, output, input } of this.incoming) {
      const signal = source.signal(values);
      if (this.kind === 'merger') sum[input] += source.kind === 'splitter' ? signal[output] : signal[0];
      else signal.forEach((value, channel) => { if (channel < sum.length) sum[channel] += value; });
    }
    return this.kind === 'gain' ? sum.map(value => value * this.gain.value) : sum;
  }
}

class Context {
  static capacity = 4;
  static instances: Context[] = [];
  destination = new Node('destination');
  state = 'running';
  currentTime = 1;
  nodes: Node[] = [];
  resume = vi.fn(async () => {});
  close = vi.fn(async () => { this.state = 'closed'; });
  setSinkId = vi.fn(async (_id: string) => {});
  constructor() {
    this.destination.maxChannelCount = Context.capacity;
    Context.instances.push(this);
  }
  createGain() { const node = new Node('gain'); this.nodes.push(node); return node; }
  createChannelSplitter = vi.fn((_channels: number) => {
    const node = new Node('splitter'); this.nodes.push(node); return node;
  });
  createChannelMerger = vi.fn((_channels: number) => {
    const node = new Node('merger'); this.nodes.push(node); return node;
  });
}

function output(engine: DJFourChannelAudio, source: DJAudioSource = 'deckA'): number[] {
  const context = engine.context as unknown as Context;
  return context.destination.signal(new Map([[engine.inputs[source] as unknown as Node, [0.2, 0.4]]]));
}

describe('single-device four-channel DJ audio', () => {
  beforeEach(() => {
    Context.capacity = 4;
    Context.instances = [];
    vi.stubGlobal('AudioContext', Context);
  });
  afterEach(() => vi.unstubAllGlobals());

  it('selects the sink and explicitly maps stereo master and headphones to four discrete channels', async () => {
    const engine = await DJFourChannelAudio.create({ deviceId: 'mixtrack-audio' });
    const context = Context.instances[0];
    expect(context.setSinkId).toHaveBeenCalledWith('mixtrack-audio');
    expect(context.destination.channelCount).toBe(4);
    expect(context.destination.channelCountMode).toBe('explicit');
    expect(context.destination.channelInterpretation).toBe('discrete');
    expect(context.createChannelSplitter).toHaveBeenCalledTimes(2);
    expect(context.createChannelMerger).toHaveBeenCalledWith(4);
    expect(context.destination.incoming).toHaveLength(1);
    expect(output(engine)).toEqual([0.2, 0.4, 0, 0]);
    engine.setCue('deckA', true);
    expect(output(engine)).toEqual([0.2, 0.4, 0.2, 0.4]);
    await engine.dispose();
  });

  it.each(['deckA', 'deckB', 'sampler'] as const)('%s has independent pre-fader cue and source volume', async source => {
    const engine = await DJFourChannelAudio.create();
    engine.setSourceVolume(source, 0);
    engine.setMasterVolume(0);
    engine.setCue(source, true);
    expect(output(engine, source)).toEqual([0, 0, 0.2, 0.4]);
    engine.setCue(source, false);
    expect(output(engine, source)).toEqual([0, 0, 0, 0]);
    await engine.dispose();
  });

  it('blends post-volume master into headphones, with exact cue-only and master-only endpoints', async () => {
    const engine = await DJFourChannelAudio.create();
    engine.setCue('deckA', true);
    engine.setSourceVolume('deckA', 0.5);
    engine.setMasterVolume(0.5);
    engine.setCueMix(0);
    expect(output(engine)).toEqual([0.05, 0.1, 0.2, 0.4]);
    engine.setCueMix(1);
    expect(output(engine)).toEqual([0.05, 0.1, 0.05, 0.1]);
    engine.setCueMix(0.5);
    expect(output(engine)[2]).toBeCloseTo(0.25 / Math.sqrt(2));
    expect(output(engine)[3]).toBeCloseTo(0.5 / Math.sqrt(2));
    await engine.dispose();
  });

  it('sums independently selected cues and controls headphone volume without affecting master', async () => {
    const engine = await DJFourChannelAudio.create();
    engine.setCue('deckA', true);
    engine.setCue('sampler', true);
    engine.setCueVolume(0.5);
    const values = new Map([
      [engine.inputs.deckA as unknown as Node, [0.1, 0.2]],
      [engine.inputs.deckB as unknown as Node, [0.2, 0.1]],
      [engine.inputs.sampler as unknown as Node, [0.3, 0.4]],
    ]);
    const result = Context.instances[0].destination.signal(values);
    expect(result[0]).toBeCloseTo(0.6);
    expect(result[1]).toBeCloseTo(0.7);
    expect(result[2]).toBeCloseTo(0.2);
    expect(result[3]).toBeCloseTo(0.3);
    engine.setCueVolume(0);
    expect(output(engine).slice(2)).toEqual([0, 0]);
    await engine.dispose();
  });

  it('clamps controls, rejects invalid values, and ramps gain changes', async () => {
    const engine = await DJFourChannelAudio.create();
    engine.setCueMix(100);
    expect(output(engine)).toEqual([0.2, 0.4, 0.2, 0.4]);
    engine.setCueVolume(-1);
    expect(output(engine)).toEqual([0.2, 0.4, 0, 0]);
    expect(() => engine.setCueMix(NaN)).toThrow(RangeError);
    expect(() => engine.setSourceVolume('deckA', Infinity)).toThrow(RangeError);
    const ramped = Context.instances[0].nodes.filter(node => node.gain.setTargetAtTime.mock.calls.length);
    expect(ramped.length).toBeGreaterThan(0);
    for (const node of ramped) expect(node.gain.cancelScheduledValues).toHaveBeenCalledWith(1);
    await engine.dispose();
  });

  it('refuses stereo-only devices before connecting audio and closes the context', async () => {
    Context.capacity = 2;
    await expect(DJFourChannelAudio.create()).rejects.toThrow('four are required');
    expect(Context.instances[0].destination.incoming).toHaveLength(0);
    expect(Context.instances[0].close).toHaveBeenCalledOnce();
  });

  it('allows the OS default when sink selection is unavailable; rejects a named sink', async () => {
    class NoSinkContext extends Context { setSinkId = undefined; }
    vi.stubGlobal('AudioContext', NoSinkContext);
    await expect(DJFourChannelAudio.create({ deviceId: 'mixtrack-audio' })).rejects.toThrow('OS default');
    expect(Context.instances[0].close).toHaveBeenCalledOnce();
    const engine = await DJFourChannelAudio.create({ deviceId: 'default' });
    expect(output(engine)).toEqual([0.2, 0.4, 0, 0]);
    await engine.dispose();
  });

  it('closes the context if device selection fails', async () => {
    class FailedSinkContext extends Context {
      setSinkId = vi.fn(async () => { throw new Error('Device disconnected'); });
    }
    vi.stubGlobal('AudioContext', FailedSinkContext);
    await expect(DJFourChannelAudio.create({ deviceId: 'gone' })).rejects.toThrow('Device disconnected');
    expect(Context.instances[0].close).toHaveBeenCalledOnce();
  });

  it('checks channel capacity after selecting the device', async () => {
    Context.capacity = 2;
    class MultichannelSink extends Context {
      setSinkId = vi.fn(async () => { this.destination.maxChannelCount = 4; });
    }
    vi.stubGlobal('AudioContext', MultichannelSink);
    const engine = await DJFourChannelAudio.create({ deviceId: 'four-channel' });
    expect(output(engine)).toEqual([0.2, 0.4, 0, 0]);
    await engine.dispose();
  });

  it('closes immediately on cancellation even if resume is still pending', async () => {
    let resume: () => void;
    class PendingResumeContext extends Context {
      resume = vi.fn(() => new Promise<void>(resolve => { resume = resolve; }));
    }
    vi.stubGlobal('AudioContext', PendingResumeContext);
    const controller = new AbortController();
    const pending = DJFourChannelAudio.create({}, controller.signal);
    controller.abort();
    expect(Context.instances[0].close).toHaveBeenCalledOnce();
    resume();
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
    expect(Context.instances[0].destination.incoming).toHaveLength(0);
  });

  it('disconnects and closes if the browser rejects resume', async () => {
    class RejectedResumeContext extends Context {
      resume = vi.fn(async () => { throw new Error('Audio permission denied'); });
    }
    vi.stubGlobal('AudioContext', RejectedResumeContext);
    await expect(DJFourChannelAudio.create()).rejects.toThrow('Audio permission denied');
    expect(Context.instances[0].destination.incoming).toHaveLength(0);
    expect(Context.instances[0].close).toHaveBeenCalledOnce();
  });

  it('disconnects all graph nodes, closes once, and rejects controls after disposal', async () => {
    const engine = await DJFourChannelAudio.create();
    await engine.dispose();
    await engine.dispose();
    const context = Context.instances[0];
    expect(context.close).toHaveBeenCalledOnce();
    expect(context.destination.incoming).toHaveLength(0);
    expect(context.nodes.every(node => node.outgoing.length === 0)).toBe(true);
    expect(() => engine.setCue('deckB', true)).toThrow('disposed');
  });
});
