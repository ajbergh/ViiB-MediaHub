
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import {
  Archive,
  ArchiveRestore,
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  Check,
  Compass,
  Download,
  FileText,
  Headphones,
  ListChecks,
  Loader2,
  Play,
  Plus,
  RefreshCw,
  Rss,
  Search,
  Star,
  Trash2,
} from 'lucide-react';
import { api } from '../services/api';
import { getEventStreamURL } from '../services/eventStreamURL';
import { podcastEpisodeToTrack, queuePodcastEpisode, type PodcastQueuePosition } from '../lib/podcastPlayback';
import { useStore } from '../store';
import {
  podcastsAPI,
  PodcastAPIError,
  type PodcastFeed,
  type PodcastEpisode,
  type PodcastEpisodeView,
  type PodcastViewEpisode,
  type PodcastQueueItem,
  type PodcastRetentionPolicy,
  type PodcastEventsSnapshot,
  type PodcastPreview,
  type PodcastRefreshStatus,
  type PodcastOPMLImportPreview,
  type PodcastContinueEpisode,
  type PodcastDownload,
  type PodcastStorage,
  type PodcastAutoDownloadPolicy,
  type PodcastShowPlaybackSettings,
} from '../services/podcasts';
import { Page, PageHeader } from '../components/ui/Page';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { TextInput } from '../components/ui/TextInput';
import { Chip } from '../components/ui/Chip';
import { ToggleSwitch } from '../components/ui/ToggleSwitch';
import { Skeleton } from '../components/Skeleton';

const selectClass =
  'rounded-md border border-surface-border bg-surface-3 px-3 py-2 text-sm text-text-main outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:opacity-50';

const numberInputClass =
  'mt-1 w-24 rounded-md border border-surface-border bg-surface-3 px-2.5 py-1.5 text-sm text-text-main outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:opacity-50';

type PodcastHubTab = 'home' | 'episodes' | 'queue' | 'downloads';

const EPISODE_VIEW_OPTIONS: Array<{ id: PodcastEpisodeView; label: string }> = [
  { id: 'inbox', label: 'Inbox' },
  { id: 'starred', label: 'Starred' },
  { id: 'in_progress', label: 'In Progress' },
  { id: 'played', label: 'Played' },
  { id: 'archived', label: 'Archived' },
  { id: 'all', label: 'All' },
];

const RETENTION_OPTIONS = [0, 1, 3, 5, 10];

function readableError(error: unknown): string {
  return error instanceof Error ? error.message : 'The podcast service is unavailable.';
}

/** Subscribes to `/api/v2/podcasts/events` and falls back cleanly when disconnected. */
function usePodcastEventsSSE(
  enabled: boolean,
  onSnapshot: (snapshot: PodcastEventsSnapshot) => void,
): boolean {
  const [sseHealthy, setSseHealthy] = useState(false);
  const onSnapshotRef = useRef(onSnapshot);
  onSnapshotRef.current = onSnapshot;

  useEffect(() => {
    if (!enabled || typeof EventSource === 'undefined') {
      setSseHealthy(false);
      return;
    }

    let disposed = false;
    let source: EventSource | null = null;
    let reconnectTimer: number | null = null;

    const handlePodcastEvent = (event: Event) => {
      if (disposed) return;
      const message = event as MessageEvent<string>;
      if (typeof message.data !== 'string') return;
      try {
        const snapshot = JSON.parse(message.data) as PodcastEventsSnapshot;
        if (snapshot && Array.isArray(snapshot.downloads)) {
          onSnapshotRef.current(snapshot);
        }
      } catch {
        // Ignore malformed SSE payloads.
      }
    };

    const scheduleReconnect = () => {
      if (disposed || reconnectTimer !== null) return;
      reconnectTimer = window.setTimeout(() => {
        reconnectTimer = null;
        if (!disposed) void connect();
      }, 5000);
    };

    const connect = async () => {
      try {
        const streamURL = await getEventStreamURL('/api/v2/podcasts/events');
        if (disposed) return;
        const nextSource = new EventSource(streamURL);
        if (disposed) {
          nextSource.close();
          return;
        }
        source = nextSource;
        nextSource.onopen = () => {
          if (!disposed) setSseHealthy(true);
        };
        nextSource.addEventListener('podcasts', handlePodcastEvent);
        nextSource.onerror = () => {
          if (disposed) return;
          setSseHealthy(false);
          nextSource.removeEventListener('podcasts', handlePodcastEvent);
          nextSource.close();
          if (source === nextSource) source = null;
          scheduleReconnect();
        };
      } catch {
        if (!disposed) {
          setSseHealthy(false);
          scheduleReconnect();
        }
      }
    };

    void connect();

    return () => {
      disposed = true;
      setSseHealthy(false);
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
      if (source) {
        source.removeEventListener('podcasts', handlePodcastEvent);
        source.close();
        source = null;
      }
    };
  }, [enabled]);

  return sseHealthy;
}

/** Publisher images only load through the public-network-validated local API. */
const PodcastArtwork: React.FC<{ feedId: string; title: string; imageURL?: string; large?: boolean }> = ({
  feedId,
  title,
  imageURL,
  large = false,
}) => {
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    setFailed(false);
  }, [feedId, imageURL]);
  return (
    <span
      className={`relative flex shrink-0 items-center justify-center overflow-hidden rounded-xl bg-surface-highlight text-brand ${
        large ? 'size-20' : 'size-14'
      }`}
    >
      <Headphones size={large ? 40 : 25} aria-hidden="true" />
      {imageURL && !failed && (
        <img
          src={'/api/v2/podcasts/feeds/' + encodeURIComponent(feedId) + '/artwork'}
          alt={title + ' cover art'}
          loading="lazy"
          referrerPolicy="no-referrer"
          onError={() => setFailed(true)}
          className="absolute inset-0 h-full w-full object-cover"
        />
      )}
    </span>
  );
};

/** Public RSS subscriptions, shared playback and durable podcast queue sync. */
function queueEpisodeAction(episode: PodcastEpisode, feed: PodcastFeed, position: PodcastQueuePosition) {
  const player = useStore.getState();
  if (!queuePodcastEpisode(episode, feed, position, player)) return;
  void podcastsAPI.addToQueue(episode.id, position).catch(() => {});
  player.showToast({
    type: 'success',
    message: player.currentSong
      ? position === 'next'
        ? 'Episode added to Play Next.'
        : 'Episode added to the queue.'
      : 'Started podcast playback in the shared player.',
    duration: 3500,
  });
}

function buildFallbackFeed(
  episode: Pick<PodcastViewEpisode, 'feedId' | 'feedTitle' | 'feedAuthor'>,
  feeds: PodcastFeed[],
): PodcastFeed {
  return (
    feeds.find(item => item.id === episode.feedId) ?? {
      id: episode.feedId,
      url: '',
      title: episode.feedTitle,
      author: episode.feedAuthor || '',
      description: '',
      imageUrl: '',
      language: '',
      episodeCount: 0,
    }
  );
}

interface DownloadsSectionProps {
  downloads: PodcastDownload[];
  downloadStorage: PodcastStorage | null;
  downloadsLoading: boolean;
  downloadsError: string;
  downloadsFilter: 'all' | 'active' | 'completed' | 'attention';
  setDownloadsFilter: (filter: 'all' | 'active' | 'completed' | 'attention') => void;
  workingDownloadId: string | null;
  confirmRemoveDownloadId: string | null;
  setConfirmRemoveDownloadId: (id: string | null) => void;
  selectedPodcastId?: string;
  sseHealthy: boolean;
  onRefresh: () => void;
  onAction: (row: PodcastDownload, action: 'cancel' | 'retry' | 'remove') => Promise<void>;
}

const DownloadsSection: React.FC<DownloadsSectionProps> = ({
  downloads,
  downloadStorage,
  downloadsLoading,
  downloadsError,
  downloadsFilter,
  setDownloadsFilter,
  workingDownloadId,
  confirmRemoveDownloadId,
  setConfirmRemoveDownloadId,
  selectedPodcastId,
  sseHealthy,
  onRefresh,
  onAction,
}) => {
  const filteredDownloads = downloads.filter(
    row =>
      downloadsFilter === 'all' ||
      (downloadsFilter === 'active' &&
        (row.status === 'queued' || row.status === 'downloading' || row.status === 'paused')) ||
      (downloadsFilter === 'completed' && row.status === 'completed') ||
      (downloadsFilter === 'attention' && ['failed', 'canceled', 'paused'].includes(row.status)),
  );

  return (
    <section aria-label="Podcast downloads">
      <Card className="border border-surface-border p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-xl font-semibold text-text-main">Downloads</h2>
              {sseHealthy && (
                <span className="rounded-full bg-brand/15 px-2.5 py-0.5 text-[11px] font-semibold text-brand">
                  Live
                </span>
              )}
            </div>
            <p className="mt-1 text-xs text-text-secondary">
              Recent download activity from followed podcasts (up to 100). Offline copies play through your shared player.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <label htmlFor="podcast-download-filter" className="text-xs text-text-subtle">
              Show
            </label>
            <select
              id="podcast-download-filter"
              aria-label="Filter podcast downloads"
              value={downloadsFilter}
              onChange={event => {
                setDownloadsFilter(event.target.value as 'all' | 'active' | 'completed' | 'attention');
                setConfirmRemoveDownloadId(null);
              }}
              className={selectClass}
            >
              <option value="all">All</option>
              <option value="active">In progress</option>
              <option value="completed">Available offline</option>
              <option value="attention">Needs attention</option>
            </select>
            <Button
              variant="ghost"
              size="icon"
              onClick={onRefresh}
              aria-label="Refresh podcast downloads"
            >
              <RefreshCw size={17} />
            </Button>
          </div>
        </div>
        {downloadStorage && (
          <p className="mt-2 text-xs text-text-subtle">
            {(downloadStorage.usedBytes / (1024 * 1024)).toFixed(0)} MiB of{' '}
            {(downloadStorage.maxBytes / (1024 * 1024 * 1024)).toFixed(1)} GiB reserved
          </p>
        )}
        {downloadsError && (
          <p role="alert" className="mt-3 rounded-lg border border-error/30 bg-error/10 p-3 text-sm text-error">
            {downloadsError}
          </p>
        )}
        {downloadsLoading && (
          <div role="status" aria-label="Loading podcast downloads…" className="mt-4 space-y-2">
            <span className="sr-only">Loading podcast downloads…</span>
            <Skeleton className="h-20 w-full rounded-lg" />
            <Skeleton className="h-20 w-full rounded-lg" />
          </div>
        )}
        {!downloadsLoading && filteredDownloads.length === 0 && (
          <p className="mt-4 text-sm text-text-secondary">
            {downloads.length
              ? 'No downloads match this filter.'
              : 'No podcast downloads yet. Open a followed show to save an episode for offline listening.'}
          </p>
        )}
        {filteredDownloads.length > 0 && (
          <div className="mt-4 space-y-2">
            {filteredDownloads.map(row => {
              const isActive = row.status === 'queued' || row.status === 'downloading' || row.status === 'paused';
              const completed = row.status === 'completed';
              const playableElsewhere = selectedPodcastId === row.episodeId;
              const progress = row.expectedBytes
                ? Math.min(100, Math.floor((row.bytesReceived * 100) / row.expectedBytes))
                : null;
              const name = row.episodeTitle || 'Podcast episode';
              return (
                <article
                  key={row.episodeId}
                  className="rounded-lg border border-surface-border bg-surface-1 p-4"
                >
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <p className="text-xs font-semibold text-brand">{row.feedTitle || 'Followed show'}</p>
                      <h3 className="mt-1 break-words text-sm font-semibold text-text-main">
                        <Link
                          to={'/podcasts/episode/' + encodeURIComponent(row.episodeId)}
                          className="hover:text-brand hover:underline"
                        >
                          {name}
                        </Link>
                      </h3>
                      <p className="mt-1 text-xs text-text-secondary">
                        {completed
                          ? 'Available offline'
                          : row.status === 'downloading'
                          ? `Downloading${progress === null ? '' : ' ' + progress + '%'}`
                          : row.status === 'paused' && row.retryAfter
                          ? `Retry scheduled for ${new Date(row.retryAfter).toLocaleTimeString([], {
                              hour: 'numeric',
                              minute: '2-digit',
                            })}`
                          : row.status === 'queued'
                          ? 'Queued'
                          : row.status === 'failed'
                          ? `Failed: ${row.errorCode || 'unavailable'}`
                          : row.status === 'canceled'
                          ? 'Canceled'
                          : 'Paused'}
                        {completed && row.verifiedSize > 0
                          ? ` · ${(row.verifiedSize / (1024 * 1024)).toFixed(1)} MiB`
                          : ''}
                      </p>
                      {row.status === 'downloading' && progress !== null && (
                        <div className="mt-2 h-1.5 w-full max-w-xs overflow-hidden rounded-full bg-surface-3">
                          <div className="h-full rounded-full bg-brand transition-all" style={{ width: `${progress}%` }} />
                        </div>
                      )}
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      {row.feedId && (
                        <Link
                          to={'/podcasts/show/' + encodeURIComponent(row.feedId)}
                          className="inline-flex items-center justify-center rounded-full bg-surface-3 px-3 py-1.5 text-xs font-semibold text-text-main hover:bg-surface-highlight transition-colors"
                        >
                          View show
                        </Link>
                      )}
                      {isActive ? (
                        <Button
                          variant="secondary"
                          size="sm"
                          disabled={workingDownloadId !== null}
                          onClick={() => void onAction(row, 'cancel')}
                          aria-label={'Cancel download of ' + name}
                        >
                          Cancel
                        </Button>
                      ) : completed ? (
                        <Button
                          variant="secondary"
                          size="sm"
                          disabled={workingDownloadId !== null || playableElsewhere}
                          onClick={() => setConfirmRemoveDownloadId(row.episodeId)}
                          aria-label={'Remove offline download of ' + name}
                        >
                          Remove download
                        </Button>
                      ) : (
                        <Button
                          variant="secondary"
                          size="sm"
                          disabled={workingDownloadId !== null}
                          onClick={() => void onAction(row, 'retry')}
                          aria-label={'Retry download of ' + name}
                        >
                          Retry
                        </Button>
                      )}
                    </div>
                  </div>
                  {completed && playableElsewhere && (
                    <p className="mt-2 text-xs text-text-subtle">
                      Switch away from this episode to remove its offline audio.
                    </p>
                  )}
                  {completed && confirmRemoveDownloadId === row.episodeId && (
                    <div
                      role="group"
                      aria-label={'Confirm removing download of ' + name}
                      className="mt-3 flex flex-wrap items-center gap-2 text-xs text-text-secondary"
                    >
                      <span>Remove offline audio? Your listening history will remain.</span>
                      <Button
                        variant="primary"
                        size="sm"
                        disabled={workingDownloadId !== null}
                        onClick={() => void onAction(row, 'remove')}
                        aria-label={'Confirm remove download of ' + name}
                      >
                        Remove copy
                      </Button>
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => setConfirmRemoveDownloadId(null)}
                      >
                        Keep
                      </Button>
                    </div>
                  )}
                </article>
              );
            })}
          </div>
        )}
      </Card>
    </section>
  );
};

