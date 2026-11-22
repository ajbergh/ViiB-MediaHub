import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router';
import {
  ArrowLeft,
  Check,
  ChevronDown,
  ChevronUp,
  ExternalLink,
  Headphones,
  KeyRound,
  Loader2,
  Plus,
  Rss,
  Search,
  Settings as SettingsIcon,
  TrendingUp,
  X,
} from 'lucide-react';
import { Page, PageHeader } from '../components/ui/Page';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { TextInput } from '../components/ui/TextInput';
import { Chip } from '../components/ui/Chip';
import { Skeleton } from '../components/Skeleton';
import { EmptyState } from '../components/EmptyState';
import { useStore } from '../store';
import {
  podcastsAPI,
  PodcastAPIError,
  type PodcastDirectoryShow,
  type PodcastDirectoryStatus,
  type PodcastPreview,
} from '../services/podcasts';

const TRENDING_CATEGORIES = ['All', 'Technology', 'News', 'Science', 'Music', 'Comedy'] as const;
const TRENDING_LANGUAGES = [
  { code: '', label: 'All Languages' },
  { code: 'en', label: 'English' },
  { code: 'es', label: 'Spanish' },
  { code: 'fr', label: 'French' },
  { code: 'de', label: 'German' },
  { code: 'ja', label: 'Japanese' },
] as const;

function readableError(error: unknown): string {
  return error instanceof Error ? error.message : 'The podcast discovery service is unavailable.';
}

function isProviderNotConfiguredError(error: unknown): boolean {
  return (
    error instanceof PodcastAPIError &&
    (error.code === 'provider_not_configured' || error.code === 'directory_reauth_required')
  );
}

function formatEpisodeDuration(durationSeconds?: number): string {
  if (!durationSeconds || durationSeconds <= 0) return '';
  const mins = Math.round(durationSeconds / 60);
  if (mins < 60) return `${mins} min`;
  const hrs = Math.floor(mins / 60);
  const rem = mins % 60;
  return rem > 0 ? `${hrs}h ${rem}m` : `${hrs}h`;
}

const DirectoryArtwork: React.FC<{ title: string; artworkUrl?: string; size?: 'md' | 'lg' }> = ({
  title,
  artworkUrl,
  size = 'md',
}) => {
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    setFailed(false);
  }, [artworkUrl]);

  const sizeClass = size === 'lg' ? 'size-20' : 'size-16';
  const iconSize = size === 'lg' ? 32 : 26;

  return (
    <span
      className={`relative flex ${sizeClass} shrink-0 items-center justify-center overflow-hidden rounded-lg bg-surface-highlight text-brand`}
    >
      <Headphones size={iconSize} aria-hidden="true" />
      {artworkUrl && !failed && (
        <img
          src={artworkUrl}
          alt={`${title} cover art`}
          loading="lazy"
          referrerPolicy="no-referrer"
          onError={() => setFailed(true)}
          className="absolute inset-0 h-full w-full object-cover"
        />
      )}
    </span>
  );
};

const DirectorySkeletonGrid: React.FC<{ count?: number }> = ({ count = 6 }) => (
  <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" aria-label="Loading podcasts">
    {Array.from({ length: count }).map((_, index) => (
      <Card key={index} variant="default" className="border border-surface-border space-y-3">
        <div className="flex gap-3">
          <Skeleton className="size-16 shrink-0 rounded-lg" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-4 w-4/5" />
            <Skeleton className="h-3 w-1/2" />
            <Skeleton className="h-3 w-1/3" />
          </div>
        </div>
        <div className="flex gap-2">
          <Skeleton className="h-6 w-16 rounded-full" />
          <Skeleton className="h-6 w-20 rounded-full" />
        </div>
        <Skeleton className="h-3 w-full" />
        <Skeleton className="h-3 w-5/6" />
        <div className="flex gap-2 pt-1">
          <Skeleton className="h-8 w-24 rounded-full" />
          <Skeleton className="h-8 w-20 rounded-full" />
        </div>
      </Card>
    ))}
  </div>
);

