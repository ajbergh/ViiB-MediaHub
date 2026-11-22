/** Adapts podcast episode metadata to the existing ViiB audio player.
 * Queue entries are ephemeral; they are never inserted into music tables. */
import type { Song } from '../types';
import type { PlayerSlice } from '../slices/types';
import type { PodcastContinueEpisode, PodcastEpisode, PodcastFeed } from '../services/podcasts';
import { podcastsAPI } from '../services/podcasts';

export function podcastEpisodeToTrack(episode: PodcastEpisode, feed: PodcastFeed): Song {
  const continueEpisode = episode as Partial<PodcastContinueEpisode>;
  return {
    id: 'podcast_' + episode.id,
    source: 'podcast',
    podcastEpisodeId: episode.id,
    podcastFeedId: feed.id,
    podcastResumeMs: episode.positionMs ?? 0,
    podcastListenedMs: episode.listenedMs ?? 0,
    podcastProgressRevision: episode.progressRevision ?? 0,
    podcastIsPlayed: episode.isPlayed ?? false,
    podcastDefaultSpeed: feed.defaultSpeed ?? continueEpisode.defaultSpeed ?? 0,
    podcastSkipIntroSeconds: feed.skipIntroSeconds ?? continueEpisode.skipIntroSeconds ?? 0,
    podcastSkipOutroSeconds: feed.skipOutroSeconds ?? continueEpisode.skipOutroSeconds ?? 0,
    podcastHasChapters: episode.hasChapters ?? false,
    podcastHasTranscript: episode.hasTranscript ?? false,
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

// Queue changes use the existing music/Plex/podcast queue and transport.
// The podcast domain never writes to music tables or audio preferences.
type PodcastQueuePlayer = Pick<PlayerSlice, 'currentSong' | 'playSong' | 'playNext' | 'addToQueue'>;
export type PodcastQueuePosition = 'next' | 'end';

export function queuePodcastEpisode(
  episode: PodcastEpisode,
  feed: PodcastFeed,
  position: PodcastQueuePosition,
  player: PodcastQueuePlayer,
): boolean {
  if (!episode.hasAudio) return false;
  const track = podcastEpisodeToTrack(episode, feed);
  if (!player.currentSong) {
    // An idle queue has no "next" transport owner; begin playback instead of
    // leaving an unplayable queue item. Use the normal CAS-resume adapter.
    void player.playSong(track, [track], 'podcast', 0);
  } else if (position === 'next') {
    player.playNext(track);
  } else {
    player.addToQueue(track);
  }
  return true;
}
