import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useParams } from 'react-router';
import {
  Archive,
  ArchiveRestore,
  ArrowLeft,
  Bookmark,
  Check,
  Clock,
  Download,
  ExternalLink,
  FileText,
  Headphones,
  ListOrdered,
  ListPlus,
  Loader2,
  Pause,
  Play,
  RotateCcw,
  Search,
  Star,
  Trash2,
  X,
} from 'lucide-react';
import { Page, PageHeader } from '../components/ui/Page';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Chip } from '../components/ui/Chip';
import { TextInput } from '../components/ui/TextInput';
import { Skeleton } from '../components/Skeleton';
import { podcastEpisodeToTrack, queuePodcastEpisode, type PodcastQueuePosition } from '../lib/podcastPlayback';
import { useStore } from '../store';
import { formatTime } from '../utils';
import {
  PodcastAPIError,
  podcastsAPI,
  type PodcastBookmark,
  type PodcastChapter,
  type PodcastEpisodeDetail,
  type PodcastFeed,
  type PodcastTranscriptCue,
} from '../services/podcasts';

type EpisodeTab = 'notes' | 'chapters' | 'transcript' | 'bookmarks';

const MAX_BOOKMARK_NOTE_LENGTH = 500;
const INITIAL_TRANSCRIPT_RENDER_LIMIT = 500;

const BLOCKED_HTML_TAGS = new Set([
  'script',
  'style',
  'iframe',
  'object',
  'embed',
  'svg',
  'math',
  'template',
  'noscript',
  'form',
  'input',
  'textarea',
  'select',
  'button',
  'meta',
  'link',
  'base',
]);

function readableError(error: unknown): string {
  return error instanceof Error ? error.message : 'Unable to reach the podcast service.';
}

function isSafeExternalURL(raw?: string): string | null {
  if (!raw) return null;
  const trimmed = raw.trim();
  if (!trimmed) return null;
  try {
    const parsed = new URL(trimmed);
    if (parsed.protocol === 'https:' || parsed.protocol === 'http:') {
      return parsed.toString();
    }
  } catch {
    return null;
  }
  return null;
}

function isSafeArtworkURL(raw?: string): string | null {
  if (!raw) return null;
  const trimmed = raw.trim();
  if (!trimmed) return null;
  try {
    const parsed = new URL(trimmed);
    if (parsed.protocol === 'https:') {
      return parsed.toString();
    }
  } catch {
    return null;
  }
  return null;
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '';
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function parseTimestampToMs(raw: string): number | null {
  const trimmed = raw.trim();
  if (!trimmed) return null;
  if (/^\d+(?:\.\d+)?$/.test(trimmed)) {
    const secs = Number(trimmed);
    return Number.isFinite(secs) && secs >= 0 ? Math.round(secs * 1000) : null;
  }
  const parts = trimmed.split(':');
  if (parts.length < 2 || parts.length > 3) return null;
  let totalSeconds = 0;
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i].trim();
    if (!part) return null;
    if (i < parts.length - 1) {
      if (!/^\d+$/.test(part)) return null;
      const value = Number(part);
      if (!Number.isFinite(value) || value < 0) return null;
      totalSeconds = totalSeconds * 60 + value;
    } else {
      if (!/^\d+(?:\.\d+)?$/.test(part)) return null;
      const value = Number(part);
      if (!Number.isFinite(value) || value < 0 || (parts.length === 3 && value >= 60)) return null;
      totalSeconds = totalSeconds * 60 + value;
    }
  }
  return Number.isFinite(totalSeconds) && totalSeconds >= 0 ? Math.round(totalSeconds * 1000) : null;
}

function buildFallbackFeed(detail: PodcastEpisodeDetail): PodcastFeed {
  return {
    id: detail.feedId,
    url: '',
    title: detail.feedTitle,
    author: detail.feedAuthor,
    description: '',
    imageUrl: `/api/v2/podcasts/feeds/${encodeURIComponent(detail.feedId)}/artwork`,
    language: '',
    episodeCount: 0,
    defaultSpeed: detail.defaultSpeed,
    skipIntroSeconds: detail.skipIntroSeconds,
    skipOutroSeconds: detail.skipOutroSeconds,
  };
}

function renderSanitizedNode(node: Node, keyPrefix: string): React.ReactNode {
  if (node.nodeType === 3) {
    return node.textContent;
  }
  if (node.nodeType !== 1) return null;
  const element = node as Element;
  const tag = element.tagName.toLowerCase();
  if (BLOCKED_HTML_TAGS.has(tag)) return null;

  const children = Array.from(element.childNodes).map((child, idx) =>
    renderSanitizedNode(child, `${keyPrefix}-${idx}`)
  );

  switch (tag) {
    case 'p':
      return (
        <p key={keyPrefix} className="mb-3 leading-relaxed text-text-secondary last:mb-0">
          {children}
        </p>
      );
    case 'br':
      return <br key={keyPrefix} />;
    case 'strong':
    case 'b':
      return (
        <strong key={keyPrefix} className="font-semibold text-text-main">
          {children}
        </strong>
      );
    case 'em':
    case 'i':
      return (
        <em key={keyPrefix} className="italic">
          {children}
        </em>
      );
    case 'ul':
      return (
        <ul key={keyPrefix} className="mb-3 list-disc space-y-1 pl-5 text-text-secondary last:mb-0">
          {children}
        </ul>
      );
    case 'ol':
      return (
        <ol key={keyPrefix} className="mb-3 list-decimal space-y-1 pl-5 text-text-secondary last:mb-0">
          {children}
        </ol>
      );
    case 'li':
      return <li key={keyPrefix}>{children}</li>;
    case 'blockquote':
      return (
        <blockquote
          key={keyPrefix}
          className="mb-3 border-l-2 border-brand/50 pl-3 italic text-text-secondary last:mb-0"
        >
          {children}
        </blockquote>
      );
    case 'code':
      return (
        <code key={keyPrefix} className="rounded bg-surface-3 px-1.5 py-0.5 font-mono text-xs text-text-main">
          {children}
        </code>
      );
    case 'pre':
      return (
        <pre
          key={keyPrefix}
          className="mb-3 overflow-x-auto rounded-lg bg-surface-1 p-3 font-mono text-xs text-text-main last:mb-0"
        >
          {children}
        </pre>
      );
    case 'h1':
    case 'h2':
    case 'h3':
    case 'h4':
    case 'h5':
    case 'h6':
      return (
        <h3 key={keyPrefix} className="mb-2 mt-4 text-base font-semibold text-text-main first:mt-0">
          {children}
        </h3>
      );
    case 'hr':
      return <hr key={keyPrefix} className="my-4 border-surface-border" />;
    case 'a': {
      const safeHref = isSafeExternalURL(element.getAttribute('href') ?? undefined);
      if (!safeHref) {
        return <span key={keyPrefix}>{children}</span>;
      }
      return (
        <a
          key={keyPrefix}
          href={safeHref}
          target="_blank"
          rel="noopener noreferrer"
          className="text-brand underline underline-offset-2 hover:text-brand-hover"
        >
          {children}
        </a>
      );
    }
    default:
      return <React.Fragment key={keyPrefix}>{children}</React.Fragment>;
  }
}

