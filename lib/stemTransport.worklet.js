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
 * buses, so the buses stay phase-coherent. Each quantum it re-reads the
 * transport's own frames ahead of the playhead (the package's buffer-mode
 * technique), so steady playback is time-aligned with the dry path.
 */
class StemStretch {
  constructor(Core) {
    this.core = new Core({ numberOfInputs: 1, numberOfOutputs: 1, outputChannelCount: [BUSES * 2] });
    this.ready = false;
    this.output = [];
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
    this.ready = true;
    return true;
  }

  memory() {
    const wasm = this.core.wasmModule;
    return wasm.exports ? wasm.exports.memory.buffer : wasm.HEAP8.buffer;
  }

  inputViews() {
    const memory = this.memory();
    return this.core.buffersIn.map(pointer => new Float32Array(memory, pointer, this.core.bufferLength));
  }

  render(rate, frameCount) {
    const wasm = this.core.wasmModule;
    wasm._seek(this.core.bufferLength, rate);
    wasm._process(0, frameCount);
    const memory = this.memory();
    if (this.output[0]?.length !== frameCount) this.output = Array.from({ length: BUSES * 2 }, () => new Float32Array(frameCount));
    this.core.buffersOut.forEach((pointer, channel) => this.output[channel].set(new Float32Array(memory, pointer, frameCount)));
    return this.output;
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
    this.underrun = false;
    this.sincePositionReport = 0;
    this.retainedFrom = 0;
    this.keyLock = false;
    this.wetMix = 0;
    this.wetWarmFrames = 0;
    this.wetSmoothing = 1 - Math.exp(-1 / (this.outputSampleRate * 0.01));
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
  }

