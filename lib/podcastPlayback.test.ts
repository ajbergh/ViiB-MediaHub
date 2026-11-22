import { describe, expect, it } from 'vitest';
import { podcastEpisodeToTrack } from './podcastPlayback';
import type { PodcastEpisode, PodcastFeed } from '../services/podcasts';

describe('podcast playback adaptation', () => {
  it('uses the existing music/Plex queue shape with an internal media URL', () => {
    const feed = { id: 'feed:1', title: 'A show', author: 'Host', imageUrl: 'https://publisher.example/art.png' } as PodcastFeed;
    const episode = { id: 'ep:1', title: 'Chapter 1', durationMs: 1800000,
      positionMs: 12000, listenedMs: 9000, progressRevision: 3, isPlayed: false } as PodcastEpisode;
    const song = podcastEpisodeToTrack(episode, feed);
    expect(song.source).toBe('podcast');
    expect(song.playbackContext).toBe('podcast');
    expect(song.podcastEpisodeId).toBe('ep:1');
    expect(song.url).toBe('/api/v2/podcasts/episodes/ep%3A1/media');
    expect(song.coverUrl).toBe('/api/v2/podcasts/feeds/feed%3A1/artwork');
    expect(song.duration).toBe(1800);
    expect(song.podcastResumeMs).toBe(12000);
    expect(song.podcastProgressRevision).toBe(3);
    expect(song.spotifyId).toBeUndefined();
  });
});
