/** Playback-speed units (1 = normal speed), seconds, and seconds of audio. */
export function vinylMomentum(initialRate: number, targetRate: number, elapsed: number) {
  const initial = Math.max(-8, Math.min(8, initialRate));
  const duration = Math.min(1.6, 0.5 + Math.abs(initial - targetRate) * 0.14);
  const time = Math.max(0, Math.min(duration, elapsed));
  const remaining = 1 - time / duration;
  // Integrate the cubic speed curve exactly so travel is independent of FPS.
  return {
    distance: targetRate * time + (initial - targetRate) * duration / 4 * (1 - remaining ** 4),
    done: elapsed >= duration,
  };
}