  command(message) {
    if (message.type === 'configure') {
      this.generation = message.generation;
      this.sourceSampleRate = message.sampleRate;
      this.channels = message.channels;
      this.frames = message.frames;
      this.position = 0;
      this.blocks.clear();
      this.retainedFrom = 0;
      this.playing = false;
      this.loop = { startFrame: 0, endFrame: 0, enabled: false };
      this.scratch.active = false;
      this.scratch.coast = null;
      this.scratch.gain = 0;
      this.discontinuity();
    } else if (message.type === 'flush') {
      this.generation = message.generation ?? this.generation;
      this.blocks.clear();
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
      // The stretcher's read-ahead may already span the old loop boundary.
      this.wetWarmFrames = 0;
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

  findBlock(frame) {
    for (const block of this.blocks.values()) {
      if (frame >= block.start && frame < block.start + block.count) return block;
    }
    return null;
  }

  nextBlockStart(frame) {
    let next = Infinity;
    for (const block of this.blocks.values()) if (block.start > frame && block.start < next) next = block.start;
    return next;
  }

  sampleAt(block, frame, bus, channel) {
    if (!block || frame < block.start || frame >= block.start + block.count) return null;
    const offset = ((frame - block.start) * 4 + bus) * block.channels;
    if (channel >= block.channels) return block.samples[offset] ?? 0;
    return block.samples[offset + channel] ?? 0;
  }

  // Frames past the loop end belong to the next pass of the loop.
  wrapFrame(frame) {
    const loop = this.loop;
    if (!loop.enabled || frame < loop.endFrame) return frame;
    return loop.startFrame + ((frame - loop.startFrame) % (loop.endFrame - loop.startFrame));
  }

  /** Interpolated four-bus read at a fractional source frame; missing data is silence. */
  readInterpolated(position, out, index, gain, wrap) {
    const frame = Math.floor(position);
    const fraction = position - frame;
    const current = wrap ? this.wrapFrame(frame) : frame;
    const next = wrap ? this.wrapFrame(frame + 1) : Math.min(frame + 1, Math.max(0, this.frames - 1));
    const block = this.findBlock(current);
    if (!block) {
      for (let bus = 0; bus < BUSES; bus++) { out[bus * 2][index] = 0; out[bus * 2 + 1][index] = 0; }
      return false;
    }
    const nextBlock = next >= block.start && next < block.start + block.count ? block : this.findBlock(next);
    for (let bus = 0; bus < BUSES; bus++) {
      for (let channel = 0; channel < 2; channel++) {
        const a = this.sampleAt(block, current, bus, channel) ?? 0;
        const b = this.sampleAt(nextBlock, next, bus, channel) ?? a;
        out[bus * 2 + channel][index] = (a + (b - a) * fraction) * gain;
      }
    }
    return true;
  }

  /** Deinterleave `count` whole frames (loop-wrapped) into eight channel views. */
  copyFrames(views, firstFrame, count) {
    let offset = 0;
    while (offset < count) {
      let frame = this.wrapFrame(firstFrame + offset);
      let run = count - offset;
      if (this.loop.enabled && frame < this.loop.endFrame) run = Math.min(run, this.loop.endFrame - frame);
      if (frame < 0 || frame >= this.frames) {
        run = frame < 0 ? Math.min(run, -frame) : run;
        for (const view of views) view.fill(0, offset, offset + run);
      } else {
        const block = this.findBlock(frame);
        if (!block) {
          run = Math.min(run, this.nextBlockStart(frame) - frame, this.frames - frame);
          for (const view of views) view.fill(0, offset, offset + run);
        } else {
          run = Math.min(run, block.start + block.count - frame);
          const channels = block.channels;
          const stride = BUSES * channels;
          const right = channels > 1 ? 1 : 0;
          const samples = block.samples;
          let source = (frame - block.start) * stride;
          for (let i = 0; i < run; i++, source += stride) {
            const at = offset + i;
            for (let bus = 0; bus < BUSES; bus++) {
              const base = source + bus * channels;
              views[bus * 2][at] = samples[base];
              views[bus * 2 + 1][at] = samples[base + right];
            }
          }
        }
      }
      offset += run;
    }
  }

  /** Stretcher input window ending where its output latency will have carried the playhead. */
  fillStretchWindow(startPosition) {
    const stretch = this.stretch;
    const views = stretch.inputViews();
    const length = views[0].length;
    const ratio = this.sourceSampleRate / this.outputSampleRate;
    const end = startPosition + ratio * (stretch.outputLatency * this.rate + stretch.inputLatency);
    if (ratio === 1) {
      this.copyFrames(views, Math.round(end) - length, length);
      return;
    }
    for (let i = 0; i < length; i++) this.readInterpolated(end - (length - i) * ratio, views, i, 1, true);
  }

  renderScratch(outputs, frameCount) {
    const scratch = this.scratch;
    const sampleRate = this.outputSampleRate;
    const step = this.sourceSampleRate / sampleRate;
    const last = Math.max(0, this.frames - 1);
    const out = [outputs[0][0], outputs[0][1], outputs[1][0], outputs[1][1], outputs[2][0], outputs[2][1], outputs[3][0], outputs[3][1]];
    for (let i = 0; i < frameCount; i++) {
      // A short position servo bridges pointer events without sample jumps and
      // settles to silence when the hand stops moving.
      let desiredRate = scratch.active
        ? Math.max(-8, Math.min(8, (scratch.target - scratch.position) / (this.sourceSampleRate * 0.008))) : 0;
      const coast = scratch.coast;
      if (coast && scratch.active) {
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
            if (Number.isFinite(coast.shadow)) scratch.position = (coast.shadow + elapsed * coast.shadowRate) * this.sourceSampleRate;
          }
        }
      }
      scratch.rate += (desiredRate - scratch.rate) * this.scratchSmoothing;
      const audible = scratch.active && Math.abs(scratch.rate) > 0.001 && this.frames > 1 &&
        !(scratch.position <= 0 && scratch.rate < 0) && !(scratch.position >= last && scratch.rate > 0);
      scratch.gain += ((audible ? 1 : 0) - scratch.gain) * this.scratchSmoothing;
      scratch.position = Math.max(0, Math.min(last, scratch.position + scratch.rate * step));
      this.readInterpolated(scratch.position, out, i, scratch.gain, false);
    }
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
    for (let index = 0; index < frameCount; index++) {
      if (this.loop.enabled && this.position >= this.loop.endFrame) {
        this.position = this.loop.startFrame + ((this.position - this.loop.startFrame) % (this.loop.endFrame - this.loop.startFrame));
      }
      if (this.position >= this.frames) {
        this.position = this.frames;
        this.playing = false;
        this.port.postMessage({ type: 'ended', frame: this.position });
        return index;
      }
      const frame = Math.floor(this.position);
      const block = this.findBlock(frame);
      if (block) {
        let nextFrame = frame + 1;
        if (this.loop.enabled && nextFrame >= this.loop.endFrame) nextFrame = this.loop.startFrame;
        else if (nextFrame >= this.frames) nextFrame = frame;
        const nextBlock = nextFrame < block.start + block.count ? block : this.findBlock(nextFrame);
        const fraction = this.position - frame;
        for (let bus = 0; bus < 4; bus++) {
          for (let channel = 0; channel < 2; channel++) {
            const current = this.sampleAt(block, frame, bus, channel) ?? 0;
            const next = this.sampleAt(nextBlock, nextFrame, bus, channel) ?? current;
            outputs[bus][channel][index] = current + (next - current) * fraction;
          }
        }
        this.underrun = false;
      } else if (!this.underrun) {
        this.underrun = true;
        this.port.postMessage({ type: 'underrun', frame });
      }
      // Resample package frames onto the AudioContext clock. Tempo remains a
      // varispeed multiplier on top of the source/output-rate conversion.
      this.position += (this.sourceSampleRate / this.outputSampleRate) * this.rate;
      this.sincePositionReport++;
      if (this.sincePositionReport >= 4096) {
        this.sincePositionReport = 0;
        this.port.postMessage({ type: 'position', frame: this.position });
      }
    }
    return frameCount;
  }

