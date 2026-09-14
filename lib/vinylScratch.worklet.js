// Sample-domain transport: signed motion reads the actual recording backwards
// and forwards. No pitch preservation, media seeks, or synthesized scratch noise.
export class VinylScratchTransport {
  constructor(sampleRate) {
    this.sampleRate = sampleRate;
    this.channels = [];
    this.position = 0;
    this.target = 0;
    this.rate = 0;
    this.gain = 0;
    this.active = false;
    this.smoothing = 1 - Math.exp(-1 / (sampleRate * 0.002));
  }

  command(message) {
    if (message.type === 'load') {
      this.channels = message.channels;
      this.active = false;
      this.gain = 0;
    } else if (message.type === 'start') {
      this.position = this.target = message.position * this.sampleRate;
      this.rate = this.gain = 0;
      this.active = true;
    } else if (message.type === 'move') {
      this.target = Math.max(0, Math.min((this.channels[0]?.length || 1) - 1,
        message.position * this.sampleRate));
    } else if (message.type === 'stop') {
      this.active = false;
    }
  }

  render(output) {
    if (!this.active && this.gain < 0.000001) {
      for (const channel of output) channel.fill(0);
      return;
    }
    const length = this.channels[0]?.length || 0;
    for (let i = 0; i < output[0].length; i++) {
      // A short position servo bridges pointer events without discontinuous
      // sample jumps. It settles to silence when the hand stops moving.
      const desiredRate = this.active
        ? Math.max(-8, Math.min(8, (this.target - this.position) / (this.sampleRate * 0.008))) : 0;
      this.rate += (desiredRate - this.rate) * this.smoothing;
      const audible = this.active && Math.abs(this.rate) > 0.001 && length > 1;
      this.gain += ((audible ? 1 : 0) - this.gain) * this.smoothing;
      this.position = Math.max(0, Math.min(Math.max(0, length - 1), this.position + this.rate));
      const index = Math.floor(this.position);
      const fraction = this.position - index;
      for (let ch = 0; ch < output.length; ch++) {
        const samples = this.channels[Math.min(ch, this.channels.length - 1)];
        output[ch][i] = samples && length > 1
          ? (samples[index] + (samples[Math.min(index + 1, length - 1)] - samples[index]) * fraction) * this.gain : 0;
      }
    }
  }
}

if (typeof registerProcessor === 'function') {
  class VinylScratchProcessor extends AudioWorkletProcessor {
    constructor() {
      super();
      this.transport = new VinylScratchTransport(sampleRate);
      this.port.onmessage = event => this.transport.command(event.data);
    }
    process(_inputs, outputs) {
      this.transport.render(outputs[0]);
      return true;
    }
  }
  registerProcessor('vinyl-scratch', VinylScratchProcessor);
}
