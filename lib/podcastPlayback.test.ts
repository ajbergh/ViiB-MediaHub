import { describe, expect, it, vi } from 'vitest';
import { podcastEpisodeToTrack, queuePodcastEpisode } from './podcastPlayback';
import type { PodcastEpisode, PodcastFeed } from '../services/podcasts';
import type { Song } from '../types';

describe('podcast playback adaptation', () => {
  it('uses the existing music/Plex queue shape with an internal media URL', () => {
    const feed = { id: 'feed:1', title: 'A show', author: 'Host', imageUrl: 'https://publisher.example/art.png',
      defaultSpeed: 1.5, skipIntroSeconds: 25, skipOutroSeconds: 40 } as PodcastFeed;
    const episode = { id: 'ep:1', title: 'Chapter 1', durationMs: 1800000,
      positionMs: 12000, listenedMs: 9000, progressRevision: 3, isPlayed: false } as PodcastEpisode;
    const song = podcastEpisodeToTrack(episode, feed);
    expect(song.source).toBe('podcast');
    expect(song.playbackContext).toBe('podcast');
    expect(song.podcastEpisodeId).toBe('ep:1');
    expect(song.podcastFeedId).toBe('feed:1');
    expect(song.podcastDefaultSpeed).toBe(1.5);
    expect(song.podcastSkipIntroSeconds).toBe(25);
    expect(song.podcastSkipOutroSeconds).toBe(40);
    expect(song.url).toBe('/api/v2/podcasts/episodes/ep%3A1/media');
    expect(song.coverUrl).toBe('/api/v2/podcasts/feeds/feed%3A1/artwork');
    expect(song.duration).toBe(1800);
    expect(song.podcastResumeMs).toBe(12000);
    expect(song.podcastProgressRevision).toBe(3);
    expect(song.spotifyId).toBeUndefined();
  });
});

// Both ordering actions must preserve the current music/Plex session;
// podcast-only metadata is passed through the common queue contract.
describe('podcast queue adaptation', () => {
  const feed = { id: 'show-1', title: 'A podcast', author: 'Host', imageUrl: '' } as PodcastFeed;
  const episode = { id: 'ep-2', title: 'Next podcast', hasAudio: true,
    positionMs: 123000, listenedMs: 89000, progressRevision: 4 } as PodcastEpisode;
  const player = (currentSong: Song | null) => ({
    currentSong, playSong: vi.fn().mockResolvedValue(undefined),
    playNext: vi.fn(), addToQueue: vi.fn(),
  });

  it('plays an episode using its saved position if the player was idle', () => {
    const state = player(null);
    expect(queuePodcastEpisode(episode, feed, 'next', state)).toBe(true);
    expect(state.playSong).toHaveBeenCalledWith(
      expect.objectContaining({ podcastEpisodeId: 'ep-2', podcastResumeMs: 123000,
        podcastProgressRevision: 4, source: 'podcast' }),
      [expect.objectContaining({ podcastEpisodeId: 'ep-2' })], 'podcast', 0,
    );
    expect(state.playNext).not.toHaveBeenCalled();
    expect(state.addToQueue).not.toHaveBeenCalled();
  });

  it('inserts after currently playing music without replacing its session', () => {
    const state = player({ id: 'music-1' } as Song);
    expect(queuePodcastEpisode(episode, feed, 'next', state)).toBe(true);
    expect(state.playNext).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'podcast_ep-2', source: 'podcast', podcastResumeMs: 123000 }));
    expect(state.playSong).not.toHaveBeenCalled();
    expect(state.addToQueue).not.toHaveBeenCalled();
  });

  it('appends to the same queue without starting a competing player', () => {
    const state = player({ id: 'plex-1', source: 'plex' } as Song);
    expect(queuePodcastEpisode(episode, feed, 'end', state)).toBe(true);
    expect(state.addToQueue).toHaveBeenCalledWith(
      expect.objectContaining({ podcastEpisodeId: 'ep-2', url: '/api/v2/podcasts/episodes/ep-2/media' }));
    expect(state.playSong).not.toHaveBeenCalled();
    expect(state.playNext).not.toHaveBeenCalled();
  });

  it('rejects episodes without audio without modifying the queue', () => {
    const state = player(null);
    expect(queuePodcastEpisode({ ...episode, hasAudio: false }, feed, 'next', state)).toBe(false);
    expect(state.playSong).not.toHaveBeenCalled();
    expect(state.playNext).not.toHaveBeenCalled();
    expect(state.addToQueue).not.toHaveBeenCalled();
  });
});