  /** Crossfade the key-locked render over the dry varispeed render once it is valid. */
  mixKeyLock(outputs, startPosition, rendered) {
    const stretch = this.stretch;
    if (!stretch?.poll()) return;
    this.reportStretch(true);
    const wanted = this.keyLock && Math.abs(this.rate - 1) > KEY_LOCK_MIN_DEVIATION;
    if (!wanted && this.wetMix === 0) {
      this.wetWarmFrames = 0;
      return;
    }
    this.fillStretchWindow(startPosition);
    const wet = stretch.render(this.rate, rendered);
    this.wetWarmFrames += rendered;
    const target = wanted && this.wetWarmFrames > stretch.outputLatency ? 1 : 0;
    let mix = this.wetMix;
    for (let index = 0; index < rendered; index++) {
      mix += (target - mix) * this.wetSmoothing;
      for (let bus = 0; bus < BUSES; bus++) {
        for (let channel = 0; channel < 2; channel++) {
          const dry = outputs[bus][channel][index];
          outputs[bus][channel][index] = dry + (wet[bus * 2 + channel][index] - dry) * mix;
        }
      }
    }
    this.wetMix = target === 0 && mix < 0.0001 ? 0 : mix;
  }

  evictBlocks(position) {
    const keepFrom = Math.floor(position) - 1 - Math.floor(HISTORY_SECONDS * this.sourceSampleRate);
    let evicted = false;
    for (const [start, block] of this.blocks) {
      const intersectsLoop = this.loop.enabled && start < this.loop.endFrame && start + block.count > this.loop.startFrame;
      if (start + block.count < keepFrom && !intersectsLoop) {
        this.blocks.delete(start);
        evicted = true;
      }
    }
    if (evicted && keepFrom > this.retainedFrom) {
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
      return true;
    }
    const startPosition = this.position;
    const rendered = this.renderDry(outputs, frameCount);
    if (rendered > 0) this.mixKeyLock(outputs, startPosition, rendered);
    this.evictBlocks(this.position);
    return true;
  }
}

registerProcessor('viib-stem-transport', ViibStemTransportProcessor);
