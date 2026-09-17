export function vinylMomentum(initialRate, targetRate, elapsed) {
  const initial = Math.max(-8, Math.min(8, initialRate));
  const duration = Math.min(1.6, 0.5 + Math.abs(initial - targetRate) * 0.14);
  const time = Math.max(0, Math.min(duration, elapsed));
  const remaining = 1 - time / duration;
  return {
    distance: targetRate * time + (initial - targetRate) * duration / 4 * (1 - remaining ** 4),
    done: elapsed >= duration,
  };
}

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
    this.coast = null;
    this.token = 0;
    this.smoothing = 1 - Math.exp(-1 / (sampleRate * 0.002));
  }

  command(message) {
    if (message.type === 'load') {
      this.coast = null;
      this.channels = message.channels;
      this.active = false;
      this.gain = 0;
    } else if (message.type === 'start') {
      this.coast = null;
      this.token = message.token ?? 0;
      this.position = this.target = message.position * this.sampleRate;
      this.rate = this.gain = 0;
      this.active = true;
    } else if (message.type === 'coast') {
      this.token = message.token;
      this.coast = { initial: message.velocity, target: message.targetRate, frames: 0,
        duration: Math.min(1.6, 0.5 + Math.abs(message.velocity - message.targetRate) * 0.14),
        distance: 0, done: false, shadow: message.shadowPosition, shadowRate: message.shadowRate };
    } else if (message.type === 'hold') {
      this.token = message.token;
      this.coast = null;
      this.target = this.position;
      this.rate = 0;
    } else if (message.type === 'move') {
      this.target = Math.max(0, Math.min((this.channels[0]?.length || 1) - 1,
        Number.isFinite(message.delta) ? this.target + message.delta * this.sampleRate : message.position * this.sampleRate));
    } else if (message.type === 'stop') {
      this.active = false;
      this.coast = null;
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
      let desiredRate = this.active
        ? Math.max(-8, Math.min(8, (this.target - this.position) / (this.sampleRate * 0.008))) : 0;
      if (this.coast && this.active) {
        const coast = this.coast;
        coast.frames++;
        const elapsed = coast.frames / this.sampleRate;
        if (coast.done) {
          desiredRate = coast.target;
        } else {
          // Integrate the curve without allocating objects in the sample loop.
          const time = Math.min(coast.duration, elapsed);
          const remaining = 1 - time / coast.duration;
          const distance = coast.target * time + (coast.initial - coast.target) * coast.duration / 4 * (1 - remaining ** 4);
          desiredRate = (distance - coast.distance) * this.sampleRate;
          coast.distance = distance;
          if (elapsed >= coast.duration) {
            coast.done = true;
            if (Number.isFinite(coast.shadow)) this.position = (coast.shadow + elapsed * coast.shadowRate) * this.sampleRate;
          }
        }
      }
      this.rate += (desiredRate - this.rate) * this.smoothing;
      const audible = this.active && Math.abs(this.rate) > 0.001 && length > 1 &&
        !(this.position <= 0 && this.rate < 0) && !(this.position >= length - 1 && this.rate > 0);
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
      this.reportFrames = 0;
      this.reportedToken = -1;
      this.port.onmessage = event => {
        this.transport.command(event.data);
        if (event.data.type === 'load') this.port.postMessage({ type: 'ready' });
        if (event.data.type === 'hold') this.port.postMessage({ type: 'held', token: this.transport.token,
          position: this.transport.position / sampleRate });
      };
    }
    process(_inputs, outputs) {
      this.transport.render(outputs[0]);
      this.reportFrames += outputs[0][0].length;
      const t = this.transport;
      const settled = t.coast?.done && this.reportedToken !== t.token;
      if (t.coast && (settled || this.reportFrames >= sampleRate / 30)) {
        this.reportFrames = 0;
        if (settled) this.reportedToken = t.token;
        this.port.postMessage({ type: settled ? 'settled' : 'position', token: t.token,
          position: t.position / sampleRate, rate: t.rate,
          time: currentTime + outputs[0][0].length / sampleRate });
      }
      return true;
    }
  }
  registerProcessor('vinyl-scratch', VinylScratchProcessor);
}