function renderSafeShowNotes(rawHtml: string): React.ReactNode {
  const trimmed = rawHtml.trim();
  if (!trimmed) return null;

  const hasMarkup = /<[a-z!/]/i.test(trimmed);
  if (!hasMarkup) {
    return (
      <div className="whitespace-pre-line leading-relaxed text-text-secondary">
        {trimmed}
      </div>
    );
  }

  if (typeof DOMParser !== 'undefined') {
    const doc = new DOMParser().parseFromString(trimmed, 'text/html');
    const nodes = Array.from(doc.body.childNodes).map((node, index) =>
      renderSanitizedNode(node, `note-${index}`)
    );
    return <div className="space-y-1 text-sm">{nodes}</div>;
  }

  const stripped = trimmed.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
  return <p className="leading-relaxed text-text-secondary">{stripped}</p>;
}

function downloadStatusBadge(status?: string): { label: string; className: string } | null {
  switch (status) {
    case 'completed':
      return { label: 'Available offline', className: 'bg-success/15 text-success border border-success/30' };
    case 'downloading':
      return { label: 'Downloading', className: 'bg-brand/15 text-brand border border-brand/30' };
    case 'queued':
      return { label: 'Download queued', className: 'bg-surface-3 text-text-secondary border border-surface-border' };
    case 'paused':
      return { label: 'Download paused', className: 'bg-warning/15 text-warning border border-warning/30' };
    case 'failed':
      return { label: 'Download failed', className: 'bg-error/15 text-error border border-error/30' };
    case 'canceled':
      return { label: 'Download canceled', className: 'bg-surface-3 text-text-subtle border border-surface-border' };
    default:
      return null;
  }
}

const PodcastEpisodeSkeleton: React.FC = () => (
  <Page aria-busy="true" aria-label="Loading podcast episode">
    <div className="mb-4 flex items-center gap-3">
      <Skeleton className="h-4 w-32" />
      <Skeleton className="h-4 w-24" />
    </div>
    <Card variant="hero" className="mb-6">
      <div className="flex flex-col gap-6 sm:flex-row sm:items-start">
        <Skeleton className="h-32 w-32 shrink-0 rounded-xl sm:h-40 sm:w-40" />
        <div className="min-w-0 flex-1 space-y-3">
          <Skeleton className="h-4 w-40" />
          <Skeleton className="h-8 w-3/4" />
          <div className="flex flex-wrap gap-2">
            <Skeleton className="h-6 w-20 rounded-full" />
            <Skeleton className="h-6 w-24 rounded-full" />
            <Skeleton className="h-6 w-16 rounded-full" />
          </div>
          <div className="flex flex-wrap gap-2 pt-2">
            <Skeleton className="h-9 w-28 rounded-full" />
            <Skeleton className="h-9 w-28 rounded-full" />
            <Skeleton className="h-9 w-28 rounded-full" />
            <Skeleton className="h-9 w-24 rounded-full" />
          </div>
        </div>
      </div>
    </Card>
    <div className="mb-6 flex gap-2 border-b border-surface-border pb-4">
      <Skeleton className="h-8 w-28 rounded-full" />
      <Skeleton className="h-8 w-24 rounded-full" />
      <Skeleton className="h-8 w-24 rounded-full" />
      <Skeleton className="h-8 w-28 rounded-full" />
    </div>
    <Card>
      <div className="space-y-3">
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-11/12" />
        <Skeleton className="h-4 w-4/5" />
      </div>
    </Card>
  </Page>
);