export const PodcastDiscover: React.FC = () => {
  const navigate = useNavigate();

  // Directory configuration state
  const [status, setStatus] = useState<PodcastDirectoryStatus | null>(null);
  const [statusLoading, setStatusLoading] = useState(true);
  const [statusError, setStatusError] = useState('');

  // Inline quick credentials form (when unconfigured)
  const [showCredentialForm, setShowCredentialForm] = useState(false);
  const [credApiKey, setCredApiKey] = useState('');
  const [credApiSecret, setCredApiSecret] = useState('');
  const [credSaving, setCredSaving] = useState(false);
  const [credError, setCredError] = useState('');

  // Followed feed tracking (feedUrl -> feedId)
  const [followedByUrl, setFollowedByUrl] = useState<Record<string, string>>({});
  const [subscribingUrl, setSubscribingUrl] = useState<string | null>(null);

  // Directory Search state
  const [searchInput, setSearchInput] = useState('');
  const [activeSearchQuery, setActiveSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<PodcastDirectoryShow[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState('');
  const searchAbortRef = useRef<AbortController | null>(null);
  const searchDebounceRef = useRef<number | null>(null);

  // Directory Trending state
  const [selectedCategory, setSelectedCategory] = useState<string>('All');
  const [selectedLang, setSelectedLang] = useState<string>('');
  const [trendingShows, setTrendingShows] = useState<PodcastDirectoryShow[]>([]);
  const [trendingLoading, setTrendingLoading] = useState(false);
  const [trendingError, setTrendingError] = useState('');
  const trendingAbortRef = useRef<AbortController | null>(null);

  // Inline RSS Preview panel (shared for directory shows or direct RSS fallback)
  const [previewTargetUrl, setPreviewTargetUrl] = useState<string | null>(null);
  const [previewLoadingUrl, setPreviewLoadingUrl] = useState<string | null>(null);
  const [previewData, setPreviewData] = useState<PodcastPreview | null>(null);
  const [previewError, setPreviewError] = useState('');
  const previewAbortRef = useRef<AbortController | null>(null);

  // Direct RSS URL fallback state
  const [directUrl, setDirectUrl] = useState('');
  const [directPreview, setDirectPreview] = useState<PodcastPreview | null>(null);
  const [directWorking, setDirectWorking] = useState<'preview' | 'subscribe' | null>(null);
  const [directError, setDirectError] = useState('');

  const loadDiscoverStatus = useCallback(async (signal?: AbortSignal) => {
    setStatusLoading(true);
    setStatusError('');
    try {
      const response = await podcastsAPI.getDiscoverStatus(signal);
      if (signal?.aborted) return;
      setStatus(response.status);
    } catch (error) {
      if (signal?.aborted) return;
      setStatusError(readableError(error));
    } finally {
      if (!signal?.aborted) {
        setStatusLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void loadDiscoverStatus(controller.signal);
    void podcastsAPI
      .listSubscriptions(controller.signal)
      .then(({ feeds }) => {
        if (controller.signal.aborted) return;
        const nextMap: Record<string, string> = {};
        for (const feed of feeds) {
          if (feed.url) nextMap[feed.url] = feed.id;
        }
        setFollowedByUrl(nextMap);
      })
      .catch(() => {
        // Subscription list is supplemental for "Following" badges.
      });

    return () => {
      controller.abort();
      searchAbortRef.current?.abort();
      trendingAbortRef.current?.abort();
      previewAbortRef.current?.abort();
      if (searchDebounceRef.current !== null) {
        window.clearTimeout(searchDebounceRef.current);
      }
    };
  }, [loadDiscoverStatus]);

  // Load Trending when configured
  const fetchTrending = useCallback(async (category: string, lang: string) => {
    trendingAbortRef.current?.abort();
    const controller = new AbortController();
    trendingAbortRef.current = controller;

    setTrendingLoading(true);
    setTrendingError('');
    try {
      const response = await podcastsAPI.trendingDirectory(
        {
          category: category === 'All' ? undefined : category,
          lang: lang || undefined,
          max: 18,
        },
        controller.signal,
      );
      if (controller.signal.aborted) return;
      setTrendingShows(response.shows);
    } catch (error) {
      if (controller.signal.aborted) return;
      if (isProviderNotConfiguredError(error)) {
        setStatus(prev => ({
          configured: false,
          provider: prev?.provider ?? 'none',
          keyHint: '',
        }));
      }
      setTrendingError(readableError(error));
    } finally {
      if (!controller.signal.aborted) {
        setTrendingLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    if (!status?.configured) return;
    void fetchTrending(selectedCategory, selectedLang);
  }, [status?.configured, selectedCategory, selectedLang, fetchTrending]);

  const executeSearch = useCallback(async (rawQuery: string) => {
    const trimmed = rawQuery.trim();
    if (!trimmed) {
      searchAbortRef.current?.abort();
      setActiveSearchQuery('');
      setSearchResults([]);
      setSearchError('');
      setSearching(false);
      return;
    }

    if (trimmed.length > 200) {
      setSearchError('Search query must be 200 characters or fewer.');
      return;
    }

    searchAbortRef.current?.abort();
    const controller = new AbortController();
    searchAbortRef.current = controller;

    setActiveSearchQuery(trimmed);
    setSearching(true);
    setSearchError('');
    try {
      const response = await podcastsAPI.searchDirectory(trimmed, 24, controller.signal);
      if (controller.signal.aborted) return;
      setSearchResults(response.shows);
    } catch (error) {
      if (controller.signal.aborted) return;
      if (isProviderNotConfiguredError(error)) {
        setStatus(prev => ({
          configured: false,
          provider: prev?.provider ?? 'none',
          keyHint: '',
        }));
      }
      setSearchError(readableError(error));
    } finally {
      if (!controller.signal.aborted) {
        setSearching(false);
      }
    }
  }, []);

  const handleSearchInputChange = (value: string) => {
    setSearchInput(value);
    if (searchDebounceRef.current !== null) {
      window.clearTimeout(searchDebounceRef.current);
      searchDebounceRef.current = null;
    }
    const trimmed = value.trim();
    if (!trimmed) {
      searchAbortRef.current?.abort();
      setActiveSearchQuery('');
      setSearchResults([]);
      setSearchError('');
      setSearching(false);
      return;
    }
    if (trimmed.length < 2 || !status?.configured) return;

    searchDebounceRef.current = window.setTimeout(() => {
      searchDebounceRef.current = null;
      void executeSearch(trimmed);
    }, 400);
  };

  const handleSearchSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    if (searchDebounceRef.current !== null) {
      window.clearTimeout(searchDebounceRef.current);
      searchDebounceRef.current = null;
    }
    if (!searchInput.trim()) return;
    void executeSearch(searchInput);
  };

  const handleClearSearch = () => {
    if (searchDebounceRef.current !== null) {
      window.clearTimeout(searchDebounceRef.current);
      searchDebounceRef.current = null;
    }
    searchAbortRef.current?.abort();
    setSearchInput('');
    setActiveSearchQuery('');
    setSearchResults([]);
    setSearchError('');
    setSearching(false);
  };

  const handleSaveCredentials = async (event: React.FormEvent) => {
    event.preventDefault();
    const apiKey = credApiKey.trim();
    const apiSecret = credApiSecret.trim();
    if (!apiKey || !apiSecret) {
      setCredError('Provide both a Podcast Index API Key and API Secret.');
      return;
    }
    setCredSaving(true);
    setCredError('');
    try {
      const response = await podcastsAPI.setDiscoverCredentials(apiKey, apiSecret);
      setCredApiKey('');
      setCredApiSecret('');
      setShowCredentialForm(false);
      setStatus(response.status);
      setTrendingError('');
      setSearchError('');
      useStore.getState().showToast({
        type: 'success',
        message: 'Podcast Index credentials saved.',
      });
    } catch (error) {
      const msg = readableError(error);
      setCredError(msg);
      useStore.getState().showToast({
        type: 'error',
        message: msg,
      });
    } finally {
      setCredSaving(false);
    }
  };

  const handlePreviewShow = async (feedUrl: string) => {
    if (previewTargetUrl === feedUrl && previewData && !previewLoadingUrl) {
      setPreviewTargetUrl(null);
      setPreviewData(null);
      setPreviewError('');
      return;
    }

    previewAbortRef.current?.abort();
    const controller = new AbortController();
    previewAbortRef.current = controller;

    setPreviewTargetUrl(feedUrl);
    setPreviewLoadingUrl(feedUrl);
    setPreviewData(null);
    setPreviewError('');
    try {
      const preview = await podcastsAPI.previewFeed(feedUrl, controller.signal);
      if (controller.signal.aborted) return;
      setPreviewData(preview);
    } catch (error) {
      if (controller.signal.aborted) return;
      setPreviewError(readableError(error));
    } finally {
      if (!controller.signal.aborted) {
        setPreviewLoadingUrl(null);
      }
    }
  };

  const handleFollowFeed = async (feedUrl: string) => {
    if (subscribingUrl) return;
    setSubscribingUrl(feedUrl);
    try {
      const result = await podcastsAPI.subscribe(feedUrl);
      setFollowedByUrl(prev => ({ ...prev, [feedUrl]: result.id }));
      useStore.getState().showToast({
        type: 'success',
        message: `Following ${result.title}`,
      });
      navigate('/podcasts/show/' + encodeURIComponent(result.id));
    } catch (error) {
      useStore.getState().showToast({
        type: 'error',
        message: readableError(error),
      });
    } finally {
      setSubscribingUrl(null);
    }
  };

  const handleDirectPreview = async (event: React.FormEvent) => {
    event.preventDefault();
    const trimmed = directUrl.trim();
    if (!trimmed) return;
    setDirectWorking('preview');
    setDirectError('');
    setDirectPreview(null);
    try {
      const preview = await podcastsAPI.previewFeed(trimmed);
      setDirectPreview(preview);
    } catch (error) {
      setDirectError(readableError(error));
    } finally {
      setDirectWorking(null);
    }
  };

  const handleDirectFollow = async () => {
    const trimmed = directUrl.trim();
    if (!trimmed || directWorking) return;
    setDirectWorking('subscribe');
    setDirectError('');
    try {
      const result = await podcastsAPI.subscribe(trimmed);
      setFollowedByUrl(prev => ({ ...prev, [trimmed]: result.id }));
      setDirectPreview(null);
      setDirectUrl('');
      useStore.getState().showToast({
        type: 'success',
        message: `Following ${result.title}`,
      });
      navigate('/podcasts/show/' + encodeURIComponent(result.id));
    } catch (error) {
      const msg = readableError(error);
      setDirectError(msg);
      useStore.getState().showToast({
        type: 'error',
        message: msg,
      });
    } finally {
      setDirectWorking(null);
    }
  };

  const renderShowGrid = (shows: PodcastDirectoryShow[]) => (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {shows.map(show => {
        const followedId = followedByUrl[show.feedUrl];
        const isPreviewingThis = previewLoadingUrl === show.feedUrl;
        const isPreviewOpen = previewTargetUrl === show.feedUrl;
        const isSubscribingThis = subscribingUrl === show.feedUrl;
        const categories = (show.categories ?? []).filter(Boolean).slice(0, 3);

        return (
          <Card
            key={`${show.id}-${show.feedUrl}`}
            variant="default"
            className="flex flex-col justify-between gap-4 border border-surface-border"
          >
            <div className="space-y-3">
              <div className="flex items-start gap-3">
                <DirectoryArtwork title={show.title} artworkUrl={show.artworkUrl} />
                <div className="min-w-0 flex-1">
                  <h3 className="line-clamp-2 text-base font-semibold text-text-main">
                    {show.title}
                  </h3>
                  <p className="mt-0.5 truncate text-xs text-text-secondary">
                    {show.author || 'Independent publisher'}
                  </p>
                  <p className="mt-1 text-xs text-text-subtle">
                    {[
                      typeof show.episodeCount === 'number' && show.episodeCount > 0
                        ? `${show.episodeCount} episode${show.episodeCount === 1 ? '' : 's'}`
                        : null,
                      show.language ? show.language.toUpperCase() : null,
                    ]
                      .filter(Boolean)
                      .join(' · ') || 'Public RSS feed'}
                  </p>
                </div>
              </div>

              {categories.length > 0 && (
                <div className="flex flex-wrap gap-1.5">
                  {categories.map(category => (
                    <Chip
                      key={category}
                      selected={
                        !activeSearchQuery &&
                        selectedCategory.toLowerCase() === category.toLowerCase()
                      }
                      onClick={() => {
                        if (activeSearchQuery) {
                          setSearchInput(category);
                          void executeSearch(category);
                        } else if (
                          TRENDING_CATEGORIES.some(
                            item => item.toLowerCase() === category.toLowerCase(),
                          )
                        ) {
                          const matched = TRENDING_CATEGORIES.find(
                            item => item.toLowerCase() === category.toLowerCase(),
                          );
                          if (matched) setSelectedCategory(matched);
                        } else {
                          setSearchInput(category);
                          void executeSearch(category);
                        }
                      }}
                      className="px-2.5 py-0.5 text-[11px]"
                    >
                      {category}
                    </Chip>
                  ))}
                </div>
              )}

              {show.description && (
                <p className="line-clamp-3 text-xs leading-relaxed text-text-secondary">
                  {show.description}
                </p>
              )}
            </div>

            <div className="flex flex-wrap items-center gap-2 pt-1">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => void handlePreviewShow(show.feedUrl)}
                disabled={isPreviewingThis}
                leftIcon={
                  isPreviewingThis ? (
                    <Loader2 size={14} className="animate-spin" aria-hidden="true" />
                  ) : (
                    <Rss size={14} aria-hidden="true" />
                  )
                }
              >
                {isPreviewOpen && previewData ? 'Hide Preview' : 'Preview RSS'}
              </Button>

              {followedId ? (
                <Link
                  to={'/podcasts/show/' + encodeURIComponent(followedId)}
                  className="inline-flex items-center justify-center gap-1.5 rounded-full bg-surface-highlight px-3 py-1.5 text-xs font-semibold text-success transition-colors duration-150 ease-out hover:bg-surface-3"
                >
                  <Check size={14} aria-hidden="true" />
                  Following
                </Link>
              ) : (
                <Button
                  variant="primary"
                  size="sm"
                  accent="brand"
                  onClick={() => void handleFollowFeed(show.feedUrl)}
                  disabled={subscribingUrl !== null}
                  leftIcon={
                    isSubscribingThis ? (
                      <Loader2 size={14} className="animate-spin" aria-hidden="true" />
                    ) : (
                      <Plus size={14} aria-hidden="true" />
                    )
                  }
                >
                  {isSubscribingThis ? 'Following…' : 'Follow'}
                </Button>
              )}
            </div>
          </Card>
        );
      })}
    </div>
  );

  return (
    <Page>
      <PageHeader
        heading="Discover Podcasts"
        subtitle="Search public podcasts via Podcast Index or subscribe directly by RSS feed URL."
        actions={
          <Link
            to="/podcasts"
            className="inline-flex items-center justify-center gap-2 rounded-full bg-surface-3 px-4 py-2 text-[13px] font-semibold text-text-main transition-colors duration-150 ease-out hover:bg-surface-highlight focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-surface-0"
          >
            <ArrowLeft size={16} aria-hidden="true" />
            Back to Podcasts
          </Link>
        }
      />

      <div className="space-y-6">
        {statusError && (
          <div
            role="alert"
            className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-error/40 bg-error/10 p-4 text-sm text-error"
          >
            <span>{statusError}</span>
            <Button variant="secondary" size="sm" onClick={() => void loadDiscoverStatus()}>
              Retry
            </Button>
          </div>
        )}

        {/* Initial Status Loading Skeleton */}
        {statusLoading && (
          <div className="space-y-4">
            <Card variant="default" className="border border-surface-border space-y-3">
              <Skeleton className="h-5 w-48" />
              <Skeleton className="h-10 w-full rounded-full" />
            </Card>
            <DirectorySkeletonGrid count={3} />
          </div>
        )}

        {/* Unconfigured State */}
        {!statusLoading && status && !status.configured && (
          <Card variant="hero" className="border border-surface-border space-y-4">
            <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
              <div className="space-y-2">
                <div className="flex items-center gap-2 text-brand">
                  <KeyRound size={18} aria-hidden="true" />
                  <span className="text-xs font-bold uppercase tracking-wider">
                    Podcast Index Directory
                  </span>
                </div>
                <h2 className="text-lg font-bold text-text-main">
                  Connect your free Podcast Index API credentials
                </h2>
                <p className="max-w-2xl text-sm text-text-secondary">
                  Directory search and trending charts use user-supplied free credentials from{' '}
                  <a
                    href="https://api.podcastindex.org"
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1 font-medium text-brand hover:underline"
                  >
                    https://api.podcastindex.org
                    <ExternalLink size={12} aria-hidden="true" />
                  </a>
                  . Your credentials are stored encrypted on this device and never shared. You can
                  also subscribe directly with any public RSS feed URL below.
                </p>
              </div>

              <div className="flex flex-wrap items-center gap-2 shrink-0">
                <Button
                  variant="primary"
                  size="md"
                  accent="brand"
                  onClick={() => {
                    setShowCredentialForm(current => !current);
                    setCredError('');
                  }}
                  rightIcon={
                    showCredentialForm ? (
                      <ChevronUp size={16} aria-hidden="true" />
                    ) : (
                      <ChevronDown size={16} aria-hidden="true" />
                    )
                  }
                >
                  Configure Podcast Index Credentials
                </Button>
                <Link
                  to="/settings"
                  state={{ tab: 'integrations' }}
                  className="inline-flex items-center justify-center gap-2 rounded-full bg-surface-3 px-4 py-2 text-[13px] font-semibold text-text-main transition-colors duration-150 ease-out hover:bg-surface-highlight focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-surface-0"
                >
                  <SettingsIcon size={15} aria-hidden="true" />
                  Open Settings
                </Link>
              </div>
            </div>

            {showCredentialForm && (
              <form
                onSubmit={handleSaveCredentials}
                className="mt-4 space-y-4 rounded-lg border border-surface-border bg-surface-1 p-4"
              >
                <div className="grid gap-4 sm:grid-cols-2">
                  <div>
                    <label
                      htmlFor="discover-inline-api-key"
                      className="mb-1.5 block text-xs font-semibold uppercase tracking-wide text-text-secondary"
                    >
                      API Key
                    </label>
                    <TextInput
                      id="discover-inline-api-key"
                      type="password"
                      value={credApiKey}
                      onChange={event => {
                        setCredApiKey(event.target.value);
                        setCredError('');
                      }}
                      placeholder="Enter Podcast Index API key"
                      autoComplete="off"
                      disabled={credSaving}
                    />
                  </div>
                  <div>
                    <label
                      htmlFor="discover-inline-api-secret"
                      className="mb-1.5 block text-xs font-semibold uppercase tracking-wide text-text-secondary"
                    >
                      API Secret
                    </label>
                    <TextInput
                      id="discover-inline-api-secret"
                      type="password"
                      value={credApiSecret}
                      onChange={event => {
                        setCredApiSecret(event.target.value);
                        setCredError('');
                      }}
                      placeholder="Enter Podcast Index API secret"
                      autoComplete="off"
                      disabled={credSaving}
                    />
                  </div>
                </div>

                {credError && (
                  <p role="alert" className="text-sm text-error">
                    {credError}
                  </p>
                )}

                <div className="flex flex-wrap items-center gap-3">
                  <Button
                    type="submit"
                    variant="primary"
                    size="md"
                    accent="brand"
                    disabled={credSaving || !credApiKey.trim() || !credApiSecret.trim()}
                    leftIcon={
                      credSaving ? (
                        <Loader2 size={15} className="animate-spin" aria-hidden="true" />
                      ) : undefined
                    }
                  >
                    {credSaving ? 'Saving…' : 'Save Credentials'}
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="md"
                    onClick={() => {
                      setShowCredentialForm(false);
                      setCredApiKey('');
                      setCredApiSecret('');
                      setCredError('');
                    }}
                    disabled={credSaving}
                  >
                    Cancel
                  </Button>
                </div>
              </form>
            )}
          </Card>
        )}

        {/* Configured Directory Search & Trending */}
        {!statusLoading && status?.configured && (
          <>
            {/* Search Form */}
            <Card variant="default" className="border border-surface-border space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="text-base font-semibold text-text-main">Search Podcast Index</h2>
                  <a
                    href="https://podcastindex.org"
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1 text-xs text-text-subtle hover:text-text-secondary"
                  >
                    Powered by Podcast Index
                    <ExternalLink size={11} aria-hidden="true" />
                  </a>
                </div>
                <Link
                  to="/settings"
                  state={{ tab: 'integrations' }}
                  className="inline-flex items-center gap-1.5 text-xs font-medium text-text-secondary hover:text-text-main"
                >
                  <SettingsIcon size={13} aria-hidden="true" />
                  {status.source === 'embedded' ? 'Built-in key' : `Key ${status.keyHint}`}
                </Link>
              </div>

              <form onSubmit={handleSearchSubmit} className="flex flex-col gap-3 sm:flex-row">
                <TextInput
                  aria-label="Search podcasts"
                  value={searchInput}
                  onChange={event => handleSearchInputChange(event.target.value)}
                  onClear={handleClearSearch}
                  clearLabel="Clear podcast search"
                  placeholder="Search podcasts by title, host, or topic…"
                  className="flex-1"
                  leftIcon={<Search size={16} className="shrink-0 text-text-subtle" aria-hidden="true" />}
                />
                <Button
                  type="submit"
                  variant="primary"
                  size="md"
                  accent="brand"
                  disabled={searching || !searchInput.trim()}
                  leftIcon={
                    searching ? (
                      <Loader2 size={16} className="animate-spin" aria-hidden="true" />
                    ) : (
                      <Search size={16} aria-hidden="true" />
                    )
                  }
                >
                  Search
                </Button>
              </form>
            </Card>

            {/* Inline RSS Preview Panel for Directory Shows */}
            {previewTargetUrl && (
              <Card
                variant="hero"
                aria-label="Podcast feed preview"
                className="border border-brand/40 space-y-4"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-center gap-2 text-brand">
                    <Rss size={17} aria-hidden="true" />
                    <span className="text-xs font-bold uppercase tracking-wider">RSS Preview</span>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Close RSS preview"
                    onClick={() => {
                      previewAbortRef.current?.abort();
                      setPreviewTargetUrl(null);
                      setPreviewLoadingUrl(null);
                      setPreviewData(null);
                      setPreviewError('');
                    }}
                  >
                    <X size={16} aria-hidden="true" />
                  </Button>
                </div>

                {previewLoadingUrl && (
                  <div className="flex items-center gap-3 text-sm text-text-secondary" role="status">
                    <Loader2 size={16} className="animate-spin text-brand" aria-hidden="true" />
                    Fetching live RSS feed…
                  </div>
                )}

                {previewError && (
                  <p role="alert" className="text-sm text-error">
                    {previewError}
                  </p>
                )}

                {previewData && (
                  <div className="space-y-4">
                    <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                      <div className="flex items-start gap-4">
                        <DirectoryArtwork
                          title={previewData.feed.title}
                          artworkUrl={previewData.feed.imageUrl}
                          size="lg"
                        />
                        <div className="min-w-0 space-y-1">
                          <h3 className="text-lg font-bold text-text-main">
                            {previewData.feed.title}
                          </h3>
                          <p className="text-xs text-text-secondary">
                            {previewData.feed.author || 'Independent publisher'} ·{' '}
                            {previewData.episodeCount} episode
                            {previewData.episodeCount === 1 ? '' : 's'} in feed
                          </p>
                          {previewData.feed.description && (
                            <p className="line-clamp-3 max-w-3xl text-sm text-text-secondary">
                              {previewData.feed.description}
                            </p>
                          )}
                        </div>
                      </div>

                      {followedByUrl[previewTargetUrl] ? (
                        <Link
                          to={'/podcasts/show/' + encodeURIComponent(followedByUrl[previewTargetUrl])}
                          className="inline-flex shrink-0 items-center justify-center gap-2 rounded-full bg-surface-highlight px-4 py-2 text-[13px] font-semibold text-success hover:bg-surface-3"
                        >
                          <Check size={15} aria-hidden="true" />
                          Open Show
                        </Link>
                      ) : (
                        <Button
                          variant="primary"
                          size="md"
                          accent="brand"
                          className="shrink-0"
                          onClick={() => void handleFollowFeed(previewTargetUrl)}
                          disabled={subscribingUrl !== null}
                          leftIcon={
                            subscribingUrl === previewTargetUrl ? (
                              <Loader2 size={15} className="animate-spin" aria-hidden="true" />
                            ) : (
                              <Plus size={15} aria-hidden="true" />
                            )
                          }
                        >
                          Follow Podcast
                        </Button>
                      )}
                    </div>

                    {previewData.feed.episodes.length > 0 && (
                      <div className="space-y-2 border-t border-surface-border pt-3">
                        <p className="text-xs font-semibold uppercase tracking-wide text-text-subtle">
                          Recent Episodes
                        </p>
                        <ul className="divide-y divide-surface-border rounded-lg border border-surface-border bg-surface-1">
                          {previewData.feed.episodes.slice(0, 5).map((episode, index) => {
                            const durationText = formatEpisodeDuration(episode.durationSeconds);
                            return (
                              <li
                                key={episode.guid || `${episode.title}-${index}`}
                                className="flex items-center justify-between gap-3 px-3 py-2 text-xs"
                              >
                                <span className="truncate font-medium text-text-main">
                                  {episode.title}
                                </span>
                                {durationText && (
                                  <span className="shrink-0 text-text-subtle">{durationText}</span>
                                )}
                              </li>
                            );
                          })}
                        </ul>
                      </div>
                    )}
                  </div>
                )}
              </Card>
            )}

            {/* Search Results Section */}
            {activeSearchQuery ? (
              <section aria-label="Podcast search results" className="space-y-4">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div>
                    <h2 className="text-section text-text-main">
                      Results for &ldquo;{activeSearchQuery}&rdquo;
                    </h2>
                    {!searching && !searchError && (
                      <p className="text-xs text-text-subtle">
                        {searchResults.length} podcast{searchResults.length === 1 ? '' : 's'} found
                      </p>
                    )}
                  </div>
                  <Button variant="secondary" size="sm" onClick={handleClearSearch}>
                    Back to Trending
                  </Button>
                </div>

                {searchError && (
                  <div
                    role="alert"
                    className="rounded-lg border border-error/40 bg-error/10 p-4 text-sm text-error"
                  >
                    {searchError}
                  </div>
                )}

                {searching && <DirectorySkeletonGrid />}

                {!searching && !searchError && searchResults.length === 0 && (
                  <EmptyState
                    icon={<Search size={28} />}
                    title="No podcasts matched your search"
                    description="Try different keywords, or clear your search to browse trending shows."
                    action={{ label: 'Clear search', onClick: handleClearSearch }}
                  />
                )}

                {!searching && !searchError && searchResults.length > 0 && renderShowGrid(searchResults)}
              </section>
            ) : (
              /* Trending Section */
              <section aria-label="Trending podcasts" className="space-y-4">
                <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                  <div className="flex items-center gap-2">
                    <TrendingUp size={20} className="text-brand" aria-hidden="true" />
                    <h2 className="text-section text-text-main">Trending Podcasts</h2>
                  </div>

                  <div className="flex flex-wrap items-center gap-1.5" aria-label="Language filter">
                    {TRENDING_LANGUAGES.map(langOption => (
                      <Chip
                        key={langOption.code || 'all-lang'}
                        selected={selectedLang === langOption.code}
                        onClick={() => setSelectedLang(langOption.code)}
                        className="px-3 py-1 text-xs"
                      >
                        {langOption.label}
                      </Chip>
                    ))}
                  </div>
                </div>

                <div className="flex flex-wrap gap-2" aria-label="Category filter">
                  {TRENDING_CATEGORIES.map(category => (
                    <Chip
                      key={category}
                      selected={selectedCategory === category}
                      onClick={() => setSelectedCategory(category)}
                    >
                      {category}
                    </Chip>
                  ))}
                </div>

                {trendingError && (
                  <div
                    role="alert"
                    className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-error/40 bg-error/10 p-4 text-sm text-error"
                  >
                    <span>{trendingError}</span>
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => void fetchTrending(selectedCategory, selectedLang)}
                    >
                      Retry
                    </Button>
                  </div>
                )}

                {trendingLoading && <DirectorySkeletonGrid />}

                {!trendingLoading && !trendingError && trendingShows.length === 0 && (
                  <EmptyState
                    icon={<Headphones size={28} />}
                    title="No trending podcasts for this filter"
                    description="Try selecting All categories or a different language filter."
                    action={
                      selectedCategory !== 'All' || selectedLang !== ''
                        ? {
                            label: 'Reset filters',
                            onClick: () => {
                              setSelectedCategory('All');
                              setSelectedLang('');
                            },
                          }
                        : undefined
                    }
                  />
                )}

                {!trendingLoading &&
                  !trendingError &&
                  trendingShows.length > 0 &&
                  renderShowGrid(trendingShows)}
              </section>
            )}
          </>
        )}

        {/* Direct RSS URL Fallback (Always available, zero directory key required) */}
        <Card variant="default" className="border border-surface-border space-y-4">
          <div className="space-y-1">
            <div className="flex items-center gap-2 text-brand">
              <Rss size={18} aria-hidden="true" />
              <h2 className="text-lg font-semibold text-text-main">Direct RSS URL</h2>
            </div>
            <p className="text-sm text-text-secondary">
              Have a podcast feed URL? Preview and follow any public HTTPS RSS feed directly without
              directory credentials.
            </p>
          </div>

          <form onSubmit={handleDirectPreview} className="flex flex-col gap-3 sm:flex-row">
            <TextInput
              type="url"
              aria-label="Podcast RSS feed URL"
              value={directUrl}
              onChange={event => {
                setDirectUrl(event.target.value);
                setDirectPreview(null);
                setDirectError('');
              }}
              onClear={() => {
                setDirectUrl('');
                setDirectPreview(null);
                setDirectError('');
              }}
              clearLabel="Clear RSS URL"
              placeholder="https://publisher.example/podcast.xml"
              required
              autoComplete="url"
              className="flex-1"
              leftIcon={<Rss size={16} className="shrink-0 text-text-subtle" aria-hidden="true" />}
            />
            <div className="flex flex-wrap items-center gap-2 shrink-0">
              <Button
                type="submit"
                variant="secondary"
                size="md"
                disabled={directWorking !== null || !directUrl.trim()}
                leftIcon={
                  directWorking === 'preview' ? (
                    <Loader2 size={15} className="animate-spin" aria-hidden="true" />
                  ) : (
                    <Rss size={15} aria-hidden="true" />
                  )
                }
              >
                Preview RSS
              </Button>
              <Button
                type="button"
                variant="primary"
                size="md"
                accent="brand"
                disabled={directWorking !== null || !directUrl.trim()}
                onClick={() => void handleDirectFollow()}
                leftIcon={
                  directWorking === 'subscribe' ? (
                    <Loader2 size={15} className="animate-spin" aria-hidden="true" />
                  ) : (
                    <Plus size={15} aria-hidden="true" />
                  )
                }
              >
                {directWorking === 'subscribe' ? 'Following…' : 'Follow'}
              </Button>
            </div>
          </form>

          {directError && (
            <p role="alert" className="text-sm text-error">
              {directError}
            </p>
          )}

          {directPreview && (
            <div className="rounded-lg border border-surface-border bg-surface-1 p-4 space-y-3">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="flex items-start gap-3">
                  <DirectoryArtwork
                    title={directPreview.feed.title}
                    artworkUrl={directPreview.feed.imageUrl}
                  />
                  <div className="min-w-0 space-y-1">
                    <h3 className="text-base font-semibold text-text-main">
                      {directPreview.feed.title}
                    </h3>
                    <p className="text-xs text-text-secondary">
                      {directPreview.feed.author || 'Independent publisher'} ·{' '}
                      {directPreview.episodeCount} episode
                      {directPreview.episodeCount === 1 ? '' : 's'}
                    </p>
                    {directPreview.feed.description && (
                      <p className="line-clamp-3 text-xs text-text-secondary">
                        {directPreview.feed.description}
                      </p>
                    )}
                  </div>
                </div>
                <Button
                  type="button"
                  variant="primary"
                  size="sm"
                  accent="brand"
                  className="shrink-0"
                  disabled={directWorking !== null}
                  onClick={() => void handleDirectFollow()}
                  leftIcon={<Plus size={14} aria-hidden="true" />}
                >
                  Follow Podcast
                </Button>
              </div>
            </div>
          )}
        </Card>
      </div>
    </Page>
  );
};