export const Podcasts: React.FC = () => {
  const [activeTab, setActiveTab] = useState<PodcastHubTab>('home');
  const [feeds, setFeeds] = useState<PodcastFeed[]>([]);
  const [continueEpisodes, setContinueEpisodes] = useState<PodcastContinueEpisode[]>([]);
  const [followingQuery, setFollowingQuery] = useState('');
  const [downloads, setDownloads] = useState<PodcastDownload[]>([]);
  const [downloadStorage, setDownloadStorage] = useState<PodcastStorage | null>(null);
  const [downloadsLoading, setDownloadsLoading] = useState(true);
  const [downloadsError, setDownloadsError] = useState('');
  const [downloadsFilter, setDownloadsFilter] = useState<'all' | 'active' | 'completed' | 'attention'>('all');
  const [workingDownloadId, setWorkingDownloadId] = useState<string | null>(null);
  const [confirmRemoveDownloadId, setConfirmRemoveDownloadId] = useState<string | null>(null);
  const [refreshCount, setRefreshCount] = useState(0);
  const prevRefreshCountRef = useRef<number | null>(null);
  const selectedPodcastId = useStore(state => state.currentSong?.podcastEpisodeId);
  const showToast = useStore(state => state.showToast);

  // OPML & RSS add state
  const [exporting, setExporting] = useState(false);
  const [opml, setOPML] = useState<PodcastOPMLImportPreview | null>(null);
  const [importMode, setImportMode] = useState<'idle' | 'preview' | 'import'>('idle');
  const [importDone, setImportDone] = useState(0);
  const [importSummary, setImportSummary] = useState('');
  const [importFailures, setImportFailures] = useState<string[]>([]);
  const importController = useRef<AbortController | null>(null);
  const [url, setURL] = useState('');
  const [preview, setPreview] = useState<PodcastPreview | null>(null);
  const [loading, setLoading] = useState(true);
  const [working, setWorking] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [revision, setRevision] = useState(0);

  // Episodes triage & cross-show search state (POD-04D + POD-05F)
  const [episodesView, setEpisodesView] = useState<PodcastEpisodeView>('inbox');
  const [episodesQuery, setEpisodesQuery] = useState('');
  const [debouncedEpisodesQuery, setDebouncedEpisodesQuery] = useState('');
  const [viewEpisodes, setViewEpisodes] = useState<PodcastViewEpisode[]>([]);
  const [viewEpisodesNextOffset, setViewEpisodesNextOffset] = useState<number | null>(null);
  const [viewEpisodesLoading, setViewEpisodesLoading] = useState(false);
  const [viewEpisodesLoadingMore, setViewEpisodesLoadingMore] = useState(false);
  const [viewEpisodesError, setViewEpisodesError] = useState('');
  const [triagingEpisodeId, setTriagingEpisodeId] = useState<string | null>(null);

  // Durable Queue tab state (POD-04D)
  const [queueItems, setQueueItems] = useState<PodcastQueueItem[]>([]);
  const [queueLoading, setQueueLoading] = useState(false);
  const [queueSaving, setQueueSaving] = useState(false);
  const [queueError, setQueueError] = useState('');

  const navigate = useNavigate();
  useEffect(() => () => { importController.current?.abort(); }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDebouncedEpisodesQuery(episodesQuery);
    }, 220);
    return () => window.clearTimeout(timer);
  }, [episodesQuery]);

  const handleSseSnapshot = useCallback((snapshot: PodcastEventsSnapshot) => {
    setDownloads(snapshot.downloads);
    setDownloadStorage(snapshot.storage ?? null);
    setDownloadsLoading(false);
    setDownloadsError('');
    const nextRefreshCount = snapshot.refreshCount ?? 0;
    setRefreshCount(nextRefreshCount);
    if (prevRefreshCountRef.current !== null && prevRefreshCountRef.current > 0 && nextRefreshCount === 0) {
      setRevision(v => v + 1);
    }
    prevRefreshCountRef.current = nextRefreshCount;
  }, []);

  const sseHealthy = usePodcastEventsSSE(true, handleSseSnapshot);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    void podcastsAPI
      .listContinueListening(12, controller.signal)
      .then(({ episodes }) => {
        if (!controller.signal.aborted) setContinueEpisodes(episodes);
      })
      .catch(() => {
        if (!controller.signal.aborted) setContinueEpisodes([]);
      });
    void podcastsAPI
      .listSubscriptions(controller.signal)
      .then(({ feeds }) => {
        if (!controller.signal.aborted) {
          setFeeds(feeds);
          setError('');
        }
      })
      .catch(err => {
        if (!controller.signal.aborted) setError(readableError(err));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [revision]);

  // Initial fetch + fallback polling only when SSE is disconnected.
  useEffect(() => {
    const controller = new AbortController();
    const refresh = () => {
      void podcastsAPI
        .listDownloads(controller.signal)
        .then(({ downloads: rows, storage }) => {
          if (controller.signal.aborted) return;
          setDownloads(rows);
          setDownloadStorage(storage ?? null);
          setDownloadsError('');
        })
        .catch(err => {
          if (!controller.signal.aborted) setDownloadsError(readableError(err));
        })
        .finally(() => {
          if (!controller.signal.aborted) setDownloadsLoading(false);
        });
    };
    refresh();
    if (sseHealthy) {
      return () => {
        controller.abort();
      };
    }
    const timer = window.setInterval(refresh, 5000);
    return () => {
      controller.abort();
      window.clearInterval(timer);
    };
  }, [revision, sseHealthy]);

  // Load Episodes triage / cross-show search when Episodes tab is active.
  useEffect(() => {
    if (activeTab !== 'episodes') return;
    const controller = new AbortController();
    const trimmed = debouncedEpisodesQuery.trim();
    setViewEpisodesLoading(true);
    setViewEpisodesError('');
    const requestPromise =
      episodesView === 'all' && trimmed.length > 0
        ? podcastsAPI.searchEpisodes(trimmed, 0, 50, controller.signal)
        : podcastsAPI.listEpisodesByView(
            episodesView,
            { q: trimmed || undefined, offset: 0, limit: 50 },
            controller.signal,
          );
    void requestPromise
      .then(({ episodes, nextOffset }) => {
        if (controller.signal.aborted) return;
        setViewEpisodes(episodes);
        setViewEpisodesNextOffset(nextOffset ?? null);
      })
      .catch(err => {
        if (!controller.signal.aborted) {
          setViewEpisodesError(readableError(err));
          setViewEpisodes([]);
          setViewEpisodesNextOffset(null);
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setViewEpisodesLoading(false);
      });
    return () => controller.abort();
  }, [activeTab, episodesView, debouncedEpisodesQuery, revision]);

  const loadMoreViewEpisodes = async () => {
    if (viewEpisodesNextOffset === null || viewEpisodesLoadingMore) return;
    setViewEpisodesLoadingMore(true);
    setViewEpisodesError('');
    const trimmed = debouncedEpisodesQuery.trim();
    try {
      const page =
        episodesView === 'all' && trimmed.length > 0
          ? await podcastsAPI.searchEpisodes(trimmed, viewEpisodesNextOffset, 50)
          : await podcastsAPI.listEpisodesByView(episodesView, {
              q: trimmed || undefined,
              offset: viewEpisodesNextOffset,
              limit: 50,
            });
      setViewEpisodes(prev => {
        const seen = new Set(prev.map(item => item.id));
        return [...prev, ...page.episodes.filter(item => !seen.has(item.id))];
      });
      setViewEpisodesNextOffset(page.nextOffset ?? null);
    } catch (err) {
      setViewEpisodesError(readableError(err));
    } finally {
      setViewEpisodesLoadingMore(false);
    }
  };

  // Load durable Queue when Queue tab is active.
  const fetchQueue = useCallback(async (signal?: AbortSignal) => {
    setQueueLoading(true);
    setQueueError('');
    try {
      const { queue } = await podcastsAPI.getQueue(signal);
      if (!signal?.aborted) setQueueItems(queue);
    } catch (err) {
      if (!signal?.aborted) setQueueError(readableError(err));
    } finally {
      if (!signal?.aborted) setQueueLoading(false);
    }
  }, []);

  useEffect(() => {
    if (activeTab !== 'queue') return;
    const controller = new AbortController();
    void fetchQueue(controller.signal);
    return () => controller.abort();
  }, [activeTab, fetchQueue, revision]);

  const downloadsByEpisodeId = React.useMemo(
    () => new Map(downloads.map(item => [item.episodeId, item])),
    [downloads],
  );

  const changeLibraryDownload = async (row: PodcastDownload, action: 'cancel' | 'retry' | 'remove') => {
    if (workingDownloadId) return;
    if (action === 'remove' && selectedPodcastId === row.episodeId) {
      setDownloadsError('Switch away from this playing episode before deleting its offline copy.');
      return;
    }
    setWorkingDownloadId(row.episodeId);
    setDownloadsError('');
    try {
      if (action === 'remove') {
        await podcastsAPI.removeDownload(row.episodeId);
        setDownloads(current => current.filter(item => item.episodeId !== row.episodeId));
        setConfirmRemoveDownloadId(null);
      } else {
        const { download } =
          action === 'cancel'
            ? await podcastsAPI.cancelDownload(row.episodeId)
            : await podcastsAPI.queueDownload(row.episodeId);
        setDownloads(current =>
          current.map(item => (item.episodeId === row.episodeId ? { ...item, ...download } : item)),
        );
      }
      const { downloads: refreshed, storage } = await podcastsAPI.listDownloads();
      setDownloads(refreshed);
      if (storage) setDownloadStorage(storage);
    } catch (err) {
      setDownloadsError(readableError(err));
    } finally {
      setWorkingDownloadId(null);
    }
  };

  const toggleEpisodeDownload = async (episode: PodcastViewEpisode) => {
    if (workingDownloadId) return;
    const live = downloadsByEpisodeId.get(episode.id);
    const status = live?.status ?? episode.downloadStatus;
    const active = status === 'queued' || status === 'downloading' || status === 'paused';
    const completed = status === 'completed';
    if (completed && selectedPodcastId === episode.id) {
      showToast({ type: 'error', message: 'Switch away from this episode before removing its offline copy.' });
      return;
    }
    setWorkingDownloadId(episode.id);
    try {
      if (completed) {
        await podcastsAPI.removeDownload(episode.id);
        setDownloads(current => current.filter(item => item.episodeId !== episode.id));
        setViewEpisodes(current =>
          current.map(item => (item.id === episode.id ? { ...item, downloadStatus: undefined } : item)),
        );
      } else if (active) {
        const { download } = await podcastsAPI.cancelDownload(episode.id);
        setDownloads(current => {
          const exists = current.some(item => item.episodeId === episode.id);
          return exists
            ? current.map(item => (item.episodeId === episode.id ? { ...item, ...download } : item))
            : [download, ...current];
        });
      } else {
        const { download } = await podcastsAPI.queueDownload(episode.id);
        setDownloads(current => {
          const exists = current.some(item => item.episodeId === episode.id);
          return exists
            ? current.map(item => (item.episodeId === episode.id ? { ...item, ...download } : item))
            : [download, ...current];
        });
      }
    } catch (err) {
      showToast({ type: 'error', message: readableError(err) });
    } finally {
      setWorkingDownloadId(null);
    }
  };

  const patchViewEpisodeTriage = async (
    episode: PodcastViewEpisode,
    patch: { starred?: boolean; archived?: boolean; isPlayed?: boolean },
  ) => {
    if (triagingEpisodeId) return;
    const previousList = viewEpisodes;
    setTriagingEpisodeId(episode.id);
    setViewEpisodesError('');

    // Optimistically apply triage changes and filter out items that leave the active view.
    setViewEpisodes(current => {
      const updated = current.map(item =>
        item.id === episode.id ? { ...item, ...patch } : item,
      );
      return updated.filter(item => {
        if (item.id !== episode.id) return true;
        if (episodesView === 'starred' && patch.starred === false) return false;
        if (episodesView === 'archived' && patch.archived === false) return false;
        if (episodesView === 'inbox' && (patch.archived === true || patch.isPlayed === true)) return false;
        if (episodesView === 'played' && patch.isPlayed === false) return false;
        if (episodesView === 'in_progress' && patch.isPlayed === true) return false;
        return true;
      });
    });

    try {
      const { state } = await podcastsAPI.patchEpisodeState(episode.id, {
        ...patch,
        ...(patch.isPlayed !== undefined && typeof episode.progressRevision === 'number'
          ? { expectedRevision: episode.progressRevision }
          : {}),
      });
      setViewEpisodes(current =>
        current.map(item =>
          item.id === episode.id
            ? {
                ...item,
                starred: state.starred,
                archived: state.archived,
                isPlayed: state.isPlayed,
                positionMs: state.positionMs,
                listenedMs: state.listenedMs,
                progressRevision: state.revision,
              }
            : item,
        ),
      );
    } catch (err) {
      setViewEpisodes(previousList);
      const msg = readableError(err);
      setViewEpisodesError(msg);
      showToast({ type: 'error', message: msg });
    } finally {
      setTriagingEpisodeId(null);
    }
  };

  const moveQueueItem = async (index: number, direction: -1 | 1) => {
    const target = index + direction;
    if (queueSaving || target < 0 || target >= queueItems.length) return;
    const previous = queueItems;
    const next = [...queueItems];
    const [moved] = next.splice(index, 1);
    next.splice(target, 0, moved);
    setQueueItems(next);
    setQueueSaving(true);
    setQueueError('');
    try {
      const { queue } = await podcastsAPI.replaceQueue(next.map(item => item.id));
      setQueueItems(queue);
    } catch (err) {
      setQueueItems(previous);
      const msg = readableError(err);
      setQueueError(msg);
      showToast({ type: 'error', message: msg });
    } finally {
      setQueueSaving(false);
    }
  };

  const removeQueueItem = async (episodeId: string) => {
    if (queueSaving) return;
    const previous = queueItems;
    setQueueItems(current => current.filter(item => item.id !== episodeId));
    setQueueSaving(true);
    setQueueError('');
    try {
      await podcastsAPI.removeFromQueue(episodeId);
    } catch (err) {
      setQueueItems(previous);
      const msg = readableError(err);
      setQueueError(msg);
      showToast({ type: 'error', message: msg });
    } finally {
      setQueueSaving(false);
    }
  };

  const playDurableQueue = (startEpisodeId?: string) => {
    const playable = queueItems.filter(item => item.hasAudio);
    if (playable.length === 0) return;
    const tracks = playable.map(item => podcastEpisodeToTrack(item, buildFallbackFeed(item, feeds)));
    const startIndex = startEpisodeId
      ? Math.max(0, playable.findIndex(item => item.id === startEpisodeId))
      : 0;
    void useStore.getState().playSong(tracks[startIndex], tracks, 'podcast', startIndex);
  };

  const showPreview = useCallback(
    async (event: React.FormEvent) => {
      event.preventDefault();
      setWorking(true);
      setError('');
      setNotice('');
      setPreview(null);
      try {
        setPreview(await podcastsAPI.previewFeed(url.trim()));
      } catch (err) {
        setError(readableError(err));
      } finally {
        setWorking(false);
      }
    },
    [url],
  );

  const subscribe = useCallback(async () => {
    if (!preview) return;
    setWorking(true);
    setError('');
    try {
      const result = await podcastsAPI.subscribe(url.trim());
      setPreview(null);
      setURL('');
      setNotice(`Subscribed to ${result.title}`);
      setRevision(current => current + 1);
      navigate('/podcasts/show/' + encodeURIComponent(result.id));
    } catch (err) {
      setError(readableError(err));
    } finally {
      setWorking(false);
    }
  }, [preview, navigate, url]);

  const previewOPMLFile = async (file?: File) => {
    if (!file || importMode !== 'idle') return;
    setOPML(null);
    setImportSummary('');
    setImportFailures([]);
    setError('');
    if (file.size > 512 * 1024) {
      setError('OPML file is larger than the supported 512 KB limit.');
      return;
    }
    setImportMode('preview');
    try {
      // Raw XML is uploaded only for validation and never persisted in UI state.
      setOPML(await podcastsAPI.previewOPML(await file.text()));
      setImportDone(0);
    } catch (err) {
      setError(readableError(err));
    } finally {
      setImportMode('idle');
    }
  };

  const importOPMLFeeds = async () => {
    if (!opml || importMode !== 'idle') return;
    const candidates = opml.feeds.filter(feed => !feed.alreadyFollowed);
    const controller = new AbortController();
    importController.current = controller;
    setImportMode('import');
    setImportDone(0);
    setImportFailures([]);
    setImportSummary('');
    setError('');
    let success = 0;
    let failures = 0;
    let processed = 0;
    const failedNames: string[] = [];
    try {
      for (const candidate of candidates) {
        if (controller.signal.aborted) break;
        try {
          await podcastsAPI.subscribe(candidate.url, controller.signal);
          success++;
          setOPML(current =>
            current
              ? {
                  ...current,
                  feeds: current.feeds.map(feed =>
                    feed.url === candidate.url ? { ...feed, alreadyFollowed: true } : feed,
                  ),
                }
              : null,
          );
        } catch (err) {
          if (controller.signal.aborted) break;
          failures++;
          failedNames.push(
            `${candidate.title}: ${err instanceof PodcastAPIError ? err.code : 'request_failed'}`,
          );
        }
        processed++;
        setImportDone(processed);
      }
    } finally {
      importController.current = null;
      setImportFailures(failedNames);
      setImportSummary(
        `Followed ${success} podcast${success === 1 ? '' : 's'}; ${failures} failed.${
          controller.signal.aborted ? ' Import canceled.' : ''
        }`,
      );
      if (success > 0) setRevision(current => current + 1);
      setImportMode('idle');
    }
  };

  const getContinueFeed = (episode: PodcastContinueEpisode): PodcastFeed =>
    feeds.find(item => item.id === episode.feedId) ?? {
      id: episode.feedId,
      url: '',
      title: episode.feedTitle,
      author: '',
      description: '',
      imageUrl: '',
      language: '',
      episodeCount: 0,
    };

  const resumeEpisode = (episode: PodcastContinueEpisode) => {
    if (!episode.hasAudio) return;
    const track = podcastEpisodeToTrack(episode, getContinueFeed(episode));
    void useStore.getState().playSong(track, [track], 'podcast', 0);
  };

  const exportFollowing = async () => {
    if (exporting) return;
    setExporting(true);
    setError('');
    try {
      const content = await podcastsAPI.exportOPML();
      const objectURL = URL.createObjectURL(content);
      const anchor = document.createElement('a');
      anchor.href = objectURL;
      anchor.download = 'viib-podcasts.opml';
      document.body.appendChild(anchor);
      try {
        anchor.click();
      } finally {
        anchor.remove();
        window.setTimeout(() => URL.revokeObjectURL(objectURL), 30000);
      }
    } catch (err) {
      setError(readableError(err));
    } finally {
      setExporting(false);
    }
  };

  const filteredFeeds = feeds.filter(feed =>
    (feed.title + ' ' + feed.author).toLowerCase().includes(followingQuery.trim().toLowerCase()),
  );

  return (
    <Page>
      <div className="mx-auto w-full max-w-6xl space-y-6">
        <PageHeader
          heading="Podcasts"
          subtitle="Follow public RSS podcasts, triage episodes, manage your durable queue, and listen offline through the shared ViiB player."
          actions={
            <div className="flex flex-wrap items-center gap-2.5">
              {refreshCount > 0 && (
                <span
                  role="status"
                  className="inline-flex items-center gap-1.5 rounded-full border border-surface-border bg-surface-2 px-3 py-1.5 text-xs font-medium text-brand"
                >
                  <Loader2 size={13} className="animate-spin" aria-hidden="true" />
                  Refreshing {refreshCount} {refreshCount === 1 ? 'feed' : 'feeds'}…
                </span>
              )}
              <Link
                to="/podcasts/discover"
                className="inline-flex items-center justify-center gap-2 rounded-full bg-brand px-4 py-2 text-[13px] font-bold text-surface-0 transition-all duration-150 ease-out hover:brightness-105"
              >
                <Compass size={15} aria-hidden="true" />
                Discover podcasts
              </Link>
            </div>
          }
        />

        <nav aria-label="Podcast hub views" className="flex flex-wrap items-center gap-2">
          <Chip
            selected={activeTab === 'home'}
            aria-pressed={activeTab === 'home'}
            onClick={() => setActiveTab('home')}
          >
            Home
          </Chip>
          <Chip
            selected={activeTab === 'episodes'}
            aria-pressed={activeTab === 'episodes'}
            onClick={() => setActiveTab('episodes')}
          >
            Episodes
          </Chip>
          <Chip
            selected={activeTab === 'queue'}
            aria-pressed={activeTab === 'queue'}
            onClick={() => setActiveTab('queue')}
          >
            Queue
          </Chip>
          <Chip
            selected={activeTab === 'downloads'}
            aria-pressed={activeTab === 'downloads'}
            onClick={() => setActiveTab('downloads')}
          >
            Downloads
          </Chip>
          <Link
            to="/podcasts/discover"
            className="inline-flex items-center justify-center gap-1.5 rounded-full bg-surface-3 px-3.5 py-1.5 text-[13px] font-medium text-text-main transition-colors duration-150 ease-out hover:bg-surface-highlight"
          >
            <Compass size={14} aria-hidden="true" />
            Discover
          </Link>
        </nav>

        {activeTab === 'home' && (
          <div className="space-y-6">
            <div className="grid gap-6 lg:grid-cols-2">
              <section>
                <Card className="h-full border border-surface-border p-5">
                  <h2 className="mb-3 text-lg font-semibold text-text-main">Add a podcast by RSS URL</h2>
                  <form onSubmit={showPreview} className="flex flex-col gap-3 sm:flex-row">
                    <label className="sr-only" htmlFor="podcast-feed-url">
                      Podcast RSS URL
                    </label>
                    <TextInput
                      id="podcast-feed-url"
                      type="url"
                      placeholder="https://publisher.example/podcast.xml"
                      required
                      autoComplete="url"
                      className="flex-1"
                      value={url}
                      onChange={event => {
                        setURL(event.target.value);
                        setPreview(null);
                      }}
                    />
                    <Button
                      type="submit"
                      variant="primary"
                      disabled={working || !url.trim()}
                      leftIcon={working ? <Loader2 size={16} className="animate-spin" /> : <Rss size={16} />}
                    >
                      Preview feed
                    </Button>
                  </form>
                  <p className="mt-3 text-xs text-text-subtle">
                    Public HTTPS RSS feeds only during this preview. Private and legacy HTTP feeds will be supported after their security review.
                  </p>
                  {error && (
                    <p role="alert" className="mt-3 rounded-lg border border-error/30 bg-error/10 p-3 text-sm text-error">
                      {error}
                    </p>
                  )}
                  {notice && (
                    <p role="status" className="mt-3 text-sm text-brand">
                      {notice}
                    </p>
                  )}
                  {preview && (
                    <div className="mt-5 rounded-lg border border-surface-border bg-surface-1 p-4">
                      <h3 className="text-lg font-semibold text-text-main">{preview.feed.title}</h3>
                      <p className="text-sm text-text-secondary">
                        {preview.feed.author || 'Independent publisher'} · {preview.episodeCount} episodes in the feed
                      </p>
                      <p className="mt-2 text-sm text-text-secondary">{preview.feed.description}</p>
                      <Button
                        variant="primary"
                        className="mt-4"
                        disabled={working}
                        onClick={() => void subscribe()}
                        leftIcon={<Plus size={16} />}
                      >
                        Follow podcast
                      </Button>
                    </div>
                  )}
                </Card>
              </section>

              <section aria-label="Import podcasts from OPML">
                <Card className="h-full border border-surface-border p-5">
                  <h2 className="mb-2 text-lg font-semibold text-text-main">Import podcast subscriptions</h2>
                  <p className="mb-3 text-sm text-text-secondary">
                    Choose an OPML 2.0 file from another podcast app. Preview happens without contacting publishers; following each feed begins only when you confirm.
                  </p>
                  <label htmlFor="podcast-opml-file" className="mb-2 block text-sm font-semibold text-text-main">
                    OPML file
                  </label>
                  <input
                    id="podcast-opml-file"
                    type="file"
                    accept=".opml,.xml,application/xml,text/xml"
                    disabled={importMode !== 'idle'}
                    aria-label="Choose OPML file"
                    onChange={event => {
                      const file = event.target.files?.[0];
                      event.target.value = '';
                      void previewOPMLFile(file);
                    }}
                    className="block max-w-full text-sm text-text-secondary file:mr-3 file:rounded-full file:border file:border-surface-border file:bg-surface-3 file:px-3.5 file:py-1.5 file:text-xs file:font-semibold file:text-text-main hover:file:bg-surface-highlight"
                  />
                  {importMode === 'preview' && (
                    <p role="status" className="mt-3 text-sm text-text-secondary">
                      Checking OPML file…
                    </p>
                  )}
                  {opml && (
                    <div className="mt-4 space-y-3 rounded-lg border border-surface-border bg-surface-1 p-4">
                      <p className="text-sm font-semibold text-text-main">
                        {opml.feeds.length} supported feeds,{' '}
                        {opml.feeds.filter(feed => feed.alreadyFollowed).length} already followed
                      </p>
                      {(opml.invalidCount > 0 || opml.duplicateCount > 0) && (
                        <p className="text-xs text-text-secondary">
                          {opml.invalidCount} unsupported or private feeds skipped; {opml.duplicateCount} duplicates skipped.
                        </p>
                      )}
                      <ul className="space-y-1 text-xs text-text-secondary">
                        {opml.feeds.slice(0, 8).map(feed => (
                          <li key={feed.url}>
                            {feed.title}
                            {feed.alreadyFollowed ? ' · Following' : ''}
                          </li>
                        ))}
                      </ul>
                      {opml.feeds.length > 8 && (
                        <p className="text-xs text-text-subtle">And {opml.feeds.length - 8} more feeds…</p>
                      )}
                      <div className="flex flex-wrap items-center gap-3">
                        <Button
                          variant="primary"
                          size="sm"
                          disabled={importMode !== 'idle' || opml.feeds.every(feed => feed.alreadyFollowed)}
                          onClick={() => void importOPMLFeeds()}
                        >
                          Import {opml.feeds.filter(feed => !feed.alreadyFollowed).length} feed
                          {opml.feeds.filter(feed => !feed.alreadyFollowed).length === 1 ? '' : 's'}
                        </Button>
                        {importMode === 'import' && (
                          <Button
                            variant="secondary"
                            size="sm"
                            onClick={() => importController.current?.abort()}
                          >
                            Cancel import
                          </Button>
                        )}
                        {importMode === 'import' && (
                          <span role="status" className="text-xs text-text-secondary">
                            Processed {importDone} feeds…
                          </span>
                        )}
                      </div>
                    </div>
                  )}
                  {importSummary && (
                    <p role="status" className="mt-3 text-sm text-text-secondary">
                      {importSummary}
                    </p>
                  )}
                  {importFailures.length > 0 && (
                    <div className="mt-3 text-xs text-text-secondary">
                      <p className="font-semibold">Feeds that could not be followed</p>
                      <ul className="mt-1 list-inside list-disc">
                        {importFailures.map((message, index) => (
                          <li key={index}>{message}</li>
                        ))}
                      </ul>
                    </div>
                  )}
                </Card>
              </section>
            </div>

            {continueEpisodes.length > 0 && (
              <section aria-label="Continue listening">
                <h2 className="mb-3 text-xl font-bold text-text-main">Continue Listening</h2>
                <div className="grid gap-3 md:grid-cols-2">
                  {continueEpisodes.map(episode => (
                    <Card key={episode.id} className="border border-surface-border p-4">
                      <article>
                        <p className="text-xs font-semibold text-brand">{episode.feedTitle}</p>
                        <h3 className="mt-1 font-semibold text-text-main">
                          <Link
                            to={'/podcasts/episode/' + encodeURIComponent(episode.id)}
                            className="hover:text-brand hover:underline"
                          >
                            {episode.title}
                          </Link>
                        </h3>
                        <p className="mt-1 text-xs text-text-secondary">
                          Saved at {Math.floor(episode.positionMs / 60000)} min
                        </p>
                        <div className="mt-3 flex flex-wrap items-center gap-2">
                          {episode.hasAudio ? (
                            <Button
                              variant="primary"
                              size="sm"
                              onClick={() => resumeEpisode(episode)}
                              aria-label={'Resume ' + episode.title}
                              leftIcon={<Play size={13} />}
                            >
                              Resume playback
                            </Button>
                          ) : (
                            <span className="text-xs text-text-subtle">Audio unavailable</span>
                          )}
                          {episode.hasAudio && (
                            <>
                              <Button
                                variant="secondary"
                                size="sm"
                                onClick={() => queueEpisodeAction(episode, getContinueFeed(episode), 'next')}
                                aria-label={'Play ' + episode.title + ' next'}
                              >
                                Play Next
                              </Button>
                              <Button
                                variant="secondary"
                                size="sm"
                                onClick={() => queueEpisodeAction(episode, getContinueFeed(episode), 'end')}
                                aria-label={'Add ' + episode.title + ' to queue'}
                              >
                                Add to Queue
                              </Button>
                            </>
                          )}
                          <Link
                            to={'/podcasts/show/' + encodeURIComponent(episode.feedId)}
                            className="ml-auto text-xs font-semibold text-text-secondary hover:text-text-main"
                          >
                            View show
                          </Link>
                        </div>
                      </article>
                    </Card>
                  ))}
                </div>
              </section>
            )}

            <DownloadsSection
              downloads={downloads}
              downloadStorage={downloadStorage}
              downloadsLoading={downloadsLoading}
              downloadsError={downloadsError}
              downloadsFilter={downloadsFilter}
              setDownloadsFilter={setDownloadsFilter}
              workingDownloadId={workingDownloadId}
              confirmRemoveDownloadId={confirmRemoveDownloadId}
              setConfirmRemoveDownloadId={setConfirmRemoveDownloadId}
              selectedPodcastId={selectedPodcastId}
              sseHealthy={sseHealthy}
              onRefresh={() => setRevision(value => value + 1)}
              onAction={changeLibraryDownload}
            />

            <section aria-label="Followed podcasts">
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <h2 className="text-xl font-semibold text-text-main">Following</h2>
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <TextInput
                    type="search"
                    aria-label="Search followed podcasts"
                    placeholder="Search your podcasts"
                    leftIcon={<Search size={15} className="shrink-0 text-text-subtle" />}
                    value={followingQuery}
                    onChange={event => setFollowingQuery(event.target.value)}
                    onClear={() => setFollowingQuery('')}
                    className="w-full sm:w-64"
                  />
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => void exportFollowing()}
                    disabled={loading || feeds.length === 0 || exporting}
                    aria-label="Export followed podcasts as OPML"
                    leftIcon={<Download size={15} aria-hidden="true" />}
                  >
                    {exporting ? 'Exporting…' : 'Export OPML'}
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Refresh subscribed podcasts"
                    onClick={() => setRevision(value => value + 1)}
                  >
                    <RefreshCw size={18} />
                  </Button>
                </div>
              </div>
              {loading && (
                <div role="status" aria-label="Loading subscriptions…" className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                  <span className="sr-only">Loading subscriptions…</span>
                  <Skeleton className="h-24 w-full rounded-lg" />
                  <Skeleton className="h-24 w-full rounded-lg" />
                  <Skeleton className="h-24 w-full rounded-lg" />
                </div>
              )}
              {!loading && feeds.length === 0 && (
                <Card className="border border-surface-border p-8 text-center text-text-secondary">
                  No podcasts followed yet. Add an RSS feed above to get started.
                </Card>
              )}
              {!loading && feeds.length > 0 && (
                <>
                  {filteredFeeds.length === 0 && (
                    <Card className="border border-surface-border p-5 text-sm text-text-secondary">
                      No followed podcasts match this search.
                    </Card>
                  )}
                  <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                    {filteredFeeds.map(feed => (
                      <Link
                        key={feed.id}
                        to={'/podcasts/show/' + encodeURIComponent(feed.id)}
                        className="flex items-start gap-4 rounded-lg border border-surface-border bg-surface-2/80 p-4 transition-colors duration-150 ease-out hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                      >
                        <PodcastArtwork feedId={feed.id} title={feed.title} imageURL={feed.imageUrl} />
                        <span className="min-w-0">
                          <span className="block truncate font-semibold text-text-main">{feed.title}</span>
                          <span className="block truncate text-xs text-text-secondary">
                            {feed.author || 'Independent publisher'}
                          </span>
                          <span className="mt-2 block text-xs text-text-subtle">
                            {feed.episodeCount} available episodes
                          </span>
                        </span>
                      </Link>
                    ))}
                  </div>
                </>
              )}
            </section>
          </div>
        )}

        {activeTab === 'episodes' && (
          <section aria-label="Podcast episodes triage and search" className="space-y-4">
            <Card className="border border-surface-border p-5 space-y-4">
              <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                <div className="flex flex-wrap items-center gap-2" role="group" aria-label="Filter episode view">
                  {EPISODE_VIEW_OPTIONS.map(option => (
                    <Chip
                      key={option.id}
                      selected={episodesView === option.id}
                      aria-pressed={episodesView === option.id}
                      onClick={() => setEpisodesView(option.id)}
                    >
                      {option.label}
                    </Chip>
                  ))}
                </div>
                <TextInput
                  type="search"
                  aria-label="Search episodes across shows"
                  placeholder="Search episodes across all shows…"
                  leftIcon={<Search size={16} className="shrink-0 text-text-subtle" />}
                  value={episodesQuery}
                  onChange={event => setEpisodesQuery(event.target.value)}
                  onClear={() => setEpisodesQuery('')}
                  className="w-full md:w-80"
                />
              </div>

              {viewEpisodesError && (
                <p role="alert" className="rounded-lg border border-error/30 bg-error/10 p-3 text-sm text-error">
                  {viewEpisodesError}
                </p>
              )}

              {viewEpisodesLoading ? (
                <div role="status" aria-label="Loading episodes…" className="space-y-3">
                  <span className="sr-only">Loading episodes…</span>
                  <Skeleton className="h-28 w-full rounded-lg" />
                  <Skeleton className="h-28 w-full rounded-lg" />
                  <Skeleton className="h-28 w-full rounded-lg" />
                </div>
              ) : viewEpisodes.length === 0 ? (
                <div className="rounded-lg border border-surface-border bg-surface-1 p-8 text-center text-sm text-text-secondary">
                  {debouncedEpisodesQuery.trim()
                    ? 'No episodes match your search in this view.'
                    : 'No episodes in this view yet.'}
                </div>
              ) : (
                <div className="space-y-3">
                  {viewEpisodes.map(episode => {
                    const liveDownload = downloadsByEpisodeId.get(episode.id);
                    const downloadStatus = liveDownload?.status ?? episode.downloadStatus;
                    const downloadActive =
                      downloadStatus === 'queued' ||
                      downloadStatus === 'downloading' ||
                      downloadStatus === 'paused';
                    const downloadCompleted = downloadStatus === 'completed';
                    const hasPosition = (episode.positionMs ?? 0) > 0;
                    const progressPct =
                      episode.durationMs > 0 && hasPosition
                        ? Math.min(100, Math.max(2, Math.round(((episode.positionMs ?? 0) / episode.durationMs) * 100)))
                        : 0;
                    const feedForEpisode = buildFallbackFeed(episode, feeds);

                    return (
                      <article
                        key={episode.id}
                        className="rounded-lg border border-surface-border bg-surface-1 p-4 transition-colors hover:bg-surface-hover"
                      >
                        <div className="flex flex-wrap items-start justify-between gap-2">
                          <div className="min-w-0 flex-1">
                            <div className="flex flex-wrap items-center gap-2">
                              <Link
                                to={'/podcasts/show/' + encodeURIComponent(episode.feedId)}
                                className="text-xs font-semibold text-brand hover:underline"
                              >
                                {episode.feedTitle}
                              </Link>
                              {episode.starred && (
                                <span className="inline-flex items-center gap-1 rounded-full bg-brand/15 px-2 py-0.5 text-[11px] font-semibold text-brand">
                                  <Star size={11} aria-hidden="true" /> Starred
                                </span>
                              )}
                              {episode.isPlayed && (
                                <span className="inline-flex items-center gap-1 rounded-full bg-success/15 px-2 py-0.5 text-[11px] font-semibold text-success">
                                  <Check size={11} aria-hidden="true" /> Played
                                </span>
                              )}
                              {episode.archived && (
                                <span className="rounded-full bg-surface-3 px-2 py-0.5 text-[11px] font-medium text-text-secondary">
                                  Archived
                                </span>
                              )}
                              {episode.hasChapters && (
                                <span className="inline-flex items-center gap-1 rounded-full bg-surface-3 px-2 py-0.5 text-[11px] font-medium text-text-secondary">
                                  <ListChecks size={11} aria-hidden="true" /> Chapters
                                </span>
                              )}
                              {episode.hasTranscript && (
                                <span className="inline-flex items-center gap-1 rounded-full bg-surface-3 px-2 py-0.5 text-[11px] font-medium text-text-secondary">
                                  <FileText size={11} aria-hidden="true" /> Transcript
                                </span>
                              )}
                              {downloadCompleted && (
                                <span className="rounded-full bg-brand/15 px-2 py-0.5 text-[11px] font-semibold text-brand">
                                  Offline
                                </span>
                              )}
                            </div>

                            <h3 className="mt-1.5 text-base font-semibold text-text-main">
                              <Link
                                to={'/podcasts/episode/' + encodeURIComponent(episode.id)}
                                className="hover:text-brand hover:underline"
                              >
                                {episode.title}
                              </Link>
                            </h3>

                            <p className="mt-1 text-xs text-text-subtle">
                              {episode.publishedAt
                                ? new Date(episode.publishedAt).toLocaleDateString()
                                : 'Date unavailable'}
                              {episode.durationMs ? ` · ${Math.round(episode.durationMs / 60000)} min` : ''}
                              {hasPosition && !episode.isPlayed
                                ? ` · Saved at ${Math.floor((episode.positionMs ?? 0) / 60000)} min`
                                : ''}
                            </p>

                            {episode.description && (
                              <p className="mt-2 line-clamp-2 text-sm text-text-secondary">
                                {episode.description}
                              </p>
                            )}

                            {progressPct > 0 && !episode.isPlayed && (
                              <div className="mt-2.5 h-1.5 w-full max-w-md overflow-hidden rounded-full bg-surface-3">
                                <div
                                  className="h-full rounded-full bg-brand"
                                  style={{ width: `${progressPct}%` }}
                                />
                              </div>
                            )}
                          </div>
                        </div>

                        <div className="mt-3 flex flex-wrap items-center gap-2">
                          {episode.hasAudio && (
                            <Button
                              variant="primary"
                              size="sm"
                              leftIcon={<Play size={13} />}
                              onClick={() => {
                                const track = podcastEpisodeToTrack(episode, feedForEpisode);
                                void useStore.getState().playSong(track, [track], 'podcast', 0);
                              }}
                              aria-label={
                                (hasPosition && !episode.isPlayed ? 'Resume ' : 'Play ') + episode.title
                              }
                            >
                              {hasPosition && !episode.isPlayed ? 'Resume' : 'Play'}
                            </Button>
                          )}
                          {episode.hasAudio && (
                            <>
                              <Button
                                variant="secondary"
                                size="sm"
                                onClick={() => queueEpisodeAction(episode, feedForEpisode, 'next')}
                                aria-label={'Play ' + episode.title + ' next'}
                              >
                                Play Next
                              </Button>
                              <Button
                                variant="secondary"
                                size="sm"
                                onClick={() => queueEpisodeAction(episode, feedForEpisode, 'end')}
                                aria-label={'Add ' + episode.title + ' to queue'}
                              >
                                Add to Queue
                              </Button>
                            </>
                          )}
                          <Button
                            variant="secondary"
                            size="sm"
                            disabled={triagingEpisodeId !== null}
                            onClick={() =>
                              void patchViewEpisodeTriage(episode, { starred: !episode.starred })
                            }
                            aria-label={(episode.starred ? 'Unstar ' : 'Star ') + episode.title}
                            leftIcon={<Star size={13} />}
                          >
                            {episode.starred ? 'Starred' : 'Star'}
                          </Button>
                          <Button
                            variant="secondary"
                            size="sm"
                            disabled={triagingEpisodeId !== null}
                            onClick={() =>
                              void patchViewEpisodeTriage(episode, { archived: !episode.archived })
                            }
                            aria-label={(episode.archived ? 'Unarchive ' : 'Archive ') + episode.title}
                            leftIcon={episode.archived ? <ArchiveRestore size={13} /> : <Archive size={13} />}
                          >
                            {episode.archived ? 'Unarchive' : 'Archive'}
                          </Button>
                          <Button
                            variant="secondary"
                            size="sm"
                            disabled={triagingEpisodeId !== null}
                            onClick={() =>
                              void patchViewEpisodeTriage(episode, { isPlayed: !episode.isPlayed })
                            }
                            aria-label={
                              episode.isPlayed
                                ? `Mark ${episode.title} unplayed`
                                : `Mark ${episode.title} played`
                            }
                          >
                            {episode.isPlayed ? 'Mark unplayed' : 'Mark played'}
                          </Button>
                          {episode.hasAudio && (
                            <Button
                              variant="secondary"
                              size="sm"
                              disabled={workingDownloadId !== null}
                              onClick={() => void toggleEpisodeDownload(episode)}
                              leftIcon={<Download size={13} />}
                            >
                              {workingDownloadId === episode.id
                                ? 'Working…'
                                : downloadCompleted
                                ? 'Remove download'
                                : downloadActive
                                ? 'Cancel download'
                                : 'Download'}
                            </Button>
                          )}
                        </div>
                      </article>
                    );
                  })}

                  {viewEpisodesNextOffset !== null && (
                    <Button
                      variant="secondary"
                      className="w-full"
                      disabled={viewEpisodesLoadingMore}
                      onClick={() => void loadMoreViewEpisodes()}
                    >
                      {viewEpisodesLoadingMore ? 'Loading more episodes…' : 'Load more episodes'}
                    </Button>
                  )}
                </div>
              )}
            </Card>
          </section>
        )}

        {activeTab === 'queue' && (
          <section aria-label="Podcast durable queue">
            <Card className="border border-surface-border p-5 space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <h2 className="text-xl font-semibold text-text-main">Up Next Queue</h2>
                  <p className="mt-1 text-xs text-text-secondary">
                    Persistent podcast listening queue saved across sessions.
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    variant="primary"
                    size="sm"
                    disabled={queueLoading || !queueItems.some(item => item.hasAudio)}
                    onClick={() => playDurableQueue()}
                    leftIcon={<Play size={14} />}
                  >
                    Play Queue
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Refresh podcast queue"
                    disabled={queueLoading}
                    onClick={() => void fetchQueue()}
                  >
                    <RefreshCw size={17} className={queueLoading ? 'animate-spin' : ''} />
                  </Button>
                </div>
              </div>

              {queueError && (
                <p role="alert" className="rounded-lg border border-error/30 bg-error/10 p-3 text-sm text-error">
                  {queueError}
                </p>
              )}

              {queueLoading ? (
                <div role="status" aria-label="Loading podcast queue…" className="space-y-2">
                  <span className="sr-only">Loading podcast queue…</span>
                  <Skeleton className="h-20 w-full rounded-lg" />
                  <Skeleton className="h-20 w-full rounded-lg" />
                </div>
              ) : queueItems.length === 0 ? (
                <div className="rounded-lg border border-surface-border bg-surface-1 p-8 text-center text-sm text-text-secondary">
                  Your durable podcast queue is empty. Use Play Next or Add to Queue on any episode to queue it here.
                </div>
              ) : (
                <div className="space-y-2">
                  {queueItems.map((item, index) => (
                    <article
                      key={item.id}
                      className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-surface-border bg-surface-1 p-4"
                    >
                      <div className="flex min-w-0 flex-1 items-start gap-3">
                        <span className="mt-0.5 inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-surface-3 text-xs font-semibold text-text-secondary">
                          {index + 1}
                        </span>
                        <div className="min-w-0 flex-1">
                          <Link
                            to={'/podcasts/show/' + encodeURIComponent(item.feedId)}
                            className="text-xs font-semibold text-brand hover:underline"
                          >
                            {item.feedTitle}
                          </Link>
                          <h3 className="mt-0.5 truncate text-sm font-semibold text-text-main">
                            <Link
                              to={'/podcasts/episode/' + encodeURIComponent(item.id)}
                              className="hover:text-brand hover:underline"
                            >
                              {item.title}
                            </Link>
                          </h3>
                          <p className="mt-0.5 text-xs text-text-subtle">
                            {item.durationMs ? `${Math.round(item.durationMs / 60000)} min` : 'Duration unavailable'}
                            {item.positionMs ? ` · Saved at ${Math.floor(item.positionMs / 60000)} min` : ''}
                          </p>
                        </div>
                      </div>
                      <div className="flex items-center gap-1.5">
                        {item.hasAudio && (
                          <Button
                            variant="primary"
                            size="sm"
                            onClick={() => playDurableQueue(item.id)}
                            aria-label={'Play ' + item.title}
                            leftIcon={<Play size={13} />}
                          >
                            Play
                          </Button>
                        )}
                        <Button
                          variant="secondary"
                          size="icon"
                          disabled={queueSaving || index === 0}
                          onClick={() => void moveQueueItem(index, -1)}
                          aria-label={'Move ' + item.title + ' up'}
                        >
                          <ArrowUp size={15} />
                        </Button>
                        <Button
                          variant="secondary"
                          size="icon"
                          disabled={queueSaving || index === queueItems.length - 1}
                          onClick={() => void moveQueueItem(index, 1)}
                          aria-label={'Move ' + item.title + ' down'}
                        >
                          <ArrowDown size={15} />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          disabled={queueSaving}
                          onClick={() => void removeQueueItem(item.id)}
                          aria-label={'Remove ' + item.title + ' from queue'}
                        >
                          <Trash2 size={15} />
                        </Button>
                      </div>
                    </article>
                  ))}
                </div>
              )}
            </Card>
          </section>
        )}

        {activeTab === 'downloads' && (
          <DownloadsSection
            downloads={downloads}
            downloadStorage={downloadStorage}
            downloadsLoading={downloadsLoading}
            downloadsError={downloadsError}
            downloadsFilter={downloadsFilter}
            setDownloadsFilter={setDownloadsFilter}
            workingDownloadId={workingDownloadId}
            confirmRemoveDownloadId={confirmRemoveDownloadId}
            setConfirmRemoveDownloadId={setConfirmRemoveDownloadId}
            selectedPodcastId={selectedPodcastId}
            sseHealthy={sseHealthy}
            onRefresh={() => setRevision(value => value + 1)}
            onAction={changeLibraryDownload}
          />
        )}
      </div>
    </Page>
  );
};

