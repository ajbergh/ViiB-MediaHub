/** Podcast listening state remains in podcast SQLite, not music listening events.
 * This small controller serializes revision-checked writes from the single
 * shared music/Plex/podcast audio transport. */
import { PodcastAPIError, podcastsAPI } from '../services/podcasts';
import type { Song } from '../types';

export class PodcastProgressTracker {
  readonly episodeId: string;
  private positionMs: number;
  private listenedMs: number;
  private savedPositionMs: number;
  private savedListenedMs: number;
  private revision: number;
  private completed: boolean;
  private savedCompleted: boolean;
  private lastSeconds: number | null = null;
  private lastWrite = Date.now();
  private writing = false;
  private queued = false;
  private blocked = false;
  private onError: (message: string) => void;

  constructor(song: Song, onError: (message: string) => void = () => {}) {
    if (!song.podcastEpisodeId) throw new Error('Podcast episode ID is required');
    this.episodeId = song.podcastEpisodeId;
    this.positionMs = song.podcastResumeMs ?? 0;
    this.savedPositionMs = this.positionMs;
    this.listenedMs = song.podcastListenedMs ?? 0;
    this.savedListenedMs = this.listenedMs;
    this.revision = song.podcastProgressRevision ?? 0;
    this.completed = song.podcastIsPlayed ?? false;
    this.savedCompleted = this.completed;
    this.onError = onError;
  }

  observe(seconds: number, playing: boolean, seeking: boolean): void {
    if (!Number.isFinite(seconds) || seconds < 0) return;
    if (this.lastSeconds !== null && playing && !seeking) {
      const delta = seconds - this.lastSeconds;
      // Seeking should not inflate time listened or write a fake position.
      if (delta > 0 && delta <= 5) this.listenedMs += Math.round(delta * 1000);
    }
    this.lastSeconds = seeking || !playing ? null : seconds;
    this.positionMs = Math.round(seconds * 1000);
    if (playing && this.completed) this.completed = false;
    if (Date.now() - this.lastWrite >= 10000 && !this.writing) void this.flush();
  }

  seek(seconds: number): void {
    if (!Number.isFinite(seconds) || seconds < 0) return;
    this.positionMs = Math.round(seconds * 1000);
    this.lastSeconds = null;
  }

  finished(seconds: number): void {
    this.seek(seconds);
    this.completed = true;
    void this.flush();
  }

  async flush(): Promise<void> {
    if (this.blocked) return;
    if (this.writing) { this.queued = true; return; }
    this.writing = true;
    try {
      do {
        this.queued = false;
        const positionMs = this.positionMs, listenedMs = this.listenedMs;
        const isPlayed = this.completed;
        if (positionMs === this.savedPositionMs &&
          listenedMs <= this.savedListenedMs && isPlayed === this.savedCompleted) break;
        try {
          const result = await podcastsAPI.setProgress(this.episodeId, {
            positionMs, listenedMs, isPlayed, expectedRevision: this.revision,
          });
          this.revision = result.revision;
          this.savedPositionMs = result.positionMs;
          this.savedListenedMs = result.listenedMs;
          this.savedCompleted = result.isPlayed;
        } catch (cause) {
          this.blocked = cause instanceof PodcastAPIError && cause.status === 409;
          this.onError(this.blocked
            ? 'Podcast progress changed in another session. Reload to avoid overwriting it.'
            : 'Could not save podcast listening progress.');
          break;
        } finally {
          this.lastWrite = Date.now();
        }
      } while (this.queued && !this.blocked);
    } finally {
      this.writing = false;
    }
  }
}
