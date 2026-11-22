/**
 * First typed public-RSS Podcast API client. Subscriptions and episode
 * metadata are backend-owned; do not persist them in the music Zustand store.
 * The player/download/discovery APIs will be added only as implemented.
 */
const BASE = '/api/v2/podcasts';

export interface PodcastEpisode {
  id: string;
  feedId: string;
  guid: string;
  title: string;
  description: string;
  enclosureUrl: string;
  enclosureType: string;
  publishedAt: number;
  durationMs: number;
}
export interface PodcastFeed {
  id: string;
  url: string;
  title: string;
  author: string;
  description: string;
  imageUrl: string;
  language: string;
  episodeCount: number;
}
export interface PodcastPreview {
  feed: {
    url: string;
    title: string;
    author?: string;
    description?: string;
    imageUrl?: string;
    language?: string;
    episodes: Array<{
      guid?: string;
      title: string;
      description?: string;
      enclosureUrl?: string;
      enclosureType?: string;
      durationSeconds?: number;
    }>;
  };
  episodeCount: number;
}
export class PodcastAPIError extends Error {
  constructor(readonly status: number, readonly code: string, message: string) {
    super(message);
    this.name = 'PodcastAPIError';
  }
}
async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(BASE + path, {
    ...options,
    headers: { 'Accept': 'application/json', ...(options?.body ? { 'Content-Type': 'application/json' } : {}), ...options?.headers },
  });
  if (!response.ok) {
    const payload: { error?: { code?: string; message?: string } } =
      await response.json().catch(() => ({}));
    throw new PodcastAPIError(
      response.status,
      payload.error?.code ?? 'podcast_request_failed',
      payload.error?.message ?? 'The podcast request could not be completed',
    );
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
export const podcastsAPI = {
  listSubscriptions(signal?: AbortSignal): Promise<{ feeds: PodcastFeed[] }> {
    return request('/feeds', { signal });
  },
  previewFeed(url: string, signal?: AbortSignal): Promise<PodcastPreview> {
    return request('/feeds/preview', { method: 'POST', body: JSON.stringify({ url }), signal });
  },
  subscribe(url: string, signal?: AbortSignal): Promise<{ id: string; title: string; episodeCount: number }> {
    return request('/feeds', { method: 'POST', body: JSON.stringify({ url }), signal });
  },
  listEpisodes(feedId: string, signal?: AbortSignal): Promise<{ episodes: PodcastEpisode[] }> {
    return request('/feeds/' + encodeURIComponent(feedId) + '/episodes', { signal });
  },
  unsubscribe(feedId: string, signal?: AbortSignal): Promise<void> {
    return request('/feeds/' + encodeURIComponent(feedId), { method: 'DELETE', signal });
  },
};
