import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PodcastProgressTracker } from './podcastProgressTracker';
import { podcastsAPI } from '../services/podcasts';
import type { Song } from '../types';

const track = {
  source: 'podcast', podcastEpisodeId: 'episode-1', podcastResumeMs: 10000,
  podcastListenedMs: 4000, podcastProgressRevision: 2, podcastIsPlayed: false,
} as Song;

describe('shared player podcast progress', () => {
  beforeEach(() => { vi.restoreAllMocks(); });

  it('skips untouched tracks, counts listening without counting seeks and persists CAS revision', async () => {
    const writes = vi.spyOn(podcastsAPI, 'setProgress').mockResolvedValue({
      episodeId: 'episode-1', positionMs: 23000, listenedMs: 7000, isPlayed: false,
      revision: 3, completedAt: 0, lastPlayedAt: 0,
    });
    const progress = new PodcastProgressTracker(track);
    await progress.flush();
    expect(writes).not.toHaveBeenCalled();
    progress.observe(20, true, false);
    progress.observe(23, true, false);
    progress.seek(90);
    progress.observe(90, true, true);
    await progress.flush();
    expect(writes).toHaveBeenCalledWith('episode-1', {
      positionMs: 90000, listenedMs: 7000, isPlayed: false, expectedRevision: 2,
    });
  });

  it('marks completion without logging music events', async () => {
    const write = vi.spyOn(podcastsAPI, 'setProgress').mockResolvedValue({
      episodeId: 'episode-1', positionMs: 120000, listenedMs: 4000, isPlayed: true,
      revision: 3, completedAt: 1, lastPlayedAt: 1,
    });
    const tracker = new PodcastProgressTracker(track);
    tracker.finished(120);
    await vi.waitFor(() => expect(write).toHaveBeenCalledWith('episode-1', {
      positionMs: 120000, listenedMs: 4000, isPlayed: true, expectedRevision: 2,
    }));
  });
});
