const BUSES = 4;
// Retained behind the playhead so the jog can scratch backwards without a refetch.
const HISTORY_SECONDS = 10;
// Split computation keeps the STFT work under one render quantum; the block
// length only sets read-ahead because the stretcher reads buffered frames.
const STRETCH_CONFIG = { blockMs: 90, intervalMs: 22.5, splitComputation: true };
// Below this tempo deviation dry playback already has the right pitch.
const KEY_LOCK_MIN_DEVIATION = 0.0005;

/**
 * Key lock: one 8-channel Signalsmith Stretch instance for all four stereo
 * buses, so the buses stay phase-coherent. It reads the transport's own frames
 * ahead of the playhead, so steady playback is time-aligned with the dry path.
 * After one priming seek it is fed only the frames each quantum consumes.
 */
class StemStretch {
  constructor(Core) {
    this.core = new Core({ numberOfInputs: 1, numberOfOutputs: 1, outputChannelCount: [BUSES * 2] });
    this.ready = false;
    this.viewMemory = null;
    this.inputs = [];
    this.outputs = [];
    this.outputLength = 0;
  }

  poll() {
    if (this.ready || !this.core.wasmReady) return this.ready;
    Object.assign(this.core.config, STRETCH_CONFIG);
    this.core.configure();
    const wasm = this.core.wasmModule;
    wasm._setTransposeSemitones(0, 8000 / globalThis.sampleRate);
    wasm._setFormantSemitones(0, false);
    wasm._setFormantBase(0);
    this.inputLatency = wasm._inputLatency();
    this.outputLatency = wasm._outputLatency();
    this.length = this.core.bufferLength;
    this.ready = true;
    return true;
  }

  // Views over WASM memory, rebuilt only if the memory is ever replaced.
  views(frameCount) {
    const wasm = this.core.wasmModule;
    const memory = wasm.exports ? wasm.exports.memory.buffer : wasm.HEAP8.buffer;
    if (memory !== this.viewMemory || frameCount !== this.outputLength) {
      this.viewMemory = memory;
      this.outputLength = frameCount;
      this.inputs = this.core.buffersIn.map(pointer => new Float32Array(memory, pointer, this.length));
      this.outputs = this.core.buffersOut.map(pointer => new Float32Array(memory, pointer, frameCount));
    }
  }

  /** Re-analyse from `inputs[0..length)`, ending where the next quantum's input begins. */
  seek(rate) { this.core.wasmModule._seek(this.length, rate); }

  /** Consume `inputs[0..inputFrames)` and synthesise `frameCount` output frames. */
  process(inputFrames, frameCount) {
    this.core.wasmModule._process(inputFrames, frameCount);
    this.views(frameCount);
    return this.outputs;
  }
}

class ViibStemTransportProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.generation = 0;
    this.outputSampleRate = globalThis.sampleRate;
    this.sourceSampleRate = this.outputSampleRate;
    this.channels = 2;
    this.frames = 0;
    this.position = 0;
    this.rate = 1;
    this.playing = false;
    this.loop = { startFrame: 0, endFrame: 0, enabled: false };
    this.blocks = new Map();
    this.cachedBlock = null;
    this.outChannels = new Array(BUSES * 2);
    this.underrun = false;
    this.sincePositionReport = 0;
    this.retainedFrom = 0;
    this.keyLock = false;
    this.wetMix = 0;
    this.wetWarmFrames = 0;
    this.wetSmoothing = 1 - Math.exp(-1 / (this.outputSampleRate * 0.01));
    // Loop wraps move the playhead back; the stretcher is fed the unwrapped
    // stream, so playhead + wrapOffset keeps increasing through a loop.
    this.wrapOffset = 0;
    this.stretchPrimed = false;
    this.stretchFed = 0;
    this.scratch = { active: false, position: 0, target: 0, rate: 0, gain: 0, coast: null, token: 0, reportFrames: 0, reportedToken: -1 };
    this.scratchSmoothing = 1 - Math.exp(-1 / (this.outputSampleRate * 0.002));
    this.stretch = null;
    try {
      if (typeof globalThis.__viibStretchCore === 'function') this.stretch = new StemStretch(globalThis.__viibStretchCore);
    } catch {
      this.stretch = null;
    }
    this.stretchReported = false;
    if (!this.stretch) this.reportStretch(false);
    this.port.onmessage = ({ data }) => this.command(data);
  }

  reportStretch(ready) {
    if (this.stretchReported) return;
    this.stretchReported = true;
    this.port.postMessage({ type: 'stretch', ready });
  }

  // A jump the stretcher could not anticipate: its output lags by its output
  // latency, so play dry until it has re-read the new position.
  discontinuity() {
    this.wetMix = 0;
    this.wetWarmFrames = 0;
    this.stretchPrimed = false;
  }

  clearBlocks() {
    this.blocks.clear();
    this.cachedBlock = null;
  }

  command(message) {
    if (message.type === 'configure') {
      this.generation = message.generation;
      this.sourceSampleRate = message.sampleRate;
      this.channels = message.channels;
      this.frames = message.frames;
      this.position = 0;
      this.clearBlocks();
      this.retainedFrom = 0;
      this.playing = false;
      this.loop = { startFrame: 0, endFrame: 0, enabled: false };
      this.scratch.active = false;
      this.scratch.coast = null;
      this.scratch.gain = 0;
      this.discontinuity();
    } else if (message.type === 'flush') {
      this.generation = message.generation ?? this.generation;
      this.clearBlocks();
      this.playing = false;
      this.discontinuity();
    } else if (message.type === 'keyLock') {
      this.keyLock = !!message.enabled;
    } else if (message.type === 'scratch') {
      this.scratchCommand(message.command ?? {});
    } else if (message.generation !== undefined && message.generation !== this.generation) {
      return;
    } else if (message.type === 'frames') {
      this.blocks.set(message.startFrame, { start: message.startFrame, count: message.frameCount, channels: message.channels, samples: new Float32Array(message.data) });
      this.cachedBlock = null;
      this.underrun = false;
    } else if (message.type === 'seek') {
      this.position = Math.max(0, Math.min(this.frames, message.frame));
      this.underrun = false;
      this.sincePositionReport = 0;
      this.discontinuity();
    } else if (message.type === 'rate') {
      this.rate = Math.max(0.5, Math.min(1.5, message.rate || 1));
    } else if (message.type === 'loop') {
      this.loop = { startFrame: message.startFrame, endFrame: message.endFrame, enabled: message.enabled && message.endFrame > message.startFrame };
      // Frames already fed to the stretcher may lie beyond the new boundary.
      this.wetWarmFrames = 0;
      this.stretchPrimed = false;
    } else if (message.type === 'play') {
      this.rate = Math.max(0.5, Math.min(1.5, message.rate || this.rate));
      if (!this.playing) this.discontinuity();
      this.playing = true;
    } else if (message.type === 'pause') {
      this.playing = false;
    }
  }

  // Mirrors vinylScratch.worklet.js, in source frames and for four buses, so
  // the stem mix (and its mutes) stays audible while scratching.
  scratchCommand(message) {
    const scratch = this.scratch;
    const sourceRate = this.sourceSampleRate;
    if (message.type === 'start') {
      scratch.coast = null;
      scratch.token = message.token ?? 0;
      scratch.position = scratch.target = message.position * sourceRate;
      scratch.rate = scratch.gain = 0;
      scratch.active = true;
      this.discontinuity();
    } else if (message.type === 'coast') {
      scratch.token = message.token;
      scratch.coast = { initial: message.velocity, target: message.targetRate, frames: 0,
        duration: Math.min(1.6, 0.5 + Math.abs(message.velocity - message.targetRate) * 0.14),
        distance: 0, done: false, shadow: message.shadowPosition, shadowRate: message.shadowRate };
    } else if (message.type === 'hold') {
      scratch.token = message.token;
      scratch.coast = null;
      scratch.target = scratch.position;
      scratch.rate = 0;
      this.port.postMessage({ type: 'scratch', event: { type: 'held', token: scratch.token, position: scratch.position / sourceRate } });
    } else if (message.type === 'move') {
      scratch.target = Math.max(0, Math.min(Math.max(0, this.frames - 1),
        Number.isFinite(message.delta) ? scratch.target + message.delta * sourceRate : message.position * sourceRate));
    } else if (message.type === 'stop') {
      scratch.active = false;
      scratch.coast = null;
      // Hand back to the transport without a tail from the scratch path.
      scratch.gain = 0;
    }
  }

  // Nearly every lookup hits the block the previous one did.
  findBlock(frame) {
    const cached = this.cachedBlock;
    if (cached && frame >= cached.start && frame < cached.start + cached.count) return cached;
    for (const block of this.blocks.values()) {
      if (frame >= block.start && frame < block.start + block.count) {
        this.cachedBlock = block;
        return block;
      }
    }
    return null;
  }

  nextBlockStart(frame) {
    let next = Infinity;
    for (const block of this.blocks.values()) if (block.start > frame && block.start < next) next = block.start;
    return next;
  }

  /** The eight output channels in bus order, without allocating per quantum. */
  channelsOf(outputs) {
    const out = this.outChannels;
    for (let bus = 0; bus < BUSES; bus++) { out[bus * 2] = outputs[bus][0]; out[bus * 2 + 1] = outputs[bus][1]; }
    return out;
  }

  // Frames past the loop end belong to the next pass of the loop.
  wrapFrame(frame) {
    const loop = this.loop;
    if (!loop.enabled || frame < loop.endFrame) return frame;
    return loop.startFrame + ((frame - loop.startFrame) % (loop.endFrame - loop.startFrame));
  }

  /**
   * Interpolate all eight channels between two whole frames into `out[c][index]`.
   * Missing frames read as silence; mono packages feed both channels.
   */
  writeFrame(out, index, frame, next, fraction, gain) {
    const block = this.findBlock(frame);
    if (!block) {
      for (let c = 0; c < BUSES * 2; c++) out[c][index] = 0;
      return false;
    }
    const channels = block.channels;
    const right = channels > 1 ? 1 : 0;
    const samples = block.samples;
    const a = (frame - block.start) * BUSES * channels;
    let nextSamples = samples;
    let b = a;
    if (next >= block.start && next < block.start + block.count) b = (next - block.start) * BUSES * channels;
    else {
      const nextBlock = this.findBlock(next);
      if (nextBlock) { nextSamples = nextBlock.samples; b = (next - nextBlock.start) * BUSES * channels; }
      this.cachedBlock = block;
    }
    for (let bus = 0; bus < BUSES; bus++) {
      const offset = bus * channels;
      const left = samples[a + offset];
      const leftNext = nextSamples[b + offset];
      const rightNow = samples[a + offset + right];
      const rightNext = nextSamples[b + offset + right];
      out[bus * 2][index] = (left + (leftNext - left) * fraction) * gain;
      out[bus * 2 + 1][index] = (rightNow + (rightNext - rightNow) * fraction) * gain;
    }
    return true;
  }

  /** Interpolated read at a fractional source frame, optionally through the loop. */
  readInterpolated(position, out, index, gain, wrap) {
    const frame = Math.floor(position);
    const current = wrap ? this.wrapFrame(frame) : frame;
    const next = wrap ? this.wrapFrame(frame + 1) : Math.min(frame + 1, Math.max(0, this.frames - 1));
    return this.writeFrame(out, index, current, next, position - frame, gain);
  }

  /** Deinterleave `count` whole frames (loop-wrapped) into eight channel views at `at`. */
  copyFrames(views, firstFrame, count, at = 0) {
    let offset = 0;
    while (offset < count) {
      const frame = this.wrapFrame(firstFrame + offset);
      let run = count - offset;
      if (this.loop.enabled && frame < this.loop.endFrame) run = Math.min(run, this.loop.endFrame - frame);
      const dest = at + offset;
      if (frame < 0 || frame >= this.frames) {
        run = frame < 0 ? Math.min(run, -frame) : run;
        for (const view of views) view.fill(0, dest, dest + run);
      } else {
        const block = this.findBlock(frame);
        if (!block) {
          run = Math.min(run, this.nextBlockStart(frame) - frame, this.frames - frame);
          for (const view of views) view.fill(0, dest, dest + run);
        } else {
          run = Math.min(run, block.start + block.count - frame);
          const channels = block.channels;
          const stride = BUSES * channels;
          const right = channels > 1 ? 1 : 0;
          const samples = block.samples;
          const source = (frame - block.start) * stride;
          for (let bus = 0; bus < BUSES; bus++) {
            const leftView = views[bus * 2];
            const rightView = views[bus * 2 + 1];
            let s = source + bus * channels;
            for (let i = 0; i < run; i++, s += stride) {
              leftView[dest + i] = samples[s];
              rightView[dest + i] = samples[s + right];
            }
          }
        }
      }
      offset += run;
    }
  }

  /** Fill `count` stretcher input samples, spaced by the rate ratio, from a virtual frame. */
  fillStretchInput(views, first, count, at, ratio) {
    if (ratio === 1) this.copyFrames(views, first, count, at);
    else for (let i = 0; i < count; i++) this.readInterpolated(first + i * ratio, views, at + i, 1, true);
  }

  renderScratch(outputs, frameCount) {
    const scratch = this.scratch;
    const sampleRate = this.outputSampleRate;
    const step = this.sourceSampleRate / sampleRate;
    const last = Math.max(0, this.frames - 1);
    const out = this.channelsOf(outputs);
    const smoothing = this.scratchSmoothing;
    const sourceRate = this.sourceSampleRate;
    const servoFrames = sourceRate * 0.008;
    const active = scratch.active;
    const coast = active ? scratch.coast : null;
    // Hot per-sample state lives in locals; it is written back once per quantum.
    let position = scratch.position;
    let rate = scratch.rate;
    let gain = scratch.gain;
    const target = scratch.target;
    for (let i = 0; i < frameCount; i++) {
      // A short position servo bridges pointer events without sample jumps and
      // settles to silence when the hand stops moving.
      let desiredRate = active ? Math.max(-8, Math.min(8, (target - position) / servoFrames)) : 0;
      if (coast) {
        coast.frames++;
        const elapsed = coast.frames / sampleRate;
        if (coast.done) {
          desiredRate = coast.target;
        } else {
          const time = Math.min(coast.duration, elapsed);
          const remaining = 1 - time / coast.duration;
          const distance = coast.target * time + (coast.initial - coast.target) * coast.duration / 4 * (1 - remaining ** 4);
          desiredRate = (distance - coast.distance) * sampleRate;
          coast.distance = distance;
          if (elapsed >= coast.duration) {
            coast.done = true;
            if (Number.isFinite(coast.shadow)) position = (coast.shadow + elapsed * coast.shadowRate) * sourceRate;
          }
        }
      }
      rate += (desiredRate - rate) * smoothing;
      if (desiredRate === 0 && Math.abs(rate) < 1e-9) rate = 0;
      const audible = active && Math.abs(rate) > 0.001 && last > 0 &&
        !(position <= 0 && rate < 0) && !(position >= last && rate > 0);
      gain += ((audible ? 1 : 0) - gain) * smoothing;
      if (!audible && gain < 0.000001) gain = 0; // denormals are very slow
      position = Math.max(0, Math.min(last, position + rate * step));
      if (gain === 0) {
        for (let c = 0; c < BUSES * 2; c++) out[c][i] = 0;
      } else {
        this.readInterpolated(position, out, i, gain, false);
      }
    }
    scratch.position = position;
    scratch.rate = rate;
    scratch.gain = gain;
    scratch.reportFrames += frameCount;
    const settled = scratch.coast?.done && scratch.reportedToken !== scratch.token;
    if (scratch.coast && (settled || scratch.reportFrames >= sampleRate / 30)) {
      scratch.reportFrames = 0;
      if (settled) scratch.reportedToken = scratch.token;
      this.port.postMessage({ type: 'scratch', event: { type: settled ? 'settled' : 'position', token: scratch.token,
        position: scratch.position / this.sourceSampleRate, rate: scratch.rate,
        time: (globalThis.currentTime ?? 0) + frameCount / sampleRate } });
    }
  }

  renderDry(outputs, frameCount) {
    const out = this.channelsOf(outputs);
    const step = (this.sourceSampleRate / this.outputSampleRate) * this.rate;
    const loop = this.loop;
    for (let index = 0; index < frameCount; index++) {
      if (loop.enabled && this.position >= loop.endFrame) {
        const wrapped = loop.startFrame + ((this.position - loop.startFrame) % (loop.endFrame - loop.startFrame));
        this.wrapOffset += this.position - wrapped;
        this.position = wrapped;
      }
      if (this.position >= this.frames) {
        this.position = this.frames;
        this.playing = false;
        this.port.postMessage({ type: 'ended', frame: this.position });
        return index;
      }
      const frame = Math.floor(this.position);
      let nextFrame = frame + 1;
      if (loop.enabled && nextFrame >= loop.endFrame) nextFrame = loop.startFrame;
      else if (nextFrame >= this.frames) nextFrame = frame;
      if (this.writeFrame(out, index, frame, nextFrame, this.position - frame, 1)) {
        this.underrun = false;
      } else if (!this.underrun) {
        this.underrun = true;
        this.port.postMessage({ type: 'underrun', frame });
      }
      // Resample package frames onto the AudioContext clock. Tempo remains a
      // varispeed multiplier on top of the source/output-rate conversion.
      this.position += step;
      this.sincePositionReport++;
      if (this.sincePositionReport >= 4096) {
        this.sincePositionReport = 0;
        this.port.postMessage({ type: 'position', frame: this.position });
      }
    }
    return frameCount;
  }

  /**
   * Feed the stretcher up to where its output latency will have carried the
   * playhead, and return its output for this quantum. `virtualStart` is the
   * unwrapped playhead at the start of the quantum.
   */
  renderStretch(virtualStart, rendered) {
    const stretch = this.stretch;
    const ratio = this.sourceSampleRate / this.outputSampleRate;
    // process() consumes this quantum's input before synthesising its output.
    let target = virtualStart + ratio * ((stretch.outputLatency + rendered) * this.rate + stretch.inputLatency);
    if (ratio === 1) target = Math.round(target);
    stretch.views(rendered);
    const views = stretch.inputs;
    let feed = Math.round((target - this.stretchFed) / ratio);
    if (!this.stretchPrimed || feed < 0 || feed > stretch.length) {
      // Prime so this quantum is fed exactly like a steady-state one.
      feed = Math.min(stretch.length, Math.max(0, Math.round(rendered * this.rate)));
      const windowStart = target - (stretch.length + feed) * ratio;
      this.fillStretchInput(views, windowStart, stretch.length, 0, ratio);
      stretch.seek(this.rate);
      this.stretchFed = windowStart + stretch.length * ratio;
      this.stretchPrimed = true;
    }
    this.fillStretchInput(views, this.stretchFed, feed, 0, ratio);
    this.stretchFed += feed * ratio;
    return stretch.process(feed, rendered);
  }

  /** Crossfade the key-locked render over the dry varispeed render once it is valid. */
  mixKeyLock(outputs, virtualStart, rendered) {
    const stretch = this.stretch;
    if (!stretch?.poll()) return;
    const wanted = this.keyLock && Math.abs(this.rate - 1) > KEY_LOCK_MIN_DEVIATION;
    if (!wanted && this.wetMix === 0) {
      this.wetWarmFrames = 0;
      this.stretchPrimed = false;
      return;
    }
    const wet = this.renderStretch(virtualStart, rendered);
    this.wetWarmFrames += rendered;
    const target = wanted && this.wetWarmFrames > stretch.outputLatency ? 1 : 0;
    let mix = this.wetMix;
    if (mix === target && target === 1) {
      for (let bus = 0; bus < BUSES; bus++) {
        outputs[bus][0].set(wet[bus * 2]);
        outputs[bus][1].set(wet[bus * 2 + 1]);
      }
    } else {
      for (let index = 0; index < rendered; index++) {
        mix += (target - mix) * this.wetSmoothing;
        for (let bus = 0; bus < BUSES; bus++) {
          for (let channel = 0; channel < 2; channel++) {
            const dry = outputs[bus][channel][index];
            outputs[bus][channel][index] = dry + (wet[bus * 2 + channel][index] - dry) * mix;
          }
        }
      }
      if (target === 1 && mix > 0.9999) mix = 1;
    }
    this.wetMix = target === 0 && mix < 0.0001 ? 0 : mix;
  }

  evictBlocks(position) {
    const keepFrom = Math.floor(position) - 1 - Math.floor(HISTORY_SECONDS * this.sourceSampleRate);
    if (keepFrom <= this.retainedFrom) return;
    let evicted = false;
    for (const [start, block] of this.blocks) {
      const intersectsLoop = this.loop.enabled && start < this.loop.endFrame && start + block.count > this.loop.startFrame;
      if (start + block.count < keepFrom && !intersectsLoop) {
        this.blocks.delete(start);
        if (this.cachedBlock === block) this.cachedBlock = null;
        evicted = true;
      }
    }
    if (evicted) {
      this.retainedFrom = keepFrom;
      this.port.postMessage({ type: 'evicted', frame: keepFrom });
    }
  }

  process(_inputs, outputs) {
    const frameCount = outputs[0]?.[0]?.length ?? 128;
    for (const output of outputs) for (const channel of output) channel.fill(0);
    if (this.stretch?.poll()) this.reportStretch(true);
    if (this.scratch.active || this.scratch.gain > 0.000001) {
      this.renderScratch(outputs, frameCount);
      this.evictBlocks(this.scratch.position);
      return true;
    }
    if (!this.playing) {
      this.wetMix = 0;
      this.wetWarmFrames = 0;
      this.stretchPrimed = false;
      return true;
    }
    // Only meaningful relative to the frames already fed; restart it at a re-prime.
    if (!this.stretchPrimed) this.wrapOffset = 0;
    const virtualStart = this.position + this.wrapOffset;
    const rendered = this.renderDry(outputs, frameCount);
    if (rendered > 0) this.mixKeyLock(outputs, virtualStart, rendered);
    this.evictBlocks(this.position);
    return true;
  }
}

registerProcessor('viib-stem-transport', ViibStemTransportProcessor);
