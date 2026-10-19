import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  state: { audioSettings: { djOutputMode: 'four-channel', mainOutputDevice: 'controller', headphoneOutputDevice: 'monitor' } },
  attach: vi.fn(), detach: vi.fn(),
}));
vi.mock('../store', () => ({ useStore: { getState: () => mocks.state } }));
vi.mock('./djSampler', () => ({ getDJSamplerEngine: () => ({ attachOutput: mocks.attach, detachOutput: mocks.detach }) }));
vi.mock('./djAudioEffects', () => ({ createReverbImpulse: vi.fn(), getBeatFXDelayTime: vi.fn(() => 0.5) }));
vi.mock('./stemDeckSource', () => ({ StemDeckSource: class {
  outputNode: Node;
  dispose = vi.fn();
  constructor(context: Context) { this.outputNode = context.createGain(); }
} }));
import { DJAudioEngine } from './djAudio';

const param = () => ({ value: 1, setTargetAtTime: vi.fn(), setValueAtTime: vi.fn(), linearRampToValueAtTime: vi.fn() });
class Node {
  channelCount = 2;
  channelCountMode = 'max';
  channelInterpretation = 'speakers';
  maxChannelCount = 4;
  gain = param(); frequency = param(); Q = param(); delayTime = param();
  threshold = param(); knee = param(); ratio = param(); attack = param(); release = param();
  stream = {};
  edges: { target: unknown; output: number; input: number }[] = [];
  start = vi.fn();
  stop = vi.fn();
  onended: (() => void) | null = null;
  constructor(readonly kind: string) {}
  connect(target: unknown, output = 0, input = 0): any { this.edges.push({ target, output, input }); return target; }
  disconnect(target?: unknown) { this.edges = target ? this.edges.filter(edge => edge.target !== target) : []; }
}
class Context {
  static instances: Context[] = [];
  destination = new Node('destination');
  currentTime = 0;
  state = 'running';
  sink = '';
  nodes: Node[] = [];
  constructor() { Context.instances.push(this); }
  private node(kind: string) { const node = new Node(kind); this.nodes.push(node); return node; }
  createGain = () => this.node('gain');
  createBiquadFilter = () => this.node('filter');
  createAnalyser = () => this.node('analyser');
  createDynamicsCompressor = () => this.node('limiter');
  createDelay = () => this.node('delay');
  createConvolver = () => this.node('convolver');
  createOscillator = () => this.node('oscillator');
  createMediaStreamDestination = () => this.node('stream');
  createMediaStreamSource = () => this.node('streamSource');
  createChannelSplitter = vi.fn(() => this.node('splitter'));
  createChannelMerger = vi.fn(() => this.node('merger'));
  setSinkId = vi.fn(async (id: string) => {
    if (id === 'missing') throw new Error('Device unavailable');
    this.sink = id;
    this.destination.maxChannelCount = id === 'stereo' ? 2 : 4;
  });
  close = vi.fn(async () => { this.state = 'closed'; });
  suspend = vi.fn(async () => { this.state = 'suspended'; });
  resume = vi.fn(async () => { this.state = 'running'; });
}

function engine() {
  const result = new DJAudioEngine();
  for (const method of ['setupEventListeners', 'startPositionTracking', 'startVUMetering']) {
    vi.spyOn(result as any, method).mockImplementation(() => {});
  }
  return result;
}

