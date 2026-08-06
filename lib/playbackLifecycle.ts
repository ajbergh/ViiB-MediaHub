import type { PlaybackContext } from '../types';

export const resolvePlaybackContext = (
  songContext?: PlaybackContext,
  previousContext?: PlaybackContext | null,
): PlaybackContext => songContext ?? previousContext ?? 'queue';

export const normalizeCrossfadeDuration = (
  requested: number | undefined,
  gapless: boolean,
): number => {
  if (gapless) return 0;
  if (!Number.isFinite(requested)) return 0.2;
  return Math.max(0, requested ?? 0.2);
};

export const isActivePlaybackEvent = (
  eventTarget: EventTarget | null,
  activeElement: HTMLAudioElement | null,
): boolean => Boolean(activeElement && eventTarget === activeElement);

export class ManagedObjectUrlRegistry {
  private readonly urls = new Set<string>();

  create(blob: Blob): string {
    const url = URL.createObjectURL(blob);
    this.urls.add(url);
    return url;
  }

  release(url?: string | null): void {
    if (!url || !this.urls.delete(url)) return;
    URL.revokeObjectURL(url);
  }

  releaseAll(): void {
    for (const url of this.urls) URL.revokeObjectURL(url);
    this.urls.clear();
  }
}


export const calculateReplayGain = (gainDb?: number, peak?: number): number => {
  if (!Number.isFinite(gainDb)) return 1;
  const clampedDb = Math.max(-24, Math.min(12, gainDb ?? 0));
  let linear = Math.pow(10, clampedDb / 20);
  if (Number.isFinite(peak) && (peak ?? 0) > 0 && linear * (peak ?? 0) > 1) {
    linear = 1 / (peak ?? 1);
  }
  return Math.max(0.05, Math.min(4, linear));
};
