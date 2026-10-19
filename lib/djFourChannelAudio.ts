/** A single-device DJ mixer: master on channels 0/1, headphones on 2/3. */
export type DJAudioSource = 'deckA' | 'deckB' | 'sampler';

export interface DJFourChannelAudioOptions {
  /** An audiooutput deviceId from enumerateDevices(); omit for the OS default. */
  deviceId?: string;
  sampleRate?: number;
}

type SinkContext = AudioContext & { setSinkId?: (id: string) => Promise<void> };

function level(value: number): number {
  if (!Number.isFinite(value)) throw new RangeError('Audio levels must be finite.');
  return Math.max(0, Math.min(1, value));
}

/**
 * Owns its AudioContext and output graph. Create from a user gesture, then connect
 * sources created in engine.context to engine.inputs. Do not also connect those
 * sources to context.destination or to the existing separate-device DJ graph.
 */
export class DJFourChannelAudio {
  readonly context: AudioContext;
  readonly inputs: Readonly<Record<DJAudioSource, GainNode>>;

  private readonly nodes: AudioNode[] = [];
  private readonly faders = {} as Record<DJAudioSource, GainNode>;
  private readonly cueSends = {} as Record<DJAudioSource, GainNode>;
  private readonly master: GainNode;
  private readonly cueBlend: GainNode;
  private readonly masterBlend: GainNode;
  private readonly headphones: GainNode;
  private disposed = false;

  /** Select the device and validate capacity before connecting any audio. */
  static async create(options: DJFourChannelAudioOptions = {}, signal?: AbortSignal): Promise<DJFourChannelAudio> {
    const checkAborted = () => {
      if (signal?.aborted) throw new DOMException('Audio initialization cancelled.', 'AbortError');
    };
    checkAborted();
    const context = new AudioContext({ latencyHint: 'interactive', sampleRate: options.sampleRate });
    // A resume() awaiting browser permission may not settle promptly. Close the
    // context immediately on cancellation rather than waiting for that promise.
    const cancel = () => { void context.close().catch(() => {}); };
    signal?.addEventListener('abort', cancel, { once: true });
    let engine: DJFourChannelAudio | undefined;
    try {
      // Call resume immediately, while the initiating user gesture is active.
      // Await it after device setup, but handle rejection immediately as well.
      const resumed = context.resume();
      void resumed.catch(() => {});
      const deviceId = options.deviceId === 'default' ? '' : options.deviceId;
      if (deviceId) {
        const sinkContext = context as SinkContext;
        if (typeof sinkContext.setSinkId !== 'function') {
          throw new Error('This runtime cannot select a multichannel audio device. Select it as the OS default instead.');
        }
        await sinkContext.setSinkId(deviceId);
      }
      checkAborted();
      if (context.destination.maxChannelCount < 4) {
        throw new Error(`The selected audio device exposes ${context.destination.maxChannelCount} channels; four are required for separate master and headphones.`);
      }
      // "explicit" is a channelCountMode, not a channelInterpretation value.
      context.destination.channelCount = 4;
      context.destination.channelCountMode = 'explicit';
      context.destination.channelInterpretation = 'discrete';
      if (context.destination.channelCount !== 4) {
        throw new Error('The selected audio device did not accept four-channel output.');
      }
      engine = new DJFourChannelAudio(context);
      await resumed;
      checkAborted();
      return engine;
    } catch (error) {
      if (engine) await engine.dispose();
      else await context.close().catch(() => {});
      throw error;
    } finally {
      signal?.removeEventListener('abort', cancel);
    }
  }

  private constructor(context: AudioContext) {
    this.context = context;
    this.master = this.stereoGain(1);
    const cues = this.stereoGain(1);
    this.cueBlend = this.stereoGain(1);
    this.masterBlend = this.stereoGain(0);
    this.headphones = this.stereoGain(1);

    const inputs = {} as Record<DJAudioSource, GainNode>;
    for (const source of ['deckA', 'deckB', 'sampler'] as const) {
      const input = this.stereoGain(1);
      const fader = this.stereoGain(1);
      const cue = this.stereoGain(0);
      inputs[source] = input;
      this.faders[source] = fader;
      this.cueSends[source] = cue;
      input.connect(fader).connect(this.master);
      // Pre-fader listening: source volume and master volume cannot mute CUE.
      input.connect(cue).connect(cues);
    }
    this.inputs = Object.freeze(inputs);

    cues.connect(this.cueBlend).connect(this.headphones);
    this.master.connect(this.masterBlend).connect(this.headphones);

    const masterChannels = this.keep(context.createChannelSplitter(2));
    const headphoneChannels = this.keep(context.createChannelSplitter(2));
    const output = this.keep(context.createChannelMerger(4));
    masterChannels.channelInterpretation = 'discrete';
    headphoneChannels.channelInterpretation = 'discrete';
    output.channelInterpretation = 'discrete';
    this.master.connect(masterChannels);
    this.headphones.connect(headphoneChannels);
    masterChannels.connect(output, 0, 0);
    masterChannels.connect(output, 1, 1);
    headphoneChannels.connect(output, 0, 2);
    headphoneChannels.connect(output, 1, 3);
    output.connect(context.destination);
  }

  private keep<T extends AudioNode>(node: T): T {
    this.nodes.push(node);
    return node;
  }

  private stereoGain(value: number): GainNode {
    const node = this.keep(this.context.createGain());
    node.channelCount = 2;
    node.channelCountMode = 'explicit';
    // Duplicate mono sources into L/R; preserve stereo sources without summing.
    node.channelInterpretation = 'speakers';
    node.gain.value = value;
    return node;
  }

  private checkActive(): void {
    if (this.disposed) throw new Error('The DJ audio engine has been disposed.');
  }

  private ramp(node: GainNode, value: number): void {
    this.checkActive();
    const now = this.context.currentTime;
    node.gain.cancelScheduledValues(now);
    node.gain.setTargetAtTime(value, now, 0.01);
  }

  setSourceVolume(source: DJAudioSource, volume: number): void {
    this.ramp(this.faders[source], level(volume));
  }

  setCue(source: DJAudioSource, enabled: boolean): void {
    this.ramp(this.cueSends[source], enabled ? 1 : 0);
  }

  setMasterVolume(volume: number): void {
    this.ramp(this.master, level(volume));
  }

  /** Equal-power blend: 0 = selected cues only, 1 = post-volume master only. */
  setCueMix(mix: number): void {
    const value = level(mix);
    this.ramp(this.cueBlend, value === 1 ? 0 : Math.cos(value * Math.PI / 2));
    this.ramp(this.masterBlend, value === 0 ? 0 : Math.sin(value * Math.PI / 2));
  }

  setCueVolume(volume: number): void {
    this.ramp(this.headphones, level(volume));
  }

  /** Stop/disconnect source nodes owned by the caller before disposing. */
  async dispose(): Promise<void> {
    if (this.disposed) return;
    this.disposed = true;
    for (const node of this.nodes) node.disconnect();
    if (this.context.state !== 'closed') await this.context.close();
  }
}
