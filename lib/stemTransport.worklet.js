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
    this.port.onmessage = ({ data }) => this.command(data);
  }

  command(message) {
    if (message.type === 'configure') {
      this.generation = message.generation;
      this.sourceSampleRate = message.sampleRate;
      this.channels = message.channels;
      this.frames = message.frames;
      this.position = 0;
      this.blocks.clear();
      this.playing = false;
      this.loop = { startFrame: 0, endFrame: 0, enabled: false };
    } else if (message.type === 'flush') {
      this.generation = message.generation ?? this.generation;
      this.blocks.clear();
      this.playing = false;
    } else if (message.generation !== undefined && message.generation !== this.generation) {
      return;
    } else if (message.type === 'frames') {
      this.blocks.set(message.startFrame, { start: message.startFrame, count: message.frameCount, channels: message.channels, samples: new Float32Array(message.data) });
      this.underrun = false;
    } else if (message.type === 'seek') {
      this.position = Math.max(0, Math.min(this.frames, message.frame));
      this.underrun = false;
      this.sincePositionReport = 0;
    } else if (message.type === 'rate') {
      this.rate = Math.max(0.5, Math.min(1.5, message.rate || 1));
    } else if (message.type === 'loop') {
      this.loop = { startFrame: message.startFrame, endFrame: message.endFrame, enabled: message.enabled && message.endFrame > message.startFrame };
    } else if (message.type === 'play') {
      this.rate = Math.max(0.5, Math.min(1.5, message.rate || this.rate));
      this.playing = true;
    } else if (message.type === 'pause') {
      this.playing = false;
    }
  }

  findBlock(frame) {
    for (const block of this.blocks.values()) {
      if (frame >= block.start && frame < block.start + block.count) return block;
    }
    return null;
  }

  sampleAt(block, frame, bus, channel) {
    if (!block || frame < block.start || frame >= block.start + block.count) return null;
    const offset = ((frame - block.start) * 4 + bus) * block.channels;
    if (channel >= block.channels) return block.samples[offset] ?? 0;
    return block.samples[offset + channel] ?? 0;
  }

  process(_inputs, outputs) {
    const frameCount = outputs[0]?.[0]?.length ?? 128;
    for (const output of outputs) for (const channel of output) channel.fill(0);
    if (!this.playing) return true;

    for (let index = 0; index < frameCount; index++) {
      if (this.loop.enabled && this.position >= this.loop.endFrame) {
        this.position = this.loop.startFrame + ((this.position - this.loop.startFrame) % (this.loop.endFrame - this.loop.startFrame));
      }
      if (this.position >= this.frames) {
        this.position = this.frames;
        this.playing = false;
        this.port.postMessage({ type: 'ended', frame: this.position });
        break;
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
    const keepFrom = Math.floor(this.position) - 1;
    for (const [start, block] of this.blocks) {
      const intersectsLoop = this.loop.enabled && start < this.loop.endFrame && start + block.count > this.loop.startFrame;
      if (start + block.count < keepFrom && !intersectsLoop) this.blocks.delete(start);
    }
    return true;
  }
}

registerProcessor('viib-stem-transport', ViibStemTransportProcessor);
