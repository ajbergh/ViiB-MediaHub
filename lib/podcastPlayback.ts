/** Adapts podcast episode metadata to the existing ViiB audio player.
 * Queue entries are ephemeral; they are never inserted into music tables. */
import type { Song } from '../types';
import type { PodcastEpisode, PodcastFeed } from '../services/podcasts';
import { podcastsAPI } from '../services/podcasts';

export function podcastEpisodeToTrack(episode: PodcastEpisode, feed: PodcastFeed): Song {
  return {
    id: 'podcast_' + episode.id,
    source: 'podcast',
    podcastEpisodeId: episode.id,
    podcastResumeMs: episode.positionMs ?? 0,
    podcastListenedMs: episode.listenedMs ?? 0,
    podcastProgressRevision: episode.progressRevision ?? 0,
    podcastIsPlayed: episode.isPlayed ?? false,
    playbackContext: 'podcast',
    title: episode.title,
    artist: feed.author || feed.title,
    album: feed.title,
    coverUrl: feed.imageUrl ? '/api/v2/podcasts/feeds/' + encodeURIComponent(feed.id) + '/artwork' : undefined,
    url: podcastsAPI.mediaURL(episode.id),
    duration: Math.max(0, (episode.durationMs || 0) / 1000),
    addedAt: episode.publishedAt || 0,
  };
}