export const PodcastEpisodePage: React.FC = () => {
  const { id } = useParams<{ id: string }>();

  const currentSong = useStore(state => state.currentSong);
  const isPlaying = useStore(state => state.isPlaying);

  const [detail, setDetail] = useState<PodcastEpisodeDetail | null>(null);
  const [feed, setFeed] = useState<PodcastFeed | null>(null);
  const [loading, setLoading] = useState(true);
  const [notFound, setNotFound] = useState(false);
  const [error, setError] = useState('');
  const [reloadNonce, setReloadNonce] = useState(0);

  const [artworkFailed, setArtworkFailed] = useState(false);
  const [brokenChapterImages, setBrokenChapterImages] = useState<Record<number, boolean>>({});
  const [actionError, setActionError] = useState('');
  const [stateWorking, setStateWorking] = useState(false);
  const [downloadWorking, setDownloadWorking] = useState(false);

  const [activeTab, setActiveTab] = useState<EpisodeTab>('notes');

  const [chapters, setChapters] = useState<PodcastChapter[]>([]);
  const [chaptersLoading, setChaptersLoading] = useState(false);
  const [chaptersError, setChaptersError] = useState('');
  const [chaptersRetryNonce, setChaptersRetryNonce] = useState(0);
  const chaptersControllerRef = useRef<AbortController | null>(null);
  const chaptersKeyRef = useRef<string | null>(null);

  const [cues, setCues] = useState<PodcastTranscriptCue[]>([]);
  const [transcriptLoading, setTranscriptLoading] = useState(false);
  const [transcriptError, setTranscriptError] = useState('');
  const [transcriptRetryNonce, setTranscriptRetryNonce] = useState(0);
  const transcriptControllerRef = useRef<AbortController | null>(null);
  const transcriptKeyRef = useRef<string | null>(null);
  const [transcriptQuery, setTranscriptQuery] = useState('');
  const [showAllCues, setShowAllCues] = useState(false);

  const [bookmarks, setBookmarks] = useState<PodcastBookmark[]>([]);
  const [bookmarksLoading, setBookmarksLoading] = useState(false);
  const [bookmarksError, setBookmarksError] = useState('');
  const [bookmarkTimeInput, setBookmarkTimeInput] = useState('0:00');
  const [bookmarkTimeDirty, setBookmarkTimeDirty] = useState(false);
  const [bookmarkNote, setBookmarkNote] = useState('');
  const [bookmarkFormError, setBookmarkFormError] = useState('');
  const [bookmarkSaving, setBookmarkSaving] = useState(false);
  const [deletingBookmarkId, setDeletingBookmarkId] = useState<string | null>(null);

  const [liveTimeSeconds, setLiveTimeSeconds] = useState<number | null>(null);

  const isCurrentEpisode = Boolean(detail && currentSong?.podcastEpisodeId === detail.id);

  // Reset per-episode state cleanly when route param changes.
  useEffect(() => {
    setDetail(null);
    setFeed(null);
    setLoading(true);
    setNotFound(false);
    setError('');
    setArtworkFailed(false);
    setBrokenChapterImages({});
    setActionError('');
    setActiveTab('notes');
    setChapters([]);
    setChaptersLoading(false);
    setChaptersError('');
    setChaptersRetryNonce(0);
    chaptersControllerRef.current?.abort();
    chaptersControllerRef.current = null;
    chaptersKeyRef.current = null;
    setCues([]);
    setTranscriptLoading(false);
    setTranscriptError('');
    setTranscriptRetryNonce(0);
    transcriptControllerRef.current?.abort();
    transcriptControllerRef.current = null;
    transcriptKeyRef.current = null;
    setTranscriptQuery('');
    setShowAllCues(false);
    setBookmarks([]);
    setBookmarksLoading(false);
    setBookmarksError('');
    setBookmarkTimeInput('0:00');
    setBookmarkTimeDirty(false);
    setBookmarkNote('');
    setBookmarkFormError('');
    setLiveTimeSeconds(null);
  }, [id]);

  // Primary episode detail, feed metadata, and bookmarks fetch.
  useEffect(() => {
    if (!id) {
      setLoading(false);
      setNotFound(true);
      setError('Episode ID is missing.');
      return;
    }

    const controller = new AbortController();
    setLoading(true);
    setNotFound(false);
    setError('');

    void podcastsAPI
      .getEpisodeDetail(id, controller.signal)
      .then(async ({ episode }) => {
        if (controller.signal.aborted) return;
        setDetail(episode);
        setBookmarksLoading(true);
        setBookmarksError('');

        const [feedResult, bookmarksResult] = await Promise.allSettled([
          podcastsAPI.getFeed(episode.feedId, controller.signal),
          podcastsAPI.listBookmarks(episode.id, controller.signal),
        ]);
        if (controller.signal.aborted) return;

        if (feedResult.status === 'fulfilled') {
          setFeed(feedResult.value.feed);
        } else {
          setFeed(buildFallbackFeed(episode));
        }

        if (bookmarksResult.status === 'fulfilled') {
          setBookmarks(bookmarksResult.value.bookmarks);
        } else {
          setBookmarksError(readableError(bookmarksResult.reason));
        }
        setBookmarksLoading(false);
      })
      .catch(err => {
        if (controller.signal.aborted) return;
        if (err instanceof PodcastAPIError && err.status === 404) {
          setNotFound(true);
        }
        setError(readableError(err));
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      });

    return () => controller.abort();
  }, [id, reloadNonce]);

  // Abort chapters fetch when switching episodes, retrying, or unmounting.
  useEffect(() => {
    return () => {
      chaptersControllerRef.current?.abort();
      chaptersControllerRef.current = null;
      chaptersKeyRef.current = null;
    };
  }, [detail?.id, chaptersRetryNonce]);

  // Lazily fetch chapters when detail.hasChapters is true or Chapters tab is opened.
  useEffect(() => {
    if (!detail) return;
    if (!detail.hasChapters && activeTab !== 'chapters') return;
    const key = `${detail.id}:${chaptersRetryNonce}`;
    if (chaptersKeyRef.current === key) return;
    chaptersKeyRef.current = key;

    chaptersControllerRef.current?.abort();
    const controller = new AbortController();
    chaptersControllerRef.current = controller;
    setChaptersLoading(true);
    setChaptersError('');

    void podcastsAPI
      .getEpisodeChapters(detail.id, controller.signal)
      .then(({ chapters: loaded }) => {
        if (controller.signal.aborted) return;
        setChapters(loaded);
      })
      .catch(err => {
        if (controller.signal.aborted) return;
        setChaptersError(readableError(err));
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setChaptersLoading(false);
        }
      });
  }, [detail, activeTab, chaptersRetryNonce]);

  // Abort transcript fetch when switching episodes, retrying, or unmounting.
  useEffect(() => {
    return () => {
      transcriptControllerRef.current?.abort();
      transcriptControllerRef.current = null;
      transcriptKeyRef.current = null;
    };
  }, [detail?.id, transcriptRetryNonce]);

  // Lazily fetch transcript when detail.hasTranscript is true or Transcript tab is opened.
  useEffect(() => {
    if (!detail) return;
    if (!detail.hasTranscript && activeTab !== 'transcript') return;
    const key = `${detail.id}:${transcriptRetryNonce}`;
    if (transcriptKeyRef.current === key) return;
    transcriptKeyRef.current = key;

    transcriptControllerRef.current?.abort();
    const controller = new AbortController();
    transcriptControllerRef.current = controller;
    setTranscriptLoading(true);
    setTranscriptError('');

    void podcastsAPI
      .getEpisodeTranscript(detail.id, controller.signal)
      .then(({ cues: loaded }) => {
        if (controller.signal.aborted) return;
        setCues(loaded);
      })
      .catch(err => {
        if (controller.signal.aborted) return;
        setTranscriptError(readableError(err));
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setTranscriptLoading(false);
        }
      });
  }, [detail, activeTab, transcriptRetryNonce]);

  // Observe live HTMLAudioElement time when this episode is loaded in the shared player.
  useEffect(() => {
    if (!detail || !isCurrentEpisode) {
      setLiveTimeSeconds(null);
      return;
    }

    const mediaUrl = podcastsAPI.mediaURL(detail.id);
    const readActiveTime = (): number | null => {
      if (typeof document === 'undefined') return null;
      const elements = Array.from(document.querySelectorAll('audio'));
      const matching = elements.filter(el => {
        const src = el.currentSrc || el.getAttribute('src') || '';
        return src.includes(mediaUrl);
      });
      if (matching.length === 0) return null;
      const playingEl = matching.find(el => !el.paused) ?? matching[0];
      return Number.isFinite(playingEl.currentTime) ? playingEl.currentTime : null;
    };

    const sync = () => {
      const next = readActiveTime();
      if (next !== null) {
        setLiveTimeSeconds(next);
      }
    };

    sync();
    const onMediaTick = () => sync();
    document.addEventListener('timeupdate', onMediaTick, true);
    document.addEventListener('seeked', onMediaTick, true);
    const timer = isPlaying ? window.setInterval(sync, 500) : null;

    return () => {
      document.removeEventListener('timeupdate', onMediaTick, true);
      document.removeEventListener('seeked', onMediaTick, true);
      if (timer !== null) window.clearInterval(timer);
    };
  }, [detail, isCurrentEpisode, isPlaying]);

  // Keep bookmark timestamp defaulted to live player time (when playing this episode)
  // or saved detail.positionMs while the user has not edited the field.
  useEffect(() => {
    if (!detail || bookmarkTimeDirty) return;
    const defaultSeconds =
      isCurrentEpisode && liveTimeSeconds !== null
        ? liveTimeSeconds
        : Math.max(0, (detail.positionMs ?? 0) / 1000);
    setBookmarkTimeInput(formatTime(defaultSeconds));
  }, [detail, isCurrentEpisode, liveTimeSeconds, bookmarkTimeDirty]);

  const resolvedFeed = useMemo(
    () => (detail ? feed ?? buildFallbackFeed(detail) : null),
    [detail, feed]
  );

  const seekToSeconds = useCallback(
    (seconds: number) => {
      if (!detail || !resolvedFeed) return;
      const clampedSeconds = Math.max(0, seconds);
      if (currentSong?.podcastEpisodeId === detail.id) {
        window.dispatchEvent(
          new CustomEvent('viib:podcast-seek', {
            detail: { episodeId: detail.id, seconds: clampedSeconds },
          })
        );
        setLiveTimeSeconds(clampedSeconds);
        return;
      }
      if (!detail.hasAudio) return;
      const track = {
        ...podcastEpisodeToTrack(detail, resolvedFeed),
        podcastResumeMs: Math.round(clampedSeconds * 1000),
      };
      void useStore.getState().playSong(track, [track], 'podcast', 0);
    },
    [detail, resolvedFeed, currentSong?.podcastEpisodeId]
  );

  const handleListenOrResume = () => {
    if (!detail || !resolvedFeed || !detail.hasAudio) return;
    setActionError('');
    if (isCurrentEpisode) {
      useStore.getState().togglePlay();
      return;
    }
    const track = podcastEpisodeToTrack(detail, resolvedFeed);
    void useStore.getState().playSong(track, [track], 'podcast', 0);
  };

  const handleQueue = async (position: PodcastQueuePosition) => {
    if (!detail || !resolvedFeed || !detail.hasAudio) return;
    setActionError('');
    const store = useStore.getState();
    queuePodcastEpisode(detail, resolvedFeed, position, store);
    try {
      await podcastsAPI.addToQueue(detail.id, position);
      store.showToast({
        type: 'success',
        message:
          position === 'next'
            ? 'Episode added to Play Next.'
            : 'Episode added to the queue.',
        duration: 3000,
      });
    } catch (err) {
      setActionError(readableError(err));
    }
  };

  const handleToggleStar = async () => {
    if (!detail || stateWorking) return;
    setStateWorking(true);
    setActionError('');
    try {
      const { state } = await podcastsAPI.patchEpisodeState(detail.id, {
        starred: !detail.starred,
      });
      setDetail(prev =>
        prev
          ? {
              ...prev,
              starred: state.starred,
              archived: state.archived,
              isPlayed: state.isPlayed,
              positionMs: state.positionMs,
              listenedMs: state.listenedMs,
              progressRevision: state.revision,
            }
          : prev
      );
    } catch (err) {
      setActionError(readableError(err));
    } finally {
      setStateWorking(false);
    }
  };

  const handleToggleArchive = async () => {
    if (!detail || stateWorking) return;
    setStateWorking(true);
    setActionError('');
    try {
      const { state } = await podcastsAPI.patchEpisodeState(detail.id, {
        archived: !detail.archived,
      });
      setDetail(prev =>
        prev
          ? {
              ...prev,
              starred: state.starred,
              archived: state.archived,
              isPlayed: state.isPlayed,
              positionMs: state.positionMs,
              listenedMs: state.listenedMs,
              progressRevision: state.revision,
            }
          : prev
      );
    } catch (err) {
      setActionError(readableError(err));
    } finally {
      setStateWorking(false);
    }
  };

  const handleTogglePlayed = async () => {
    if (!detail || stateWorking) return;
    setStateWorking(true);
    setActionError('');
    try {
      const { state } = await podcastsAPI.patchEpisodeState(detail.id, {
        isPlayed: !detail.isPlayed,
        expectedRevision: detail.progressRevision ?? 0,
      });
      setDetail(prev =>
        prev
          ? {
              ...prev,
              starred: state.starred,
              archived: state.archived,
              isPlayed: state.isPlayed,
              positionMs: state.positionMs,
              listenedMs: state.listenedMs,
              progressRevision: state.revision,
            }
          : prev
      );
    } catch (err) {
      setActionError(readableError(err));
    } finally {
      setStateWorking(false);
    }
  };

  const handleDownloadAction = async () => {
    if (!detail || downloadWorking || !detail.hasAudio) return;
    const status = detail.downloadStatus ?? '';
    const isActive = status === 'queued' || status === 'downloading' || status === 'paused';
    const isCompleted = status === 'completed';

    if (isCompleted && isCurrentEpisode) {
      setActionError('Switch away from this playing episode before removing its offline copy.');
      return;
    }

    setDownloadWorking(true);
    setActionError('');
    try {
      if (isCompleted) {
        await podcastsAPI.removeDownload(detail.id);
        setDetail(prev => (prev ? { ...prev, downloadStatus: '' } : prev));
      } else if (isActive) {
        const { download } = await podcastsAPI.cancelDownload(detail.id);
        setDetail(prev => (prev ? { ...prev, downloadStatus: download.status } : prev));
      } else {
        const { download } = await podcastsAPI.queueDownload(detail.id);
        setDetail(prev => (prev ? { ...prev, downloadStatus: download.status } : prev));
      }
    } catch (err) {
      setActionError(readableError(err));
    } finally {
      setDownloadWorking(false);
    }
  };

  const handleCreateBookmark = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!detail || bookmarkSaving) return;
    const positionMs = parseTimestampToMs(bookmarkTimeInput);
    if (positionMs === null) {
      setBookmarkFormError('Enter a valid timestamp (MM:SS, HH:MM:SS, or seconds).');
      return;
    }
    const trimmedNote = bookmarkNote.trim();
    if (trimmedNote.length > MAX_BOOKMARK_NOTE_LENGTH) {
      setBookmarkFormError(`Bookmark note must be ${MAX_BOOKMARK_NOTE_LENGTH} characters or fewer.`);
      return;
    }

    setBookmarkSaving(true);
    setBookmarkFormError('');
    try {
      const { bookmark } = await podcastsAPI.createBookmark(detail.id, positionMs, trimmedNote);
      setBookmarks(prev =>
        [...prev, bookmark].sort((a, b) => a.positionMs - b.positionMs || a.createdAt - b.createdAt)
      );
      setDetail(prev =>
        prev ? { ...prev, bookmarkCount: (prev.bookmarkCount ?? 0) + 1 } : prev
      );
      setBookmarkNote('');
      setBookmarkTimeDirty(false);
      useStore.getState().showToast({
        type: 'success',
        message: 'Bookmark saved.',
        duration: 2500,
      });
    } catch (err) {
      setBookmarkFormError(readableError(err));
    } finally {
      setBookmarkSaving(false);
    }
  };

  const handleDeleteBookmark = async (bookmark: PodcastBookmark) => {
    if (!detail || deletingBookmarkId) return;
    setDeletingBookmarkId(bookmark.id);
    setBookmarksError('');
    try {
      await podcastsAPI.deleteBookmark(detail.id, bookmark.id);
      setBookmarks(prev => prev.filter(item => item.id !== bookmark.id));
      setDetail(prev =>
        prev ? { ...prev, bookmarkCount: Math.max(0, (prev.bookmarkCount ?? 1) - 1) } : prev
      );
    } catch (err) {
      setBookmarksError(readableError(err));
    } finally {
      setDeletingBookmarkId(null);
    }
  };

  const activePlaybackSeconds =
    isCurrentEpisode && liveTimeSeconds !== null
      ? liveTimeSeconds
      : detail
      ? (detail.positionMs ?? 0) / 1000
      : 0;

  const activeChapterIndex = useMemo(() => {
    if (!isCurrentEpisode || chapters.length === 0) return -1;
    const t = liveTimeSeconds ?? (detail?.positionMs ?? 0) / 1000;
    let found = -1;
    for (let i = 0; i < chapters.length; i++) {
      const ch = chapters[i];
      const next = chapters[i + 1];
      const end = ch.endTime ?? next?.startTime ?? Number.POSITIVE_INFINITY;
      if (t >= ch.startTime && t < end) {
        found = i;
      }
    }
    return found;
  }, [chapters, detail?.positionMs, isCurrentEpisode, liveTimeSeconds]);

  const filteredCues = useMemo(() => {
    const query = transcriptQuery.trim().toLowerCase();
    if (!query) return cues;
    return cues.filter(
      cue =>
        cue.text.toLowerCase().includes(query) ||
        (cue.speaker ? cue.speaker.toLowerCase().includes(query) : false)
    );
  }, [cues, transcriptQuery]);

  const displayedCues = useMemo(
    () =>
      showAllCues || filteredCues.length <= INITIAL_TRANSCRIPT_RENDER_LIMIT
        ? filteredCues
        : filteredCues.slice(0, INITIAL_TRANSCRIPT_RENDER_LIMIT),
    [filteredCues, showAllCues]
  );

  if (loading) {
    return <PodcastEpisodeSkeleton />;
  }

  if (notFound || !detail) {
    return (
      <Page>
        <nav aria-label="Podcast breadcrumbs" className="mb-4 flex items-center gap-3 text-sm">
          <Link
            to="/podcasts"
            className="inline-flex items-center gap-1.5 text-text-secondary transition-colors hover:text-text-main"
          >
            <ArrowLeft size={16} aria-hidden="true" />
            Podcasts Hub
          </Link>
        </nav>
        <Card className="max-w-xl p-6">
          <PageHeader
            heading={notFound ? 'Episode not found' : 'Unable to load episode'}
            subtitle={
              error ||
              (notFound
                ? 'This podcast episode may have been removed or belongs to an unfollowed show.'
                : 'The local podcast service could not load this episode.')
            }
            className="mb-4"
            titleClassName="text-section"
          />
          <div className="flex flex-wrap items-center gap-3">
            <Link
              to="/podcasts"
              className="inline-flex items-center gap-2 rounded-full bg-brand px-4 py-2 text-xs font-bold text-surface-0 transition-all hover:brightness-105"
            >
              Back to Podcasts
            </Link>
            {!notFound && id && (
              <Button
                variant="secondary"
                size="sm"
                leftIcon={<RotateCcw size={14} aria-hidden="true" />}
                onClick={() => setReloadNonce(n => n + 1)}
                aria-label="Retry loading episode"
              >
                Retry
              </Button>
            )}
          </div>
        </Card>
      </Page>
    );
  }

  const showPath = `/podcasts/show/${encodeURIComponent(detail.feedId)}`;
  const artworkSrc = `/api/v2/podcasts/feeds/${encodeURIComponent(detail.feedId)}/artwork`;
  const downloadBadge = downloadStatusBadge(detail.downloadStatus);
  const dlStatus = detail.downloadStatus ?? '';
  const isDlActive = dlStatus === 'queued' || dlStatus === 'downloading' || dlStatus === 'paused';
  const isDlCompleted = dlStatus === 'completed';
  const downloadButtonLabel = isDlCompleted
    ? 'Remove Offline'
    : isDlActive
    ? 'Cancel Download'
    : dlStatus === 'failed' || dlStatus === 'canceled'
    ? 'Retry Download'
    : 'Download';

  const hasSavedResume = (detail.positionMs ?? 0) > 0 && !detail.isPlayed;
  const listenButtonLabel = isCurrentEpisode
    ? isPlaying
      ? 'Pause'
      : 'Resume'
    : hasSavedResume
    ? 'Resume'
    : 'Listen';

  const safeWebsiteUrl = isSafeExternalURL(detail.websiteUrl);
  const showNotesSource = detail.richDescription || detail.description;

  return (
    <Page className="mx-auto max-w-5xl">
      <nav aria-label="Podcast breadcrumbs" className="mb-4 flex flex-wrap items-center gap-4 text-sm">
        <Link
          to={showPath}
          className="inline-flex items-center gap-1.5 font-medium text-text-secondary transition-colors hover:text-text-main"
          aria-label={`Back to ${detail.feedTitle || 'show'}`}
        >
          <ArrowLeft size={16} aria-hidden="true" />
          Back to {detail.feedTitle || 'Show'}
        </Link>
        <span className="text-text-subtle" aria-hidden="true">
          /
        </span>
        <Link
          to="/podcasts"
          className="text-text-secondary transition-colors hover:text-text-main"
          aria-label="Podcasts Hub"
        >
          Podcasts Hub
        </Link>
      </nav>

      <Card variant="hero" className="mb-6">
        <div className="flex flex-col gap-6 sm:flex-row sm:items-start">
          <div className="relative flex h-32 w-32 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-surface-highlight text-brand sm:h-40 sm:w-40">
            <Headphones size={48} aria-hidden="true" />
            {!artworkFailed && (
              <img
                src={artworkSrc}
                alt={`${detail.feedTitle || detail.title} cover art`}
                referrerPolicy="no-referrer"
                onError={() => setArtworkFailed(true)}
                className="absolute inset-0 h-full w-full object-cover"
              />
            )}
          </div>

          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
              <Link
                to={showPath}
                className="font-semibold text-brand transition-colors hover:text-brand-hover"
              >
                {detail.feedTitle || 'Podcast show'}
              </Link>
              {detail.feedAuthor && (
                <span className="text-text-secondary">· {detail.feedAuthor}</span>
              )}
            </div>

            <PageHeader
              heading={detail.title}
              className="mb-3 mt-1.5"
              titleClassName="text-2xl sm:text-3xl font-bold break-words"
            />

            <div className="mb-4 flex flex-wrap items-center gap-2 text-xs text-text-secondary">
              {detail.seasonNumber !== undefined && detail.seasonNumber > 0 && (
                <Chip tabIndex={-1} className="cursor-default px-2.5 py-1 text-xs">
                  Season {detail.seasonNumber}
                </Chip>
              )}
              {detail.episodeNumber !== undefined && detail.episodeNumber > 0 && (
                <Chip tabIndex={-1} className="cursor-default px-2.5 py-1 text-xs">
                  Episode {detail.episodeNumber}
                </Chip>
              )}
              {detail.publishedAt > 0 && (
                <span className="inline-flex items-center gap-1">
                  {new Date(detail.publishedAt).toLocaleDateString(undefined, {
                    year: 'numeric',
                    month: 'short',
                    day: 'numeric',
                  })}
                </span>
              )}
              {detail.durationMs > 0 && (
                <span className="inline-flex items-center gap-1 font-mono">
                  <Clock size={12} aria-hidden="true" />
                  {formatTime(detail.durationMs / 1000)}
                </span>
              )}
              {(detail.enclosureBytes ?? 0) > 0 && (
                <span>{formatBytes(detail.enclosureBytes!)}</span>
              )}
              {hasSavedResume && (
                <span className="rounded-full bg-surface-3 px-2.5 py-0.5 font-medium text-text-main">
                  Resume at {formatTime((detail.positionMs ?? 0) / 1000)}
                </span>
              )}
              {detail.isPlayed && (
                <span className="rounded-full bg-success/15 px-2.5 py-0.5 font-medium text-success">
                  Played
                </span>
              )}
              {detail.starred && (
                <span className="rounded-full bg-warning/15 px-2.5 py-0.5 font-medium text-warning">
                  Starred
                </span>
              )}
              {detail.archived && (
                <span className="rounded-full bg-surface-3 px-2.5 py-0.5 font-medium text-text-subtle">
                  Archived
                </span>
              )}
              {downloadBadge && (
                <span
                  role="status"
                  className={`rounded-full px-2.5 py-0.5 font-medium ${downloadBadge.className}`}
                >
                  {downloadBadge.label}
                </span>
              )}
            </div>

            <div className="flex flex-wrap items-center gap-2">
              <Button
                variant="primary"
                size="md"
                disabled={!detail.hasAudio}
                onClick={handleListenOrResume}
                aria-label={`${listenButtonLabel} ${detail.title}`}
                leftIcon={
                  isCurrentEpisode && isPlaying ? (
                    <Pause size={16} aria-hidden="true" />
                  ) : (
                    <Play size={16} aria-hidden="true" />
                  )
                }
              >
                {listenButtonLabel}
              </Button>

              <Button
                variant="secondary"
                size="sm"
                disabled={!detail.hasAudio}
                onClick={() => void handleQueue('next')}
                aria-label={`Play ${detail.title} next`}
                leftIcon={<ListPlus size={15} aria-hidden="true" />}
              >
                Play Next
              </Button>

              <Button
                variant="secondary"
                size="sm"
                disabled={!detail.hasAudio}
                onClick={() => void handleQueue('end')}
                aria-label={`Add ${detail.title} to queue`}
                leftIcon={<ListOrdered size={15} aria-hidden="true" />}
              >
                Add to Queue
              </Button>

              <Button
                variant="secondary"
                size="sm"
                disabled={stateWorking}
                onClick={() => void handleToggleStar()}
                aria-pressed={Boolean(detail.starred)}
                aria-label={detail.starred ? `Unstar ${detail.title}` : `Star ${detail.title}`}
                leftIcon={
                  <Star
                    size={15}
                    className={detail.starred ? 'fill-current text-warning' : undefined}
                    aria-hidden="true"
                  />
                }
              >
                {detail.starred ? 'Unstar' : 'Star'}
              </Button>

              <Button
                variant="secondary"
                size="sm"
                disabled={stateWorking}
                onClick={() => void handleToggleArchive()}
                aria-pressed={Boolean(detail.archived)}
                aria-label={detail.archived ? `Unarchive ${detail.title}` : `Archive ${detail.title}`}
                leftIcon={
                  detail.archived ? (
                    <ArchiveRestore size={15} aria-hidden="true" />
                  ) : (
                    <Archive size={15} aria-hidden="true" />
                  )
                }
              >
                {detail.archived ? 'Unarchive' : 'Archive'}
              </Button>

              <Button
                variant="secondary"
                size="sm"
                disabled={stateWorking}
                onClick={() => void handleTogglePlayed()}
                aria-pressed={Boolean(detail.isPlayed)}
                aria-label={
                  detail.isPlayed
                    ? `Mark ${detail.title} unplayed`
                    : `Mark ${detail.title} played`
                }
                leftIcon={<Check size={15} aria-hidden="true" />}
              >
                {detail.isPlayed ? 'Mark Unplayed' : 'Mark Played'}
              </Button>

              {detail.hasAudio && (
                <Button
                  variant="secondary"
                  size="sm"
                  disabled={downloadWorking || (isDlCompleted && isCurrentEpisode)}
                  onClick={() => void handleDownloadAction()}
                  aria-label={`${downloadButtonLabel} ${detail.title}`}
                  leftIcon={
                    downloadWorking ? (
                      <Loader2 size={15} className="animate-spin" aria-hidden="true" />
                    ) : isDlCompleted ? (
                      <Trash2 size={15} aria-hidden="true" />
                    ) : isDlActive ? (
                      <X size={15} aria-hidden="true" />
                    ) : (
                      <Download size={15} aria-hidden="true" />
                    )
                  }
                >
                  {downloadButtonLabel}
                </Button>
              )}
            </div>

            {!detail.hasAudio && (
              <p className="mt-3 text-xs text-text-subtle">
                This episode does not include playable audio.
              </p>
            )}
            {actionError && (
              <p role="alert" className="mt-3 rounded-lg border border-error/30 bg-surface-1 p-3 text-xs text-error">
                {actionError}
              </p>
            )}
          </div>
        </div>
      </Card>

      <div
        role="tablist"
        aria-label="Episode details"
        className="mb-6 flex flex-nowrap items-center gap-1.5 overflow-x-auto border-b border-surface-border pb-4 no-scrollbar"
      >
        <button
          type="button"
          role="tab"
          id="tab-notes"
          aria-selected={activeTab === 'notes'}
          aria-controls="panel-notes"
          onClick={() => setActiveTab('notes')}
          className={`shrink-0 flex items-center gap-2 px-3 py-1.5 rounded-full text-xs font-semibold transition-colors whitespace-nowrap ${
            activeTab === 'notes'
              ? 'bg-text-main text-surface-0 font-bold'
              : 'bg-surface-2 text-text-secondary hover:text-text-main hover:bg-surface-hover'
          }`}
        >
          <FileText size={14} aria-hidden="true" />
          Show Notes
        </button>

        <button
          type="button"
          role="tab"
          id="tab-chapters"
          aria-selected={activeTab === 'chapters'}
          aria-controls="panel-chapters"
          onClick={() => setActiveTab('chapters')}
          className={`shrink-0 flex items-center gap-2 px-3 py-1.5 rounded-full text-xs font-semibold transition-colors whitespace-nowrap ${
            activeTab === 'chapters'
              ? 'bg-text-main text-surface-0 font-bold'
              : 'bg-surface-2 text-text-secondary hover:text-text-main hover:bg-surface-hover'
          }`}
        >
          <ListOrdered size={14} aria-hidden="true" />
          Chapters
          <span
            className={`rounded-full px-1.5 py-0.5 text-[11px] leading-none ${
              activeTab === 'chapters'
                ? 'bg-surface-0/15 text-surface-0'
                : 'bg-surface-3 text-text-secondary'
            }`}
          >
            {chapters.length}
          </span>
        </button>

        <button
          type="button"
          role="tab"
          id="tab-transcript"
          aria-selected={activeTab === 'transcript'}
          aria-controls="panel-transcript"
          onClick={() => setActiveTab('transcript')}
          className={`shrink-0 flex items-center gap-2 px-3 py-1.5 rounded-full text-xs font-semibold transition-colors whitespace-nowrap ${
            activeTab === 'transcript'
              ? 'bg-text-main text-surface-0 font-bold'
              : 'bg-surface-2 text-text-secondary hover:text-text-main hover:bg-surface-hover'
          }`}
        >
          <FileText size={14} aria-hidden="true" />
          Transcript
        </button>

        <button
          type="button"
          role="tab"
          id="tab-bookmarks"
          aria-selected={activeTab === 'bookmarks'}
          aria-controls="panel-bookmarks"
          onClick={() => setActiveTab('bookmarks')}
          className={`shrink-0 flex items-center gap-2 px-3 py-1.5 rounded-full text-xs font-semibold transition-colors whitespace-nowrap ${
            activeTab === 'bookmarks'
              ? 'bg-text-main text-surface-0 font-bold'
              : 'bg-surface-2 text-text-secondary hover:text-text-main hover:bg-surface-hover'
          }`}
        >
          <Bookmark size={14} aria-hidden="true" />
          Bookmarks
          <span
            className={`rounded-full px-1.5 py-0.5 text-[11px] leading-none ${
              activeTab === 'bookmarks'
                ? 'bg-surface-0/15 text-surface-0'
                : 'bg-surface-3 text-text-secondary'
            }`}
          >
            {bookmarks.length}
          </span>
        </button>
      </div>

      {activeTab === 'notes' && (
        <section
          role="tabpanel"
          id="panel-notes"
          aria-labelledby="tab-notes"
          className="space-y-4"
        >
          <Card className="p-5">
            {showNotesSource ? (
              renderSafeShowNotes(showNotesSource)
            ) : (
              <p className="text-sm text-text-secondary">
                No show notes were provided for this episode.
              </p>
            )}

            {safeWebsiteUrl && (
              <div className="mt-5 border-t border-surface-border pt-4">
                <a
                  href={safeWebsiteUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  aria-label="Open episode webpage in external browser"
                  className="inline-flex items-center gap-2 text-sm font-semibold text-brand hover:text-brand-hover"
                >
                  <ExternalLink size={15} aria-hidden="true" />
                  Episode webpage
                </a>
              </div>
            )}
          </Card>
        </section>
      )}

      {activeTab === 'chapters' && (
        <section
          role="tabpanel"
          id="panel-chapters"
          aria-labelledby="tab-chapters"
          className="space-y-3"
        >
          {chaptersLoading && (
            <Card className="p-5">
              <div role="status" className="flex items-center gap-2 text-sm text-text-secondary">
                <Loader2 size={16} className="animate-spin" aria-hidden="true" />
                Loading chapters…
              </div>
            </Card>
          )}

          {chaptersError && !chaptersLoading && (
            <Card className="p-5">
              <p role="alert" className="text-sm text-error">
                {chaptersError}
              </p>
              <Button
                variant="secondary"
                size="sm"
                className="mt-3"
                onClick={() => setChaptersRetryNonce(n => n + 1)}
                aria-label="Retry loading chapters"
              >
                Retry
              </Button>
            </Card>
          )}

          {!chaptersLoading && !chaptersError && chapters.length === 0 && (
            <Card className="p-5">
              <p className="text-sm text-text-secondary">
                No chapters are available for this episode.
              </p>
            </Card>
          )}

          {!chaptersLoading &&
            !chaptersError &&
            chapters.map((chapter, index) => {
              const isActive = index === activeChapterIndex;
              const safeChapterImg = !brokenChapterImages[index]
                ? isSafeArtworkURL(chapter.img)
                : null;
              const safeChapterUrl = isSafeExternalURL(chapter.url);
              const timeLabel = formatTime(chapter.startTime);

              return (
                <Card
                  key={`${chapter.startTime}-${index}`}
                  interactive
                  className={`flex items-center gap-3 p-3 ${
                    isActive
                      ? 'border border-brand/50 bg-surface-highlight'
                      : 'border border-surface-border'
                  }`}
                >
                  <button
                    type="button"
                    disabled={!detail.hasAudio && !isCurrentEpisode}
                    onClick={() => seekToSeconds(chapter.startTime)}
                    aria-label={`Seek to chapter ${chapter.title} at ${timeLabel}`}
                    className="flex min-w-0 flex-1 items-center gap-3 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand rounded-md disabled:opacity-50"
                  >
                    {safeChapterImg && (
                      <img
                        src={safeChapterImg}
                        alt=""
                        referrerPolicy="no-referrer"
                        onError={() =>
                          setBrokenChapterImages(prev => ({ ...prev, [index]: true }))
                        }
                        className="h-10 w-10 shrink-0 rounded-md object-cover bg-surface-3"
                      />
                    )}
                    <span className="w-14 shrink-0 font-mono text-xs font-semibold text-brand">
                      {timeLabel}
                    </span>
                    <span className="min-w-0 flex-1 truncate text-sm font-medium text-text-main">
                      {chapter.title}
                    </span>
                    {isActive && (
                      <span className="shrink-0 rounded-full bg-brand/15 px-2 py-0.5 text-[11px] font-semibold text-brand">
                        Playing
                      </span>
                    )}
                  </button>
                  {safeChapterUrl && (
                    <a
                      href={safeChapterUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label={`Open link for chapter ${chapter.title}`}
                      className="shrink-0 rounded-full p-2 text-text-secondary transition-colors hover:bg-surface-3 hover:text-text-main"
                    >
                      <ExternalLink size={14} aria-hidden="true" />
                    </a>
                  )}
                </Card>
              );
            })}
        </section>
      )}

      {activeTab === 'transcript' && (
        <section
          role="tabpanel"
          id="panel-transcript"
          aria-labelledby="tab-transcript"
          className="space-y-4"
        >
          <TextInput
            value={transcriptQuery}
            onChange={e => {
              setTranscriptQuery(e.target.value);
              setShowAllCues(false);
            }}
            onClear={() => {
              setTranscriptQuery('');
              setShowAllCues(false);
            }}
            placeholder="Filter transcript by keyword or speaker..."
            aria-label="Filter transcript"
            clearLabel="Clear transcript filter"
            leftIcon={<Search size={16} className="text-text-subtle shrink-0" aria-hidden="true" />}
          />

          {transcriptLoading && (
            <Card className="p-5">
              <div role="status" className="flex items-center gap-2 text-sm text-text-secondary">
                <Loader2 size={16} className="animate-spin" aria-hidden="true" />
                Loading transcript…
              </div>
            </Card>
          )}

          {transcriptError && !transcriptLoading && (
            <Card className="p-5">
              <p role="alert" className="text-sm text-error">
                {transcriptError}
              </p>
              <Button
                variant="secondary"
                size="sm"
                className="mt-3"
                onClick={() => setTranscriptRetryNonce(n => n + 1)}
                aria-label="Retry loading transcript"
              >
                Retry
              </Button>
            </Card>
          )}

          {!transcriptLoading && !transcriptError && cues.length === 0 && (
            <Card className="p-5">
              <p className="text-sm text-text-secondary">
                No transcript is available for this episode.
              </p>
            </Card>
          )}

          {!transcriptLoading && !transcriptError && cues.length > 0 && filteredCues.length === 0 && (
            <Card className="p-5">
              <p className="text-sm text-text-secondary">
                No transcript lines match "{transcriptQuery.trim()}".
              </p>
            </Card>
          )}

          {!transcriptLoading && !transcriptError && filteredCues.length > 0 && (
            <div className="space-y-2">
              {transcriptQuery.trim() && (
                <p role="status" className="text-xs text-text-subtle">
                  Showing {filteredCues.length} of {cues.length} transcript cues
                </p>
              )}
              {displayedCues.map((cue, index) => {
                const startSec = cue.startMs / 1000;
                const timeLabel = formatTime(startSec);
                const isActiveCue =
                  isCurrentEpisode &&
                  activePlaybackSeconds * 1000 >= cue.startMs &&
                  activePlaybackSeconds * 1000 < cue.endMs;

                return (
                  <button
                    key={`${cue.startMs}-${index}`}
                    type="button"
                    disabled={!detail.hasAudio && !isCurrentEpisode}
                    onClick={() => seekToSeconds(startSec)}
                    aria-label={`Seek to ${timeLabel}${cue.speaker ? ` ${cue.speaker}` : ''}`}
                    className={`flex w-full items-start gap-3 rounded-lg border p-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:opacity-50 ${
                      isActiveCue
                        ? 'border-brand/50 bg-surface-highlight'
                        : 'border-surface-border bg-surface-2/80 hover:bg-surface-hover'
                    }`}
                  >
                    <span className="mt-0.5 w-14 shrink-0 font-mono text-xs font-semibold text-brand">
                      {timeLabel}
                    </span>
                    <div className="min-w-0 flex-1">
                      {cue.speaker && (
                        <span className="mb-0.5 block text-xs font-semibold text-text-main">
                          {cue.speaker}
                        </span>
                      )}
                      <p className="text-sm leading-relaxed text-text-secondary">{cue.text}</p>
                    </div>
                  </button>
                );
              })}

              {!showAllCues && filteredCues.length > INITIAL_TRANSCRIPT_RENDER_LIMIT && (
                <div className="pt-2">
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => setShowAllCues(true)}
                    aria-label={`Show all ${filteredCues.length} transcript cues`}
                  >
                    Show all {filteredCues.length} cues
                  </Button>
                </div>
              )}
            </div>
          )}
        </section>
      )}

      {activeTab === 'bookmarks' && (
        <section
          role="tabpanel"
          id="panel-bookmarks"
          aria-labelledby="tab-bookmarks"
          className="space-y-4"
        >
          <Card className="p-5">
            <h2 className="mb-3 text-base font-semibold text-text-main">Add Bookmark</h2>
            <form onSubmit={e => void handleCreateBookmark(e)} className="space-y-3">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
                <div className="sm:w-44">
                  <TextInput
                    value={bookmarkTimeInput}
                    onChange={e => {
                      setBookmarkTimeDirty(true);
                      setBookmarkTimeInput(e.target.value);
                      setBookmarkFormError('');
                    }}
                    placeholder="MM:SS"
                    aria-label="Bookmark timestamp"
                  />
                </div>
                <div className="flex-1">
                  <TextInput
                    value={bookmarkNote}
                    maxLength={MAX_BOOKMARK_NOTE_LENGTH}
                    onChange={e => {
                      setBookmarkNote(e.target.value);
                      setBookmarkFormError('');
                    }}
                    onClear={() => setBookmarkNote('')}
                    clearLabel="Clear bookmark note"
                    placeholder="Add a note (optional, max 500 chars)"
                    aria-label="Bookmark note"
                  />
                </div>
                <Button
                  type="submit"
                  variant="primary"
                  size="md"
                  disabled={bookmarkSaving}
                  aria-label="Save bookmark"
                  leftIcon={
                    bookmarkSaving ? (
                      <Loader2 size={15} className="animate-spin" aria-hidden="true" />
                    ) : (
                      <Bookmark size={15} aria-hidden="true" />
                    )
                  }
                >
                  Save
                </Button>
              </div>
              <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-text-subtle">
                <div className="flex items-center gap-3">
                  <span>Format: MM:SS, HH:MM:SS, or seconds</span>
                  {bookmarkTimeDirty && (
                    <button
                      type="button"
                      onClick={() => {
                        setBookmarkTimeDirty(false);
                        const defaultSec =
                          isCurrentEpisode && liveTimeSeconds !== null
                            ? liveTimeSeconds
                            : Math.max(0, (detail.positionMs ?? 0) / 1000);
                        setBookmarkTimeInput(formatTime(defaultSec));
                      }}
                      className="font-semibold text-brand hover:text-brand-hover"
                      aria-label="Use current playback time"
                    >
                      Use current time
                    </button>
                  )}
                </div>
                <span>
                  {bookmarkNote.length}/{MAX_BOOKMARK_NOTE_LENGTH}
                </span>
              </div>
              {bookmarkFormError && (
                <p role="alert" className="text-xs text-error">
                  {bookmarkFormError}
                </p>
              )}
            </form>
          </Card>

          {bookmarksLoading && (
            <Card className="p-5">
              <div role="status" className="flex items-center gap-2 text-sm text-text-secondary">
                <Loader2 size={16} className="animate-spin" aria-hidden="true" />
                Loading bookmarks…
              </div>
            </Card>
          )}

          {bookmarksError && (
            <p role="alert" className="rounded-lg border border-error/30 bg-surface-1 p-3 text-sm text-error">
              {bookmarksError}
            </p>
          )}

          {!bookmarksLoading && bookmarks.length === 0 && (
            <Card className="p-5">
              <p className="text-sm text-text-secondary">
                No bookmarks saved for this episode yet.
              </p>
            </Card>
          )}

          {!bookmarksLoading && bookmarks.length > 0 && (
            <div className="space-y-2">
              {bookmarks.map(bookmark => {
                const startSec = bookmark.positionMs / 1000;
                const timeLabel = formatTime(startSec);
                return (
                  <Card
                    key={bookmark.id}
                    className="flex items-center justify-between gap-3 border border-surface-border p-3"
                  >
                    <button
                      type="button"
                      disabled={!detail.hasAudio && !isCurrentEpisode}
                      onClick={() => seekToSeconds(startSec)}
                      aria-label={`Seek to bookmark at ${timeLabel}`}
                      className="flex min-w-0 flex-1 items-start gap-3 rounded-md text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:opacity-50"
                    >
                      <span className="mt-0.5 w-14 shrink-0 font-mono text-xs font-semibold text-brand">
                        {timeLabel}
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="text-sm text-text-main break-words">
                          {bookmark.note || 'Saved moment'}
                        </p>
                        {bookmark.createdAt > 0 && (
                          <p className="mt-0.5 text-xs text-text-subtle">
                            {new Date(bookmark.createdAt).toLocaleDateString()}
                          </p>
                        )}
                      </div>
                    </button>
                    <Button
                      variant="ghost"
                      size="icon"
                      disabled={deletingBookmarkId === bookmark.id}
                      onClick={() => void handleDeleteBookmark(bookmark)}
                      aria-label={`Delete bookmark at ${timeLabel}`}
                      className="text-text-secondary hover:text-error"
                    >
                      {deletingBookmarkId === bookmark.id ? (
                        <Loader2 size={15} className="animate-spin" aria-hidden="true" />
                      ) : (
                        <Trash2 size={15} aria-hidden="true" />
                      )}
                    </Button>
                  </Card>
                );
              })}
            </div>
          )}
        </section>
      )}
    </Page>
  );
};