describe('live DJ engine output routing', () => {
  beforeEach(() => {
    Context.instances = [];
    mocks.state.audioSettings = { djOutputMode: 'four-channel', mainOutputDevice: 'controller', headphoneOutputDevice: 'monitor' };
    vi.stubGlobal('window', { AudioContext: Context });
    vi.stubGlobal('AudioContext', Context);
  });
  afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); vi.clearAllMocks(); });

  it('initializes the actual mixer from persisted settings and connects only one four-channel destination', async () => {
    const audio = engine();
    await Promise.all([audio.initialize(), audio.initialize()]);
    expect(Context.instances).toHaveLength(1);
    const ctx = Context.instances[0];
    expect(ctx.setSinkId).toHaveBeenCalledWith('controller');
    expect(ctx.destination.channelCount).toBe(4);
    expect(ctx.destination.channelCountMode).toBe('explicit');
    expect(ctx.destination.channelInterpretation).toBe('discrete');
    const internals = audio as any;
    const main = internals.analyserMaster.edges.find((edge: any) => edge.target.kind === 'splitter').target as Node;
    const cue = internals.headphoneMixer.edges[0].target as Node;
    expect(main.edges.map(edge => [edge.output, edge.input])).toEqual([[0, 0], [1, 1]]);
    expect(cue.edges.map(edge => [edge.output, edge.input])).toEqual([[0, 2], [1, 3]]);
    expect(ctx.nodes.filter(node => node.edges.some(edge => edge.target === ctx.destination)).map(node => node.kind)).toEqual(['merger']);
    expect(mocks.attach).toHaveBeenCalledWith(ctx, internals.masterGain);
    expect(audio.getHeadphoneOutputDeviceId()).toBe('controller');
    audio.dispose();
  });

  it('uses the cue mix endpoints without a separate master-cue gate in four-channel mode', async () => {
    const audio = engine(); await audio.initialize();
    audio.setMasterCueEnabled(false);
    const internals = audio as any;
    audio.updateHeadphoneMix(1);
    expect(internals.headphoneCueMix.gain.setTargetAtTime.mock.lastCall[0]).toBeCloseTo(0);
    expect(internals.headphoneMasterMix.gain.setTargetAtTime.mock.lastCall[0]).toBe(1);
    expect(audio.getMasterCueEnabled()).toBe(true);
    audio.updateHeadphoneMix(0);
    expect(internals.headphoneCueMix.gain.setTargetAtTime.mock.lastCall[0]).toBe(1);
    expect(internals.headphoneMasterMix.gain.setTargetAtTime.mock.lastCall[0]).toBe(0);
    audio.dispose();
  });

  it('switches from separate devices to four channels and closes the secondary cue context', async () => {
    mocks.state.audioSettings.djOutputMode = 'separate';
    const audio = engine(); await audio.initialize();
    const ctx = Context.instances[0], headphoneContext = Context.instances[1];
    expect((audio as any).analyserMaster.edges.some((edge: any) => edge.target === ctx.destination)).toBe(true);
    await audio.setOutputRouting('four-channel', 'controller');
    expect(headphoneContext.close).toHaveBeenCalledOnce();
    expect(ctx.suspend).toHaveBeenCalledOnce();
    expect(ctx.resume).toHaveBeenCalledOnce();
    expect((audio as any).headphoneMixer.edges).toHaveLength(1);
    expect((audio as any).headphoneMixer.edges[0].target.kind).toBe('splitter');
    await expect(audio.setHeadphoneOutputDevice('monitor')).rejects.toThrow('channels 3–4');
    audio.dispose();
  });

  it('restores the previous four-channel route when the new device only supports stereo', async () => {
    const audio = engine(); await audio.initialize();
    const ctx = Context.instances[0];
    await expect(audio.setOutputRouting('four-channel', 'stereo')).rejects.toThrow('Four channels are required');
    expect(audio.getMainOutputDeviceId()).toBe('controller');
    expect(audio.getOutputMode()).toBe('four-channel');
    expect(ctx.sink).toBe('controller');
    expect(ctx.state).toBe('running');
    expect(ctx.nodes.filter(node => node.edges.some(edge => edge.target === ctx.destination))).toHaveLength(1);
    audio.dispose();
  });

  it('returns to separate stereo routes and preserves the legacy master-cue gate', async () => {
    const audio = engine(); await audio.initialize();
    await audio.setOutputRouting('separate', 'stereo');
    const ctx = Context.instances[0];
    expect(ctx.destination.channelCount).toBe(2);
    expect(ctx.nodes.filter(node => node.edges.some(edge => edge.target === ctx.destination)).map(node => node.kind)).toEqual(['analyser']);
    expect(audio.getHeadphoneOutputDeviceId()).toBe('monitor');
    audio.setMasterCueEnabled(false);
    audio.updateHeadphoneMix(1);
    expect((audio as any).headphoneMasterMix.gain.setTargetAtTime.mock.lastCall[0]).toBe(0);
    audio.dispose();
  });

  it('keeps the saved separate headphone device when routing is applied before the first track', async () => {
    const audio = engine();
    await audio.setOutputRouting('separate', 'speakers');
    expect(audio.getMainOutputDeviceId()).toBe('speakers');
    expect(audio.getHeadphoneOutputDeviceId()).toBe('monitor');
    expect(Context.instances[1].setSinkId).toHaveBeenCalledWith('monitor');
    audio.dispose();
  });

  it('sends an isolated headphone test tone to merger input 2, bypassing cue controls', async () => {
    const audio = engine(); await audio.initialize();
    const ctx = Context.instances[0];
    const pending = audio.testOutputChannel(2);
    const tone = ctx.nodes.filter(node => node.kind === 'oscillator').at(-1)!;
    const gain = tone.edges[0].target as Node;
    expect(gain.edges[0].input).toBe(2);
    expect((gain.edges[0].target as Node).kind).toBe('merger');
    expect(gain.gain.linearRampToValueAtTime).toHaveBeenCalledWith(0.1, 0.02);
    expect(tone.stop).toHaveBeenCalledWith(0.8);
    tone.onended!(); await pending;
    expect(tone.edges).toHaveLength(0);
    expect(gain.edges).toHaveLength(0);
    audio.dispose();
  });

  it('stops a test when its route is disconnected and rejects tests on a stereo route', async () => {
    const audio = engine(); await audio.initialize();
    const pending = audio.testOutputChannel(3);
    const tone = Context.instances[0].nodes.filter(node => node.kind === 'oscillator').at(-1)!;
    await audio.setOutputRouting('separate', 'stereo');
    await pending;
    expect(tone.stop).toHaveBeenCalledTimes(2);
    expect(tone.edges).toHaveLength(0);
    await expect(audio.testOutputChannel(2)).rejects.toThrow('Apply the four-channel');
    audio.dispose();
  });
});
