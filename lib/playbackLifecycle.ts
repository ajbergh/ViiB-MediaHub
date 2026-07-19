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