export const PodcastShow: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const [feed, setFeed] = useState<PodcastFeed | null>(null);
  const [episodes, setEpisodes] = useState<PodcastEpisode[]>([]);

  const [episodeFilter, setEpisodeFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'played' | 'unplayed' | 'in_progress' | 'not_started'>('all');
  const [updatingEpisode, setUpdatingEpisode] = useState<string | null>(null);
  const [triagingEpisodeId, setTriagingEpisodeId] = useState<string | null>(null);
  const [downloads, setDownloads] = useState<Record<string, PodcastDownload>>({});
  const [downloadStorage, setDownloadStorage] = useState<PodcastStorage | null>(null);
  const [confirmDeleteDownload, setConfirmDeleteDownload] = useState<string | null>(null);
  const selectedPodcastId = useStore(state => state.currentSong?.podcastEpisodeId);
  const showToast = useStore(state => state.showToast);
  const [workingDownload, setWorkingDownload] = useState<string | null>(null);
  const [downloadError, setDownloadError] = useState('');
  // Bulk edits are explicitly limited to currently loaded episode pages.
  // Each revision is freshly read; conflict errors are reported, not forced.
  const [bulkTarget, setBulkTarget] = useState<boolean | null>(null);
  const [bulkRunning, setBulkRunning] = useState(false);
  const [bulkProcessed, setBulkProcessed] = useState(0);
  const [bulkSummary, setBulkSummary] = useState('');
  const bulkController = useRef<AbortController | null>(null);
  const [nextOffset, setNextOffset] = useState<number | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [working, setWorking] = useState(false);
  const [refreshPending, setRefreshPending] = useState(false);
  const [refreshMessage, setRefreshMessage] = useState('');
  const [refreshStatus, setRefreshStatus] = useState<PodcastRefreshStatus | null>(null);
  const [refreshCount, setRefreshCount] = useState(0);
  const prevRefreshCountRef = useRef<number | null>(null);
  const [autoRefreshEnabled, setAutoRefreshEnabled] = useState(false);
  const [autoRefreshSaving, setAutoRefreshSaving] = useState(false);
  const [autoDownload, setAutoDownload] = useState<PodcastAutoDownloadPolicy | null>(null);
  const [autoDownloadSaving, setAutoDownloadSaving] = useState(false);
  const [autoDownloadMessage, setAutoDownloadMessage] = useState('');
  // A settings request for show A must not overwrite show B after navigation.
  const autoDownloadFeedRef = useRef(id);
  autoDownloadFeedRef.current = id;
  const [retention, setRetention] = useState<PodcastRetentionPolicy | null>(null);
  const [retentionSaving, setRetentionSaving] = useState(false);
  const [retentionMessage, setRetentionMessage] = useState('');
  const retentionFeedRef = useRef(id);
  retentionFeedRef.current = id;
  const [showPlayback, setShowPlayback] = useState<PodcastShowPlaybackSettings | null>(null);
  const [skipIntroInput, setSkipIntroInput] = useState('0');
  const [skipOutroInput, setSkipOutroInput] = useState('0');
  const [showPlaybackSaving, setShowPlaybackSaving] = useState(false);
  const [showPlaybackMessage, setShowPlaybackMessage] = useState('');
  const showPlaybackFeedRef = useRef(id);
  showPlaybackFeedRef.current = id;
  const [revision, setRevision] = useState(0);
  const lastCheckedRef = useRef<number | null>(null);
  const navigate = useNavigate();
  useEffect(() => () => bulkController.current?.abort(), []);

  const handleSseSnapshot = useCallback((snapshot: PodcastEventsSnapshot) => {
    setDownloads(Object.fromEntries(snapshot.downloads.map(row => [row.episodeId, row])));
    if (snapshot.storage) setDownloadStorage(snapshot.storage);
    const nextRefreshCount = snapshot.refreshCount ?? 0;
    setRefreshCount(nextRefreshCount);
    if (prevRefreshCountRef.current !== null && prevRefreshCountRef.current > 0 && nextRefreshCount === 0) {
      setRevision(v => v + 1);
    }
    prevRefreshCountRef.current = nextRefreshCount;
  }, []);

  const sseHealthy = usePodcastEventsSSE(Boolean(id), handleSseSnapshot);

  useEffect(() => {
    let mounted = true;
    void api
      .getSetting('podcasts_preview_enabled')
      .then(value => {
        if (mounted) setAutoRefreshEnabled(value === 'true');
      })
      .catch(() => {
        if (mounted) setAutoRefreshEnabled(false);
      });
    return () => {
      mounted = false;
    };
  }, []);

  useEffect(() => {
    if (!id) return;
    const controller = new AbortController();
    setShowPlayback(null);
    setSkipIntroInput('0');
    setSkipOutroInput('0');
    setShowPlaybackSaving(false);
    setShowPlaybackMessage('');
    void podcastsAPI
      .getShowPlaybackSettings(id, controller.signal)
      .then(({ settings }) => {
        if (controller.signal.aborted) return;
        setShowPlayback(settings);
        setSkipIntroInput(String(settings.skipIntroSeconds));
        setSkipOutroInput(String(settings.skipOutroSeconds));
      })
      .catch(err => {
        if (!controller.signal.aborted) setShowPlaybackMessage(readableError(err));
      });
    return () => controller.abort();
  }, [id]);

  const saveShowPlayback = async (next: PodcastShowPlaybackSettings) => {
    if (!id || showPlaybackSaving || !showPlayback) return;
    const feedID = id;
    const previous = showPlayback;
    setShowPlayback(next);
    setSkipIntroInput(String(next.skipIntroSeconds));
    setSkipOutroInput(String(next.skipOutroSeconds));
    setShowPlaybackSaving(true);
    setShowPlaybackMessage('');
    try {
      const { settings } = await podcastsAPI.setShowPlaybackSettings(feedID, next);
      if (showPlaybackFeedRef.current !== feedID) return;
      setShowPlayback(settings);
      setSkipIntroInput(String(settings.skipIntroSeconds));
      setSkipOutroInput(String(settings.skipOutroSeconds));
      setFeed(current =>
        current
          ? {
              ...current,
              defaultSpeed: settings.defaultSpeed,
              skipIntroSeconds: settings.skipIntroSeconds,
              skipOutroSeconds: settings.skipOutroSeconds,
            }
          : current,
      );
      window.dispatchEvent(
        new CustomEvent('viib:podcast-show-settings-changed', {
          detail: { feedId: feedID, settings },
        }),
      );
      setShowPlaybackMessage('Show playback defaults saved.');
    } catch (err) {
      if (showPlaybackFeedRef.current !== feedID) return;
      setShowPlayback(previous);
      setSkipIntroInput(String(previous.skipIntroSeconds));
      setSkipOutroInput(String(previous.skipOutroSeconds));
      setShowPlaybackMessage(readableError(err));
    } finally {
      if (showPlaybackFeedRef.current === feedID) setShowPlaybackSaving(false);
    }
  };

  useEffect(() => {
    if (!id) return;
    const controller = new AbortController();
    setAutoDownload(null);
    setAutoDownloadSaving(false);
    setAutoDownloadMessage('');
    void podcastsAPI
      .getAutoDownloadPolicy(id, controller.signal)
      .then(({ policy }) => {
        if (!controller.signal.aborted) setAutoDownload(policy);
      })
      .catch(err => {
        if (!controller.signal.aborted) setAutoDownloadMessage(readableError(err));
      });
    return () => controller.abort();
  }, [id]);

  const saveAutoDownload = async (policy: PodcastAutoDownloadPolicy) => {
    if (!id || autoDownloadSaving || !autoDownload) return;
    const feedID = id;
    const previous = autoDownload;
    // Controlled inputs change immediately. The backend is authoritative;
    // revert a rejected save, but never apply a late response to another show.
    setAutoDownload(policy);
    setAutoDownloadSaving(true);
    setAutoDownloadMessage('');
    try {
      const updated = await podcastsAPI.setAutoDownloadPolicy(feedID, policy);
      if (autoDownloadFeedRef.current !== feedID) return;
      setAutoDownload(updated.policy);
      setAutoDownloadMessage(
        updated.policy.enabled
          ? 'Automatic downloads enabled for new episodes discovered on future feed refreshes.'
          : 'Automatic downloads turned off. Existing downloads are unchanged.',
      );
    } catch (err) {
      if (autoDownloadFeedRef.current !== feedID) return;
      setAutoDownload(previous);
      setAutoDownloadMessage(readableError(err));
    } finally {
      if (autoDownloadFeedRef.current === feedID) setAutoDownloadSaving(false);
    }
  };

  useEffect(() => {
    if (!id) return;
    const controller = new AbortController();
    setRetention(null);
    setRetentionSaving(false);
    setRetentionMessage('');
    void podcastsAPI
      .getRetentionPolicy(id, controller.signal)
      .then(({ policy }) => {
        if (!controller.signal.aborted) setRetention(policy);
      })
      .catch(() => {
        // Older backends or fixture mocks without retention endpoint remain usable.
      });
    return () => controller.abort();
  }, [id]);

  const saveRetention = async (keepLatestCount: number) => {
    if (!id || retentionSaving || !retention) return;
    const feedID = id;
    const previous = retention;
    setRetention({ keepLatestCount });
    setRetentionSaving(true);
    setRetentionMessage('');
    try {
      const updated = await podcastsAPI.setRetentionPolicy(feedID, { keepLatestCount });
      if (retentionFeedRef.current !== feedID) return;
      setRetention(updated.policy);
      setRetentionMessage(
        updated.policy.keepLatestCount === 0
          ? 'Retention updated: keeping all downloaded episodes.'
          : `Retention updated: keeping latest ${updated.policy.keepLatestCount} downloaded episode${
              updated.policy.keepLatestCount === 1 ? '' : 's'
            }.`,
      );
    } catch (err) {
      if (retentionFeedRef.current !== feedID) return;
      setRetention(previous);
      const msg = readableError(err);
      setRetentionMessage(msg);
      showToast({ type: 'error', message: msg });
    } finally {
      if (retentionFeedRef.current === feedID) setRetentionSaving(false);
    }
  };

  const changeAutoRefresh = async (checked: boolean) => {
    setAutoRefreshSaving(true);
    try {
      await api.setSetting('podcasts_preview_enabled', checked ? 'true' : 'false');
      setAutoRefreshEnabled(checked);
      setRefreshMessage(checked ? 'Automatic podcast refresh enabled.' : 'Automatic podcast refresh paused.');
      window.dispatchEvent(new Event('viib:podcasts-refresh-changed'));
    } catch (err) {
      setRefreshMessage(readableError(err));
    } finally {
      setAutoRefreshSaving(false);
    }
  };

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError('');
    void Promise.all([
      podcastsAPI.getFeed(id ?? '', controller.signal),
      podcastsAPI.listEpisodes(id ?? '', controller.signal),
    ])
      .then(([details, result]) => {
        if (controller.signal.aborted) return;
        setFeed(details.feed);
        setEpisodes(result.episodes);
        setNextOffset(result.nextOffset ?? null);
      })
      .catch(err => {
        if (!controller.signal.aborted) {
          setFeed(null);
          setError(readableError(err));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [id, revision]);

  // Download status is initially loaded and only polled when SSE is disconnected.
  // Never fetch or render remote signed enclosure URLs in the browser.
  useEffect(() => {
    if (!id) return;
    const controller = new AbortController();
    const poll = () => {
      void podcastsAPI
        .listDownloads(controller.signal)
        .then(({ downloads: rows, storage }) => {
          if (controller.signal.aborted) return;
          setDownloads(Object.fromEntries(rows.map(row => [row.episodeId, row])));
          if (storage) setDownloadStorage(storage);
        })
        .catch(() => {
          /* Streaming remains available on older backend builds. */
        });
    };
    poll();
    if (sseHealthy) {
      return () => {
        controller.abort();
      };
    }
    const timer = window.setInterval(poll, 3500);
    return () => {
      controller.abort();
      window.clearInterval(timer);
    };
  }, [id, sseHealthy]);

  const changeDownload = async (episode: PodcastEpisode, action: 'queue' | 'cancel' | 'remove') => {
    if (workingDownload) return;
    if (action === 'remove' && selectedPodcastId === episode.id) {
      setDownloadError('Stop or switch away from this episode before removing its offline copy.');
      return;
    }
    setDownloadError('');
    setWorkingDownload(episode.id);
    try {
      if (action === 'remove') {
        await podcastsAPI.removeDownload(episode.id);
        setDownloads(current => {
          const next = { ...current };
          delete next[episode.id];
          return next;
        });
        setConfirmDeleteDownload(null);
      } else {
        const { download } =
          action === 'cancel'
            ? await podcastsAPI.cancelDownload(episode.id)
            : await podcastsAPI.queueDownload(episode.id);
        setDownloads(current => ({ ...current, [episode.id]: download }));
      }
    } catch (err) {
      setDownloadError(readableError(err));
    } finally {
      setWorkingDownload(null);
    }
  };

  useEffect(() => {
    if (!id) return;
    lastCheckedRef.current = null;
    const controller = new AbortController();
    const updateStatus = () => {
      void podcastsAPI
        .refreshStatus(id, controller.signal)
        .then(status => {
          if (controller.signal.aborted) return;
          setRefreshStatus(status);
          const previousCheck = lastCheckedRef.current;
          if (previousCheck === null || status.lastCheckedAt > previousCheck) {
            const first = previousCheck === null;
            lastCheckedRef.current = status.lastCheckedAt;
            if (!first) setRevision(v => v + 1);
            if (status.lastErrorCode) {
              setRefreshMessage('Feed refresh failed. Cached episodes remain available.');
            } else if (!first) {
              setRefreshMessage('Podcast feed updated.');
            }
          }
        })
        .catch(() => {
          /* Episode browsing remains usable without status API. */
        });
    };
    updateStatus();
    if (sseHealthy) {
      return () => {
        controller.abort();
      };
    }
    const timer = window.setInterval(updateStatus, 10000);
    return () => {
      controller.abort();
      window.clearInterval(timer);
    };
  }, [id, sseHealthy, refreshCount]);

  const loadMoreEpisodes = async () => {
    if (!id || nextOffset === null || loadingMore) return;
    setLoadingMore(true);
    setError('');
    try {
      const page = await podcastsAPI.listEpisodes(id, undefined, nextOffset);
      setEpisodes(previous => {
        const seen = new Set(previous.map(episode => episode.id));
        return [...previous, ...page.episodes.filter(episode => !seen.has(episode.id))];
      });
      setNextOffset(page.nextOffset ?? null);
    } catch (err) {
      setError(readableError(err));
    } finally {
      setLoadingMore(false);
    }
  };

  const setEpisodePlayed = async (episode: PodcastEpisode) => {
    if (updatingEpisode || bulkRunning) return;
    setUpdatingEpisode(episode.id);
    setError('');
    try {
      const progress = await podcastsAPI.getProgress(episode.id);
      const saved = await podcastsAPI.setProgress(episode.id, {
        positionMs: progress.positionMs,
        listenedMs: progress.listenedMs,
        isPlayed: !progress.isPlayed,
        expectedRevision: progress.revision,
      });
      setEpisodes(current =>
        current.map(row =>
          row.id === episode.id
            ? {
                ...row,
                isPlayed: saved.isPlayed,
                positionMs: saved.positionMs,
                listenedMs: saved.listenedMs,
                progressRevision: saved.revision,
              }
            : row,
        ),
      );
    } catch (err) {
      setError(readableError(err));
    } finally {
      setUpdatingEpisode(null);
    }
  };

  const patchEpisodeTriage = async (
    episode: PodcastEpisode,
    patch: { starred?: boolean; archived?: boolean },
  ) => {
    if (triagingEpisodeId || bulkRunning) return;
    const previous = episode;
    setTriagingEpisodeId(episode.id);
    setError('');
    setEpisodes(current =>
      current.map(row => (row.id === episode.id ? { ...row, ...patch } : row)),
    );
    try {
      const { state } = await podcastsAPI.patchEpisodeState(episode.id, patch);
      setEpisodes(current =>
        current.map(row =>
          row.id === episode.id
            ? {
                ...row,
                starred: state.starred,
                archived: state.archived,
                isPlayed: state.isPlayed,
                positionMs: state.positionMs,
                listenedMs: state.listenedMs,
                progressRevision: state.revision,
              }
            : row,
        ),
      );
    } catch (err) {
      setEpisodes(current =>
        current.map(row => (row.id === episode.id ? previous : row)),
      );
      const msg = readableError(err);
      setError(msg);
      showToast({ type: 'error', message: msg });
    } finally {
      setTriagingEpisodeId(null);
    }
  };

  const applyBulkPlayed = async () => {
    if (bulkTarget === null || bulkRunning || updatingEpisode) return;
    const controller = new AbortController();
    bulkController.current = controller;
    const target = bulkTarget;
    const candidates = episodes.filter(episode => Boolean(episode.isPlayed) !== target);
    let updated = 0,
      failed = 0,
      processed = 0;
    setBulkRunning(true);
    setBulkProcessed(0);
    setBulkSummary('');
    setError('');
    try {
      for (const episode of candidates) {
        if (controller.signal.aborted) break;
        try {
          const progress = await podcastsAPI.getProgress(episode.id, controller.signal);
          if (progress.isPlayed !== target) {
            const saved = await podcastsAPI.setProgress(
              episode.id,
              {
                positionMs: progress.positionMs,
                listenedMs: progress.listenedMs,
                isPlayed: target,
                expectedRevision: progress.revision,
              },
              controller.signal,
            );
            setEpisodes(current =>
              current.map(item =>
                item.id === episode.id
                  ? {
                      ...item,
                      isPlayed: saved.isPlayed,
                      positionMs: saved.positionMs,
                      listenedMs: saved.listenedMs,
                      progressRevision: saved.revision,
                    }
                  : item,
              ),
            );
            updated++;
          } else {
            // Another session already applied the requested state.
            setEpisodes(current =>
              current.map(item =>
                item.id === episode.id
                  ? {
                      ...item,
                      isPlayed: progress.isPlayed,
                      positionMs: progress.positionMs,
                      listenedMs: progress.listenedMs,
                      progressRevision: progress.revision,
                    }
                  : item,
              ),
            );
          }
        } catch {
          if (controller.signal.aborted) break;
          // CAS conflicts and network failures do not override newer progress.
          failed++;
        }
        processed++;
        setBulkProcessed(processed);
      }
    } finally {
      bulkController.current = null;
      setBulkRunning(false);
      setBulkTarget(null);
      setBulkSummary(
        `Updated ${updated} of ${candidates.length} loaded episodes; ${failed} failed.${
          controller.signal.aborted ? ' Stopped early.' : ''
        }`,
      );
    }
  };

  const requestRefresh = async () => {
    if (!id) return;
    setRefreshPending(true);
    setRefreshMessage('');
    try {
      await podcastsAPI.refresh(id);
      setRefreshMessage('Refresh queued. Episodes will update when the publisher responds.');
    } catch (err) {
      setRefreshMessage(readableError(err));
    } finally {
      setRefreshPending(false);
    }
  };

  const unsubscribe = async () => {
    if (!id) return;
    setWorking(true);
    try {
      await podcastsAPI.unsubscribe(id);
      navigate('/podcasts');
    } catch (err) {
      setError(readableError(err));
      setWorking(false);
    }
  };

  // The Go API pages episodes by subscription; these status filters deliberately
  // operate on loaded rows only, without implying that the full RSS archive
  // has been searched or changing the shared playback queue.
  const visibleEpisodes = episodes.filter(episode => {
    const played = Boolean(episode.isPlayed);
    const hasPosition = (episode.positionMs ?? 0) > 0;
    const matchesText = (episode.title + ' ' + episode.description)
      .toLowerCase()
      .includes(episodeFilter.trim().toLowerCase());
    return (
      matchesText &&
      (statusFilter === 'all' ||
        (statusFilter === 'played' && played) ||
        (statusFilter === 'unplayed' && !played) ||
        (statusFilter === 'in_progress' && !played && hasPosition) ||
        (statusFilter === 'not_started' && !played && !hasPosition))
    );
  });

  const currentRetentionCount = retention?.keepLatestCount ?? 0;
  const retentionOptions = RETENTION_OPTIONS.includes(currentRetentionCount)
    ? RETENTION_OPTIONS
    : [...RETENTION_OPTIONS, currentRetentionCount].sort((a, b) => a - b);

  return (
    <Page>
      <div className="mx-auto w-full max-w-5xl space-y-6">
        <Link
          to="/podcasts"
          className="inline-flex items-center gap-2 text-sm text-text-secondary hover:text-text-main"
        >
          <ArrowLeft size={17} /> Back to Podcasts
        </Link>

        {loading && (
          <div role="status" aria-label="Loading episodes…" className="space-y-4">
            <span className="sr-only">Loading episodes…</span>
            <Skeleton className="h-44 w-full rounded-lg" />
            <Skeleton className="h-28 w-full rounded-lg" />
            <Skeleton className="h-28 w-full rounded-lg" />
          </div>
        )}

        {error && (
          <p role="alert" className="rounded-lg border border-error/30 bg-error/10 p-3 text-error">
            {error}
          </p>
        )}

        {!loading && feed && (
          <>
            <PageHeader
              heading={feed.title}
              subtitle={feed.author || 'Independent publisher'}
              className="mb-2"
              actions={
                <div className="flex flex-wrap items-center gap-2.5">
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => void requestRefresh()}
                    disabled={
                      !autoRefreshEnabled ||
                      autoRefreshSaving ||
                      refreshPending ||
                      refreshStatus?.refreshing
                    }
                    leftIcon={
                      <RefreshCw
                        size={15}
                        className={
                          refreshPending || refreshStatus?.refreshing ? 'animate-spin' : ''
                        }
                      />
                    }
                  >
                    {refreshPending || refreshStatus?.refreshing
                      ? 'Refreshing…'
                      : 'Refresh feed'}
                  </Button>
                  {!confirmRemove ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setConfirmRemove(true)}
                      leftIcon={<Trash2 size={15} />}
                      className="hover:text-error"
                    >
                      Unfollow
                    </Button>
                  ) : (
                    <div className="flex flex-wrap items-center gap-2 text-sm">
                      <span className="text-text-secondary">
                        Unfollow? Episode history is retained.
                      </span>
                      <Button
                        variant="secondary"
                        size="sm"
                        disabled={working}
                        onClick={() => void unsubscribe()}
                        className="border border-error text-error"
                      >
                        Confirm
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setConfirmRemove(false)}
                      >
                        Cancel
                      </Button>
                    </div>
                  )}
                </div>
              }
            />

            <Card className="border border-surface-border p-6 space-y-5">
              <div className="flex items-start gap-5">
                <PodcastArtwork
                  feedId={feed.id}
                  title={feed.title}
                  imageURL={feed.imageUrl}
                  large
                />
                <div className="min-w-0 flex-1 space-y-3">
                  {feed.description && (
                    <p className="text-sm text-text-secondary">{feed.description}</p>
                  )}
                  <div className="flex flex-wrap items-center gap-3 text-xs text-text-subtle">
                    {refreshStatus?.lastSuccessAt ? (
                      <span>
                        Last updated {new Date(refreshStatus.lastSuccessAt).toLocaleString()}
                      </span>
                    ) : null}
                    {refreshCount > 0 && (
                      <span className="inline-flex items-center gap-1 text-brand">
                        <Loader2 size={12} className="animate-spin" aria-hidden="true" />
                        Background refresh active
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-3">
                    <ToggleSwitch
                      id="podcast-enable-feed-refresh"
                      aria-label="Enable feed refresh (early access)"
                      checked={autoRefreshEnabled}
                      disabled={autoRefreshSaving}
                      onChange={checked => void changeAutoRefresh(checked)}
                    />
                    <label
                      htmlFor="podcast-enable-feed-refresh"
                      className="cursor-pointer text-sm text-text-secondary"
                    >
                      Enable feed refresh (early access)
                    </label>
                  </div>
                  <p className="text-xs text-text-subtle">
                    Feed refreshing is optional. You can browse and follow RSS podcasts without enabling it.
                  </p>
                  {refreshMessage && (
                    <p role="status" className="text-sm text-text-secondary">
                      {refreshMessage}
                    </p>
                  )}
                </div>
              </div>

              <div className="grid gap-4 lg:grid-cols-2">
                <div className="rounded-lg border border-surface-border bg-surface-1 p-4 space-y-4">
                  <div>
                    <h2 className="text-sm font-semibold text-text-main">
                      Automatic episode downloads
                    </h2>
                    <label className="mt-2 inline-flex cursor-pointer items-center gap-2 text-sm text-text-secondary">
                      <input
                        type="checkbox"
                        aria-label="Automatically download new podcast episodes"
                        checked={autoDownload?.enabled ?? false}
                        disabled={!autoDownload || autoDownloadSaving}
                        onChange={event =>
                          void saveAutoDownload({
                            enabled: event.target.checked,
                            newestCount: autoDownload?.newestCount ?? 1,
                          })
                        }
                        className="h-4 w-4 rounded accent-brand"
                      />
                      Download newly discovered episodes for this show
                    </label>
                    <div className="mt-3 flex flex-wrap items-center gap-2">
                      <label
                        htmlFor="podcast-auto-download-count"
                        className="text-xs text-text-secondary"
                      >
                        Maximum new episodes per refresh
                      </label>
                      <select
                        id="podcast-auto-download-count"
                        aria-label="Maximum auto-download episodes per refresh"
                        value={autoDownload?.newestCount ?? 1}
                        disabled={!autoDownload || autoDownloadSaving}
                        onChange={event =>
                          void saveAutoDownload({
                            enabled: autoDownload?.enabled ?? false,
                            newestCount: Number(event.target.value),
                          })
                        }
                        className={selectClass}
                      >
                        <option value={1}>1 episode</option>
                        <option value={2}>2 episodes</option>
                        <option value={3}>3 episodes</option>
                      </select>
                    </div>
                    <p className="mt-2 text-xs text-text-subtle">
                      Off by default. Only episodes first discovered on future successful RSS refreshes are eligible. No old episodes are downloaded automatically. A global limit of 12 pending downloads applies.
                    </p>
                    {!autoRefreshEnabled && (
                      <p className="mt-2 text-xs text-text-subtle">
                        Feed refresh is currently off. Enable it above for automatic downloads to run.
                      </p>
                    )}
                    {autoDownloadMessage && (
                      <p role="status" className="mt-2 text-xs text-text-secondary">
                        {autoDownloadMessage}
                      </p>
                    )}
                  </div>

                  <div className="border-t border-surface-border pt-3">
                    <h3 className="text-sm font-semibold text-text-main">Download retention</h3>
                    <div className="mt-2 flex flex-wrap items-center gap-2">
                      <label
                        htmlFor="podcast-retention-keep-latest"
                        className="text-xs text-text-secondary"
                      >
                        Keep latest downloaded episodes
                      </label>
                      <select
                        id="podcast-retention-keep-latest"
                        aria-label="Keep latest downloaded episodes"
                        value={currentRetentionCount}
                        disabled={!retention || retentionSaving}
                        onChange={event => void saveRetention(Number(event.target.value))}
                        className={selectClass}
                      >
                        {retentionOptions.map(count => (
                          <option key={count} value={count}>
                            {count === 0
                              ? 'Keep all'
                              : `${count} ${count === 1 ? 'episode' : 'episodes'}`}
                          </option>
                        ))}
                      </select>
                    </div>
                    <p className="mt-2 text-xs text-text-subtle">
                      Starred episodes are protected and will not be removed automatically by retention pruning.
                    </p>
                    {retentionMessage && (
                      <p role="status" className="mt-2 text-xs text-text-secondary">
                        {retentionMessage}
                      </p>
                    )}
                  </div>
                </div>

                <form
                  onSubmit={event => {
                    event.preventDefault();
                    if (!showPlayback) return;
                    const parsedIntro = Math.max(
                      0,
                      Math.min(600, Math.trunc(Number(skipIntroInput) || 0)),
                    );
                    const parsedOutro = Math.max(
                      0,
                      Math.min(600, Math.trunc(Number(skipOutroInput) || 0)),
                    );
                    void saveShowPlayback({
                      defaultSpeed: showPlayback.defaultSpeed,
                      skipIntroSeconds: parsedIntro,
                      skipOutroSeconds: parsedOutro,
                    });
                  }}
                  className="rounded-lg border border-surface-border bg-surface-1 p-4"
                >
                  <h2 className="text-sm font-semibold text-text-main">Show playback defaults</h2>
                  <div className="mt-3 flex flex-wrap items-end gap-4">
                    <div>
                      <label
                        htmlFor="podcast-show-default-speed"
                        className="block text-xs text-text-secondary"
                      >
                        Default playback speed
                      </label>
                      <select
                        id="podcast-show-default-speed"
                        aria-label="Show default playback speed"
                        value={showPlayback?.defaultSpeed ?? 0}
                        disabled={!showPlayback || showPlaybackSaving}
                        onChange={event => {
                          if (!showPlayback) return;
                          void saveShowPlayback({
                            ...showPlayback,
                            defaultSpeed: Number(event.target.value),
                          });
                        }}
                        className={`mt-1 ${selectClass}`}
                      >
                        <option value={0}>Use global default</option>
                        <option value={0.75}>0.75×</option>
                        <option value={1}>1×</option>
                        <option value={1.25}>1.25×</option>
                        <option value={1.5}>1.5×</option>
                        <option value={1.75}>1.75×</option>
                        <option value={2}>2×</option>
                      </select>
                    </div>
                    <div>
                      <label
                        htmlFor="podcast-show-skip-intro"
                        className="block text-xs text-text-secondary"
                      >
                        Skip intro (seconds)
                      </label>
                      <input
                        id="podcast-show-skip-intro"
                        type="number"
                        min={0}
                        max={600}
                        step={5}
                        aria-label="Show skip intro seconds"
                        value={skipIntroInput}
                        disabled={!showPlayback || showPlaybackSaving}
                        onChange={event => setSkipIntroInput(event.target.value)}
                        className={numberInputClass}
                      />
                    </div>
                    <div>
                      <label
                        htmlFor="podcast-show-skip-outro"
                        className="block text-xs text-text-secondary"
                      >
                        Skip outro (seconds)
                      </label>
                      <input
                        id="podcast-show-skip-outro"
                        type="number"
                        min={0}
                        max={600}
                        step={5}
                        aria-label="Show skip outro seconds"
                        value={skipOutroInput}
                        disabled={!showPlayback || showPlaybackSaving}
                        onChange={event => setSkipOutroInput(event.target.value)}
                        className={numberInputClass}
                      />
                    </div>
                    <Button
                      type="submit"
                      variant="secondary"
                      size="sm"
                      disabled={!showPlayback || showPlaybackSaving}
                    >
                      Save skip timings
                    </Button>
                  </div>
                  <p className="mt-2 text-xs text-text-subtle">
                    Applies when starting episodes of this show in the shared player. Saved resume positions are not overwritten by skip intro.
                  </p>
                  {showPlaybackMessage && (
                    <p role="status" className="mt-2 text-xs text-text-secondary">
                      {showPlaybackMessage}
                    </p>
                  )}
                </form>
              </div>
            </Card>

            <section>
              {downloadError && (
                <p
                  role="alert"
                  className="mb-3 rounded-lg border border-error/30 bg-error/10 p-3 text-sm text-error"
                >
                  {downloadError}
                </p>
              )}
              {downloadStorage && (
                <p className="mb-3 text-xs text-text-subtle">
                  Offline podcast storage: {(downloadStorage.usedBytes / (1024 * 1024)).toFixed(0)} MiB /{' '}
                  {(downloadStorage.maxBytes / (1024 * 1024 * 1024)).toFixed(1)} GiB reserved
                </p>
              )}
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <h2 className="text-lg font-bold text-text-main">
                  Episodes ({episodes.length} loaded)
                </h2>
                <div aria-label="Loaded episode bulk actions" className="flex flex-wrap items-center gap-2">
                  <Button
                    variant="secondary"
                    size="sm"
                    disabled={bulkRunning || updatingEpisode !== null || episodes.length === 0}
                    onClick={() => {
                      setBulkTarget(true);
                      setBulkSummary('');
                    }}
                    aria-label="Mark loaded episodes played"
                  >
                    Mark loaded played
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    disabled={bulkRunning || updatingEpisode !== null || episodes.length === 0}
                    onClick={() => {
                      setBulkTarget(false);
                      setBulkSummary('');
                    }}
                    aria-label="Mark loaded episodes unplayed"
                  >
                    Mark loaded unplayed
                  </Button>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <label htmlFor="podcast-status-filter" className="text-sm text-text-secondary">
                    Episode status
                  </label>
                  <select
                    id="podcast-status-filter"
                    aria-label="Filter episodes by status"
                    value={statusFilter}
                    onChange={event =>
                      setStatusFilter(event.target.value as typeof statusFilter)
                    }
                    className={selectClass}
                  >
                    <option value="all">All episodes</option>
                    <option value="unplayed">Unplayed</option>
                    <option value="in_progress">In progress</option>
                    <option value="not_started">Not started</option>
                    <option value="played">Played</option>
                  </select>
                  <TextInput
                    type="search"
                    aria-label="Filter loaded episodes"
                    placeholder="Filter loaded episodes"
                    leftIcon={<Search size={15} className="shrink-0 text-text-subtle" />}
                    value={episodeFilter}
                    onChange={event => setEpisodeFilter(event.target.value)}
                    onClear={() => setEpisodeFilter('')}
                    className="w-full sm:w-64"
                  />
                </div>
              </div>
              <p className="mb-3 text-xs text-text-subtle">
                Episode filtering and bulk played controls apply only to loaded pages. Load more to browse the full back catalog.
              </p>
              {bulkTarget !== null && !bulkRunning && (
                <div
                  role="group"
                  aria-label="Confirm bulk episode change"
                  className="mb-3 flex flex-wrap items-center gap-3 rounded-lg border border-surface-border bg-surface-1 p-3 text-sm text-text-secondary"
                >
                  <span>
                    Mark up to{' '}
                    {episodes.filter(row => Boolean(row.isPlayed) !== bulkTarget).length} loaded
                    episodes {bulkTarget ? 'played' : 'unplayed'}? Saved positions will be kept.
                  </span>
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => void applyBulkPlayed()}
                    aria-label={'Confirm mark loaded ' + (bulkTarget ? 'played' : 'unplayed')}
                  >
                    Confirm
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => setBulkTarget(null)}
                  >
                    Cancel
                  </Button>
                </div>
              )}
              {bulkRunning && (
                <div role="status" className="mb-3 flex items-center gap-3 text-sm text-text-secondary">
                  Processed {bulkProcessed} loaded episodes.
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => bulkController.current?.abort()}
                  >
                    Stop bulk update
                  </Button>
                </div>
              )}
              {bulkSummary && (
                <p role="status" className="mb-3 text-sm text-text-secondary">
                  {bulkSummary}
                </p>
              )}
              {episodes.length === 0 && (
                <p className="text-text-secondary">No episodes are available in this feed.</p>
              )}
              {episodes.length > 0 && (
                <p role="status" className="mb-3 text-xs text-text-subtle">
                  Showing {visibleEpisodes.length} of {episodes.length} loaded episodes.
                </p>
              )}
              {episodes.length > 0 && visibleEpisodes.length === 0 && (
                <p className="mb-3 text-sm text-text-secondary">
                  No loaded episodes match these filters. Clear the filters or load more episodes.
                </p>
              )}
              <div className="space-y-2">
                {visibleEpisodes.map(episode => (
                  <Card key={episode.id} className="border border-surface-border p-4">
                    <article>
                      <div className="flex flex-wrap items-center gap-2">
                        {episode.starred && (
                          <span className="inline-flex items-center gap-1 rounded-full bg-brand/15 px-2 py-0.5 text-[11px] font-semibold text-brand">
                            <Star size={11} aria-hidden="true" /> Starred
                          </span>
                        )}
                        {episode.archived && (
                          <span className="rounded-full bg-surface-3 px-2 py-0.5 text-[11px] font-medium text-text-secondary">
                            Archived
                          </span>
                        )}
                        {episode.hasChapters && (
                          <span className="inline-flex items-center gap-1 rounded-full bg-surface-3 px-2 py-0.5 text-[11px] font-medium text-text-secondary">
                            <ListChecks size={11} aria-hidden="true" /> Chapters
                          </span>
                        )}
                        {episode.hasTranscript && (
                          <span className="inline-flex items-center gap-1 rounded-full bg-surface-3 px-2 py-0.5 text-[11px] font-medium text-text-secondary">
                            <FileText size={11} aria-hidden="true" /> Transcript
                          </span>
                        )}
                      </div>
                      <h3 className="mt-1 font-semibold text-text-main">
                        <Link
                          to={'/podcasts/episode/' + encodeURIComponent(episode.id)}
                          className="hover:text-brand hover:underline"
                        >
                          {episode.title}
                        </Link>
                      </h3>
                      <p className="mt-1 text-xs text-text-subtle">
                        {episode.publishedAt
                          ? new Date(episode.publishedAt).toLocaleDateString()
                          : 'Date unavailable'}
                        {episode.durationMs
                          ? ` · ${Math.round(episode.durationMs / 60000)} min`
                          : ''}
                      </p>
                      {episode.description && (
                        <p className="mt-2 line-clamp-3 text-sm text-text-secondary">
                          {episode.description}
                        </p>
                      )}
                      <div className="mt-3 flex flex-wrap items-center gap-2">
                        {episode.positionMs ? (
                          <span className="text-xs text-text-subtle">
                            Position saved at {Math.floor(episode.positionMs / 60000)} min
                          </span>
                        ) : null}
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={() => void setEpisodePlayed(episode)}
                          disabled={updatingEpisode !== null || bulkRunning}
                          aria-label={
                            episode.isPlayed
                              ? `Mark ${episode.title} unplayed`
                              : `Mark ${episode.title} played`
                          }
                        >
                          {updatingEpisode === episode.id
                            ? 'Saving…'
                            : episode.isPlayed
                            ? 'Mark unplayed'
                            : 'Mark played'}
                        </Button>
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={() =>
                            void patchEpisodeTriage(episode, { starred: !episode.starred })
                          }
                          disabled={triagingEpisodeId !== null || bulkRunning}
                          aria-label={(episode.starred ? 'Unstar ' : 'Star ') + episode.title}
                          leftIcon={<Star size={13} />}
                        >
                          {episode.starred ? 'Starred' : 'Star'}
                        </Button>
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={() =>
                            void patchEpisodeTriage(episode, { archived: !episode.archived })
                          }
                          disabled={triagingEpisodeId !== null || bulkRunning}
                          aria-label={
                            (episode.archived ? 'Unarchive ' : 'Archive ') + episode.title
                          }
                          leftIcon={
                            episode.archived ? <ArchiveRestore size={13} /> : <Archive size={13} />
                          }
                        >
                          {episode.archived ? 'Unarchive' : 'Archive'}
                        </Button>
                        {episode.hasAudio && (
                          <Button
                            variant="primary"
                            size="sm"
                            onClick={() => {
                              // The same persistent Player owns music, Plex and podcasts.
                              // The loaded show page supplies an episode queue without
                              // ever adding podcast items to the music catalog.
                              const queue = episodes
                                .filter(item => Boolean(item.hasAudio))
                                .map(item => podcastEpisodeToTrack(item, feed));
                              const selected = queue.findIndex(
                                item => item.podcastEpisodeId === episode.id,
                              );
                              if (selected < 0) return;
                              void useStore
                                .getState()
                                .playSong(queue[selected], queue, 'podcast', selected);
                            }}
                            aria-label={'Listen to ' + episode.title}
                            leftIcon={<Play size={13} />}
                          >
                            Listen
                          </Button>
                        )}
                        {episode.hasAudio && (
                          <>
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => queueEpisodeAction(episode, feed, 'next')}
                              aria-label={'Play ' + episode.title + ' next'}
                            >
                              Play Next
                            </Button>
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => queueEpisodeAction(episode, feed, 'end')}
                              aria-label={'Add ' + episode.title + ' to queue'}
                            >
                              Add to Queue
                            </Button>
                          </>
                        )}
                        {episode.hasAudio &&
                          (() => {
                            const status = downloads[episode.id];
                            const active =
                              status?.status === 'queued' ||
                              status?.status === 'downloading' ||
                              status?.status === 'paused';
                            const completed = status?.status === 'completed';
                            const label = active
                              ? 'Cancel download'
                              : status?.status === 'failed' || status?.status === 'canceled'
                              ? 'Retry download'
                              : 'Download episode';
                            const percent = status?.expectedBytes
                              ? Math.min(
                                  100,
                                  Math.floor((100 * status.bytesReceived) / status.expectedBytes),
                                )
                              : null;
                            return (
                              <div className="flex flex-wrap items-center gap-2">
                                {completed ? (
                                  <>
                                    <Button
                                      variant="secondary"
                                      size="sm"
                                      disabled={
                                        workingDownload !== null || selectedPodcastId === episode.id
                                      }
                                      onClick={() => setConfirmDeleteDownload(episode.id)}
                                      aria-label={'Remove offline copy of ' + episode.title}
                                    >
                                      Remove download
                                    </Button>
                                    {selectedPodcastId === episode.id && (
                                      <span className="text-xs text-text-subtle">
                                        Switch away before removing
                                      </span>
                                    )}
                                    {confirmDeleteDownload === episode.id && (
                                      <span
                                        role="group"
                                        aria-label={'Confirm remove download ' + episode.title}
                                        className="flex items-center gap-2 text-xs text-text-secondary"
                                      >
                                        <span>
                                          Remove offline audio? Listening history remains.
                                        </span>
                                        <Button
                                          variant="primary"
                                          size="sm"
                                          disabled={workingDownload !== null}
                                          onClick={() => void changeDownload(episode, 'remove')}
                                          aria-label={
                                            'Confirm remove offline copy of ' + episode.title
                                          }
                                        >
                                          Remove copy
                                        </Button>
                                        <Button
                                          variant="secondary"
                                          size="sm"
                                          onClick={() => setConfirmDeleteDownload(null)}
                                        >
                                          Keep
                                        </Button>
                                      </span>
                                    )}
                                  </>
                                ) : (
                                  <Button
                                    variant="secondary"
                                    size="sm"
                                    disabled={workingDownload !== null}
                                    onClick={() =>
                                      void changeDownload(episode, active ? 'cancel' : 'queue')
                                    }
                                    aria-label={label + ' ' + episode.title}
                                    leftIcon={<Download size={14} aria-hidden="true" />}
                                  >
                                    {workingDownload === episode.id ? 'Working…' : label}
                                  </Button>
                                )}
                                {status && (
                                  <span role="status" className="text-xs text-text-subtle">
                                    {completed
                                      ? 'Available offline'
                                      : status.status === 'downloading'
                                      ? `Downloading${percent === null ? '' : ' ' + percent + '%'}`
                                      : status.status === 'failed'
                                      ? `Download failed: ${status.errorCode || 'unavailable'}`
                                      : status.status === 'canceled'
                                      ? 'Download canceled'
                                      : status.status === 'paused' && status.retryAfter
                                      ? `Retry scheduled for ${new Date(
                                          status.retryAfter,
                                        ).toLocaleTimeString([], {
                                          hour: 'numeric',
                                          minute: '2-digit',
                                        })}`
                                      : 'Download queued'}
                                  </span>
                                )}
                              </div>
                            );
                          })()}
                        <span className="text-xs text-text-subtle">
                          Uses the shared ViiB music/Plex player and queue.
                        </span>
                      </div>
                    </article>
                  </Card>
                ))}
              </div>
              {nextOffset !== null && (
                <Button
                  variant="secondary"
                  className="mt-5 w-full"
                  disabled={loadingMore || bulkRunning}
                  onClick={() => void loadMoreEpisodes()}
                >
                  {loadingMore ? 'Loading more episodes…' : 'Load more episodes'}
                </Button>
              )}
            </section>
          </>
        )}
      </div>
    </Page>
  );
};
