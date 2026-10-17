type DrawJob = (timestamp: number, elapsed: number) => boolean;

/** A single visual clock for all subscribed meters. Returning false lets the
 * clock sleep when every meter is idle; audio/transport clocks stay separate. */
export class DJVisualScheduler {
  private jobs = new Set<DrawJob>();
  private frame = 0;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private previous = 0;
  subscribe(job: DrawJob): () => void {
    this.jobs.add(job);
    if (this.timer) { clearTimeout(this.timer); this.timer = undefined; }
    if (!this.frame) this.frame = requestAnimationFrame(this.draw);
    return () => {
      this.jobs.delete(job);
      if (!this.jobs.size) {
        cancelAnimationFrame(this.frame); this.frame = 0;
        if (this.timer) clearTimeout(this.timer);
        this.timer = undefined; this.previous = 0;
      }
    };
  }
  private draw = (now: number): void => {
    this.frame = 0;
    const elapsed = this.previous ? Math.max(1, Math.min(100, now - this.previous)) : 1000 / 60;
    this.previous = now;
    let active = false;
    for (const job of this.jobs) active = job(now, elapsed) || active;
    if (!this.jobs.size) return;
    const hidden = typeof document !== 'undefined' && document.hidden;
    if (!active || hidden) {
      this.timer = setTimeout(() => { this.timer = undefined; this.frame = requestAnimationFrame(this.draw); }, hidden ? 1000 : 250);
    } else this.frame = requestAnimationFrame(this.draw);
  };
}

export const djVisualScheduler = new DJVisualScheduler();
