/** Horizontal movement of waveform content underneath a fixed playhead. */
export class WaveformScratchGesture {
  private velocity = 0;
  constructor(private x: number, private time: number, private secondsPerPixel: number) {}

  move(x: number, time: number) {
    const delta = (this.x - x) * this.secondsPerPixel;
    const elapsed = Math.max(1, time - this.time);
    const rate = Math.max(-8, Math.min(8, delta * 1000 / elapsed));
    const weight = 1 - Math.exp(-elapsed / 24);
    this.velocity = this.velocity * rate < 0 || elapsed > 80
      ? rate : this.velocity + (rate - this.velocity) * weight;
    this.x = x;
    this.time = time;
    return { delta, rate };
  }

  release(time: number, cancelled: boolean) {
    return !cancelled && time - this.time < 80 ? this.velocity : 0;
  }
}
