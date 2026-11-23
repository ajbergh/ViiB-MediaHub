/**
 * Typed Podcast API client (`/api/v2/podcasts/*`). Subscriptions, episode
 * triage, durable queue, chapters/transcripts/bookmarks, playback defaults,
 * and offline downloads are backend-owned in SQLite; provider enclosure URLs
 * and API secrets remain backend-only.
 *
 * @module podcastsAPI
 */
const BASE = '/api/v2/podcasts';

export interface PodcastEpisode {
  id: string;
  feedId: string;
  guid: string;
  title: string;
  description: string;
  /** Publisher URLs may carry ephemeral signatures; never expose those in UI JSON. */
  hasAudio: boolean;
  enclosureType: string;
  publishedAt: number;
  durationMs: number;
  isPlayed?: boolean;
  positionMs?: number;
  listenedMs?: number;
  progressRevision?: number;
  hasChapters?: boolean;
  hasTranscript?: boolean;
  websiteUrl?: string;
  enclosureBytes?: number;
  episodeNumber?: number;
  seasonNumber?: number;
  starred?: boolean;
  archived?: boolean;
}
export interface PodcastEpisodeDetail extends PodcastEpisode {
  feedTitle: string;
  feedAuthor: string;
  richDescription: string;
  chaptersType?: string;
  transcriptType?: string;
  bookmarkCount: number;
  downloadStatus?: string;
  defaultSpeed?: number;
  skipIntroSeconds?: number;
  skipOutroSeconds?: number;
}
export interface PodcastChapter {
  startTime: number;
  endTime?: number;
  title: string;
  img?: string;
  url?: string;
  toc?: boolean;
}
export interface PodcastTranscriptCue {
  startMs: number;
  endMs: number;
  speaker?: string;
  text: string;
}
export interface PodcastBookmark {
  id: string;
  episodeId: string;
  positionMs: number;
  note: string;
  createdAt: number;
}
export type PodcastEpisodeView = 'inbox' | 'starred' | 'in_progress' | 'played' | 'archived' | 'all';
export interface PodcastViewEpisode extends PodcastEpisode {
  feedTitle: string;
  feedAuthor: string;
  starred: boolean;
  archived: boolean;
  downloadStatus?: string;
}
export interface PodcastQueueItem extends PodcastViewEpisode {
  sortKey: number;
  addedAt: number;
}
export interface PodcastRetentionPolicy {
  keepLatestCount: number;
}
export interface PodcastDirectoryStatus {
  configured: boolean;
  provider: 'podcast_index' | 'none' | string;
  keyHint: string;
  source?: 'user' | 'embedded' | 'none' | string;
}
export interface PodcastDirectoryShow {
  id: number;
  title: string;
  author?: string;
  description?: string;
  feedUrl: string;
  websiteUrl?: string;
  artworkUrl?: string;
  language?: string;
  categories?: string[];
  episodeCount?: number;
}
export interface PodcastEventsSnapshot {
  downloads: PodcastDownload[];
  storage: PodcastStorage;
  refreshCount: number;
}
export interface PodcastContinueEpisode extends PodcastEpisode {
  feedTitle: string;
  positionMs: number;
  listenedMs: number;
  progressRevision: number;
  defaultSpeed?: number;
  skipIntroSeconds?: number;
  skipOutroSeconds?: number;
}
export interface PodcastStorage {
  usedBytes: number;
  maxBytes: number;
}
export interface PodcastDownload {
  id: string;
  episodeId: string;
  /** Present for subscribed download list rows; never publisher URLs. */
  feedId?: string;
  feedTitle?: string;
  episodeTitle?: string;
  status: 'queued' | 'downloading' | 'completed' | 'failed' | 'canceled' | 'paused';
  bytesReceived: number;
  expectedBytes: number;
  verifiedSize: number;
  errorCode?: string;
  /** Scheduled timestamp in milliseconds for a bounded automatic retry. */
  retryAfter?: number;
  createdAt: number;
  updatedAt: number;
  completedAt: number;
}
export interface PodcastProgress {
  episodeId: string;
  positionMs: number;
  listenedMs: number;
  isPlayed: boolean;
  revision: number;
  completedAt: number;
  lastPlayedAt: number;
}
export interface PodcastAutoDownloadPolicy {
  enabled: boolean;
  newestCount: number;
}
export interface PodcastShowPlaybackSettings {
  defaultSpeed: number;
  skipIntroSeconds: number;
  skipOutroSeconds: number;
}
export interface PodcastGlobalPlaybackSettings {
  defaultSpeed: number;
  skipBackSeconds: number;
  skipForwardSeconds: number;
}
export interface PodcastRefreshStatus {
  lastCheckedAt: number;
  lastSuccessAt: number;
  nextRefreshAt: number;
  lastErrorCode?: string;
  failures: number;
  refreshing: boolean;
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
  defaultSpeed?: number;
  skipIntroSeconds?: number;
  skipOutroSeconds?: number;
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
export interface PodcastOPMLImportPreview {
  feeds: Array<{ title: string; url: string; alreadyFollowed: boolean }>;
  invalidCount: number;
  duplicateCount: number;
}
export class PodcastAPIError extends Error {
  constructor(readonly status: number, readonly code: string, message: string) {
    super(message);
    this.name = 'PodcastAPIError';
  }
}
async function requestResponse(path: string, options?: RequestInit): Promise<Response> {
  const response = await fetch(BASE + path, {
    ...options,
    headers: { 'Accept': 'application/json', ...(options?.body ? { 'Content-Type': 'application/json' } : {}), ...options?.headers },
  });
  if (!response.ok) {
    // Supported Go handlers return structured v2 JSON errors. HTML/text
    // failures generally indicate a stale backend or a reverse-proxy issue.
    const payload: unknown = await response.json().catch(() => null);
    const error = payload && typeof payload === 'object' && 'error' in payload
      ? (payload as { error?: unknown }).error : null;
    const detail = error && typeof error === 'object'
      ? error as { code?: unknown; message?: unknown } : null;
    const code = typeof detail?.code === 'string' ? detail.code : 'podcast_api_unavailable';
    let message = typeof detail?.message === 'string' && detail.message.trim()
      ? detail.message : '';
    if (!message) {
      switch (response.status) {
        case 404:
        case 405:
          message = `Podcasts API endpoint unavailable (HTTP ${response.status}). Rebuild and restart the Go backend from the same branch as the UI.`;
          break;
        case 502:
        case 503:
        case 504:
          message = `Local podcast backend unavailable (HTTP ${response.status}). Verify that Go is running and the development proxy uses its port.`;
          break;
        case 403:
          message = 'The local API blocked this request (HTTP 403). Check the client origin and backend security settings.';
          break;
        default:
          message = `Podcast API returned HTTP ${response.status} without a structured error. Inspect the Network response and backend logs.`;
      }
    }
    throw new PodcastAPIError(response.status, code, message);
  }
  return response;
}
async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await requestResponse(path, options);
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
export const podcastsAPI = {
  mediaURL(episodeId: string): string {
    return BASE + '/episodes/' + encodeURIComponent(episodeId) + '/media';
  },
  listDownloads(signal?: AbortSignal): Promise<{ downloads: PodcastDownload[]; storage?: PodcastStorage }> {
    return request('/downloads', { signal });
  },
  queueDownload(episodeId: string, signal?: AbortSignal): Promise<{ download: PodcastDownload }> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/download', { method: 'POST', signal });
  },
  cancelDownload(episodeId: string, signal?: AbortSignal): Promise<{ download: PodcastDownload }> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/download', { method: 'DELETE', signal });
  },
  // Completed-file DELETE returns 204; never expose local disk paths.
  removeDownload(episodeId: string, signal?: AbortSignal): Promise<void> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/download', { method: 'DELETE', signal });
  },
  getDownload(episodeId: string, signal?: AbortSignal): Promise<{ download: PodcastDownload }> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/download', { signal });
  },
  listContinueListening(limit = 12, signal?: AbortSignal): Promise<{ episodes: PodcastContinueEpisode[] }> {
    return request('/episodes/continue?limit=' + encodeURIComponent(String(limit)), { signal });
  },
  getProgress(episodeId: string, signal?: AbortSignal): Promise<PodcastProgress> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/progress', { signal });
  },
  setProgress(episodeId: string, progress: Pick<PodcastProgress, 'positionMs' | 'listenedMs' | 'isPlayed'> & { expectedRevision: number }, signal?: AbortSignal): Promise<PodcastProgress> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/progress', {
      method: 'PUT', body: JSON.stringify(progress), signal,
    });
  },
  previewOPML(xml: string, signal?: AbortSignal): Promise<PodcastOPMLImportPreview> {
    return request('/import/opml/preview', {
      method: 'POST', body: xml, headers: { 'Content-Type': 'application/xml' }, signal,
    });
  },
  async exportOPML(signal?: AbortSignal): Promise<Blob> {
    const response = await requestResponse('/export/opml', { signal, headers: { Accept: 'application/xml' } });
    return response.blob();
  },
  getFeed(feedId: string, signal?: AbortSignal): Promise<{ feed: PodcastFeed }> {
    return request('/feeds/' + encodeURIComponent(feedId), { signal });
  },
  listSubscriptions(signal?: AbortSignal): Promise<{ feeds: PodcastFeed[] }> {
    return request('/feeds', { signal });
  },
  previewFeed(url: string, signal?: AbortSignal): Promise<PodcastPreview> {
    return request('/feeds/preview', { method: 'POST', body: JSON.stringify({ url }), signal });
  },
  subscribe(url: string, signal?: AbortSignal): Promise<{ id: string; title: string; episodeCount: number }> {
    return request('/feeds', { method: 'POST', body: JSON.stringify({ url }), signal });
  },
  listEpisodes(feedId: string, signal?: AbortSignal, offset = 0, limit = 50): Promise<{ episodes: PodcastEpisode[]; nextOffset: number | null }> {
    const query = new URLSearchParams({ offset: String(offset), limit: String(limit) });
    return request('/feeds/' + encodeURIComponent(feedId) + '/episodes?' + query.toString(), { signal });
  },
  refresh(feedId: string, signal?: AbortSignal): Promise<{status: 'queued'}> {
    return request('/feeds/' + encodeURIComponent(feedId) + '/refresh', { method: 'POST', signal });
  },
  getAutoDownloadPolicy(feedId: string, signal?: AbortSignal): Promise<{ policy: PodcastAutoDownloadPolicy }> {
    return request('/feeds/' + encodeURIComponent(feedId) + '/auto-download', { signal });
  },
  setAutoDownloadPolicy(feedId: string, policy: PodcastAutoDownloadPolicy, signal?: AbortSignal): Promise<{ policy: PodcastAutoDownloadPolicy }> {
    return request('/feeds/' + encodeURIComponent(feedId) + '/auto-download', {
      method: 'PUT', body: JSON.stringify(policy), signal,
    });
  },
  getShowPlaybackSettings(feedId: string, signal?: AbortSignal): Promise<{ settings: PodcastShowPlaybackSettings }> {
    return request('/feeds/' + encodeURIComponent(feedId) + '/playback-settings', { signal });
  },
  setShowPlaybackSettings(feedId: string, settings: PodcastShowPlaybackSettings, signal?: AbortSignal): Promise<{ settings: PodcastShowPlaybackSettings }> {
    return request('/feeds/' + encodeURIComponent(feedId) + '/playback-settings', {
      method: 'PUT', body: JSON.stringify(settings), signal,
    });
  },
  getGlobalSettings(signal?: AbortSignal): Promise<{ playback: PodcastGlobalPlaybackSettings }> {
    return request('/settings', { signal });
  },
  setGlobalSettings(playback: PodcastGlobalPlaybackSettings, signal?: AbortSignal): Promise<{ playback: PodcastGlobalPlaybackSettings }> {
    return request('/settings', {
      method: 'PUT', body: JSON.stringify(playback), signal,
    });
  },
  refreshStatus(feedId: string, signal?: AbortSignal): Promise<PodcastRefreshStatus> {
    return request('/feeds/' + encodeURIComponent(feedId) + '/refresh', { signal });
  },
  unsubscribe(feedId: string, signal?: AbortSignal): Promise<void> {
    return request('/feeds/' + encodeURIComponent(feedId), { method: 'DELETE', signal });
  },
  getDiscoverStatus(signal?: AbortSignal): Promise<{ status: PodcastDirectoryStatus }> {
    return request('/discover/status', { signal });
  },
  setDiscoverCredentials(apiKey: string, apiSecret: string, signal?: AbortSignal): Promise<{ status: PodcastDirectoryStatus }> {
    return request('/discover/credentials', {
      method: 'PUT', body: JSON.stringify({ apiKey, apiSecret }), signal,
    });
  },
  deleteDiscoverCredentials(signal?: AbortSignal): Promise<{ status: PodcastDirectoryStatus }> {
    return request('/discover/credentials', { method: 'DELETE', signal });
  },
  searchDirectory(q: string, max = 20, signal?: AbortSignal): Promise<{ shows: PodcastDirectoryShow[]; provider: string; query: string; count: number }> {
    const query = new URLSearchParams({ q, max: String(max) });
    return request('/search?' + query.toString(), { signal });
  },
  trendingDirectory(options: { category?: string; lang?: string; max?: number } = {}, signal?: AbortSignal): Promise<{ shows: PodcastDirectoryShow[]; provider: string; count: number }> {
    const query = new URLSearchParams();
    if (options.category) query.set('category', options.category);
    if (options.lang) query.set('lang', options.lang);
    if (options.max) query.set('max', String(options.max));
    const qs = query.toString();
    return request('/trending' + (qs ? '?' + qs : ''), { signal });
  },
  listEpisodesByView(
    view: PodcastEpisodeView,
    options: { q?: string; offset?: number; limit?: number } = {},
    signal?: AbortSignal,
  ): Promise<{ episodes: PodcastViewEpisode[]; nextOffset: number | null }> {
    const query = new URLSearchParams({
      view,
      offset: String(options.offset ?? 0),
      limit: String(options.limit ?? 50),
    });
    if (options.q?.trim()) query.set('q', options.q.trim());
    return request('/episodes?' + query.toString(), { signal });
  },
  searchEpisodes(q: string, offset = 0, limit = 50, signal?: AbortSignal): Promise<{ episodes: PodcastViewEpisode[]; nextOffset: number | null }> {
    const query = new URLSearchParams({ q, offset: String(offset), limit: String(limit) });
    return request('/episodes/search?' + query.toString(), { signal });
  },
  getEpisodeDetail(episodeId: string, signal?: AbortSignal): Promise<{ episode: PodcastEpisodeDetail }> {
    return request('/episodes/' + encodeURIComponent(episodeId), { signal });
  },
  patchEpisodeState(
    episodeId: string,
    patch: {
      expectedRevision?: number;
      starred?: boolean;
      archived?: boolean;
      isPlayed?: boolean;
      positionMs?: number;
      listenedMs?: number;
    },
    signal?: AbortSignal,
  ): Promise<{ state: PodcastProgress & { starred: boolean; archived: boolean } }> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/state', {
      method: 'PATCH', body: JSON.stringify(patch), signal,
    });
  },
  getEpisodeChapters(episodeId: string, signal?: AbortSignal): Promise<{ chapters: PodcastChapter[]; source: string }> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/chapters', { signal });
  },
  getEpisodeTranscript(episodeId: string, signal?: AbortSignal): Promise<{ cues: PodcastTranscriptCue[]; format: string }> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/transcript', { signal });
  },
  listBookmarks(episodeId: string, signal?: AbortSignal): Promise<{ bookmarks: PodcastBookmark[] }> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/bookmarks', { signal });
  },
  createBookmark(episodeId: string, positionMs: number, note: string, signal?: AbortSignal): Promise<{ bookmark: PodcastBookmark }> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/bookmarks', {
      method: 'POST', body: JSON.stringify({ positionMs, note }), signal,
    });
  },
  deleteBookmark(episodeId: string, bookmarkId: string, signal?: AbortSignal): Promise<void> {
    return request('/episodes/' + encodeURIComponent(episodeId) + '/bookmarks/' + encodeURIComponent(bookmarkId), {
      method: 'DELETE', signal,
    });
  },
  getQueue(signal?: AbortSignal): Promise<{ queue: PodcastQueueItem[] }> {
    return request('/queue', { signal });
  },
  addToQueue(episodeId: string, position: 'next' | 'end' = 'end', signal?: AbortSignal): Promise<{ queue: PodcastQueueItem[] }> {
    return request('/queue', {
      method: 'POST', body: JSON.stringify({ episodeId, position }), signal,
    });
  },
  replaceQueue(episodeIds: string[], signal?: AbortSignal): Promise<{ queue: PodcastQueueItem[] }> {
    return request('/queue', {
      method: 'PUT', body: JSON.stringify({ episodeIds }), signal,
    });
  },
  removeFromQueue(episodeId: string, signal?: AbortSignal): Promise<void> {
    return request('/queue/' + encodeURIComponent(episodeId), { method: 'DELETE', signal });
  },
  getRetentionPolicy(feedId: string, signal?: AbortSignal): Promise<{ policy: PodcastRetentionPolicy }> {
    return request('/feeds/' + encodeURIComponent(feedId) + '/retention', { signal });
  },
  setRetentionPolicy(feedId: string, policy: PodcastRetentionPolicy, signal?: AbortSignal): Promise<{ policy: PodcastRetentionPolicy }> {
    return request('/feeds/' + encodeURIComponent(feedId) + '/retention', {
      method: 'PUT', body: JSON.stringify(policy), signal,
    });
  },
};
