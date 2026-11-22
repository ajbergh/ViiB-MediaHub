
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { ArrowLeft, Download, Headphones, Loader2, Plus, RefreshCw, Rss, Trash2 } from 'lucide-react';
import { api } from '../services/api';
import { podcastEpisodeToTrack } from '../lib/podcastPlayback';
import { useStore } from '../store';
import { podcastsAPI, PodcastAPIError, type PodcastFeed, type PodcastEpisode, type PodcastPreview, type PodcastRefreshStatus, type PodcastOPMLImportPreview, type PodcastContinueEpisode } from '../services/podcasts';

const inputClass = 'w-full rounded-lg border border-surface-3 bg-surface-1 px-4 py-3 text-text-main outline-none focus-visible:ring-2 focus-visible:ring-brand';

function readableError(error: unknown): string {
  return error instanceof Error ? error.message : 'The podcast service is unavailable.';
}

/** Publisher images only load through the public-network-validated local API. */
const PodcastArtwork: React.FC<{ feedId: string; title: string; imageURL?: string; large?: boolean }> = ({ feedId, title, imageURL, large = false }) => {
  const [failed, setFailed] = useState(false);
  useEffect(() => { setFailed(false); }, [feedId, imageURL]);
  return <span className={`relative flex shrink-0 items-center justify-center overflow-hidden rounded-xl bg-surface-highlight text-brand ${large ? 'size-20' : 'size-14'}`}>
    <Headphones size={large ? 40 : 25} aria-hidden="true"/>
    {imageURL && !failed && <img
      src={'/api/v2/podcasts/feeds/' + encodeURIComponent(feedId) + '/artwork'}
      alt={title + ' cover art'} loading="lazy" referrerPolicy="no-referrer"
      onError={() => setFailed(true)}
      className="absolute inset-0 h-full w-full object-cover"
    />}
  </span>;
};

/** Early RSS subscription experience; deliberately has no fake player or download actions. */
export const Podcasts: React.FC = () => {
  const [feeds, setFeeds] = useState<PodcastFeed[]>([]);
  const [continueEpisodes, setContinueEpisodes] = useState<PodcastContinueEpisode[]>([]);
  const [followingQuery, setFollowingQuery] = useState('');
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
  const navigate = useNavigate();
  useEffect(() => () => { importController.current?.abort(); }, []);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    void podcastsAPI.listContinueListening(12, controller.signal)
      .then(({ episodes }) => { if (!controller.signal.aborted) setContinueEpisodes(episodes); })
      .catch(() => { if (!controller.signal.aborted) setContinueEpisodes([]); });
    void podcastsAPI.listSubscriptions(controller.signal)
      .then(({ feeds }) => { if (!controller.signal.aborted) { setFeeds(feeds); setError(''); } })
      .catch(err => { if (!controller.signal.aborted) setError(readableError(err)); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [revision]);

  const showPreview = useCallback(async (event: React.FormEvent) => {
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
  }, [url]);

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
          setOPML(current => current ? {
            ...current, feeds: current.feeds.map(feed => feed.url === candidate.url
              ? { ...feed, alreadyFollowed: true } : feed),
          } : null);
        } catch (err) {
          if (controller.signal.aborted) break;
          failures++;
          failedNames.push(`${candidate.title}: ${err instanceof PodcastAPIError ? err.code : 'request_failed'}`);
        }
        processed++;
        setImportDone(processed);
      }
    } finally {
      importController.current = null;
      setImportFailures(failedNames);
      setImportSummary(`Followed ${success} podcast${success === 1 ? '' : 's'}; ${failures} failed.${controller.signal.aborted ? ' Import canceled.' : ''}`);
      if (success > 0) setRevision(current => current + 1);
      setImportMode('idle');
    }
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
      try { anchor.click(); } finally {
        anchor.remove();
        window.setTimeout(() => URL.revokeObjectURL(objectURL), 30000);
      }
    } catch (err) {
      setError(readableError(err));
    } finally {
      setExporting(false);
    }
  };

  return (
    <main className="mx-auto w-full max-w-6xl space-y-8 p-5 pb-28 sm:p-8">
      <header>
        <p className="text-xs font-bold uppercase tracking-[0.18em] text-brand">Early access · RSS subscriptions</p>
        <h1 className="mt-2 text-3xl font-bold text-text-main">Podcasts</h1>
        <p className="mt-2 max-w-3xl text-text-secondary">Follow independent podcast feeds and browse their episodes. Search, streaming, offline listening, and smart playback controls are still under development.</p>
      </header>

      <section className="rounded-xl border border-surface-3 bg-surface-2 p-5">
        <h2 className="mb-3 text-lg font-semibold text-text-main">Add a podcast by RSS URL</h2>
        <form onSubmit={showPreview} className="flex flex-col gap-3 sm:flex-row">
          <label className="sr-only" htmlFor="podcast-feed-url">Podcast RSS URL</label>
          <input id="podcast-feed-url" className={inputClass} type="url" placeholder="https://publisher.example/podcast.xml"
            required autoComplete="url" value={url} onChange={event => { setURL(event.target.value); setPreview(null); }} />
          <button type="submit" disabled={working || !url.trim()} className="inline-flex min-w-36 items-center justify-center gap-2 rounded-lg bg-brand px-4 py-3 font-semibold text-surface-0 disabled:opacity-50">
            {working ? <Loader2 size={18} className="animate-spin" /> : <Rss size={18} />}
            Preview feed
          </button>
        </form>
        <p className="mt-3 text-xs text-text-subtle">Public HTTPS RSS feeds only during this preview. Private and legacy HTTP feeds will be supported after their security review.</p>
        {error && <p role="alert" className="mt-3 rounded-lg border border-error/30 p-3 text-sm text-error">{error}</p>}
        {notice && <p role="status" className="mt-3 text-sm text-brand">{notice}</p>}
        {preview && <div className="mt-5 rounded-lg border border-surface-3 bg-surface-1 p-4">
          <h3 className="text-lg font-semibold text-text-main">{preview.feed.title}</h3>
          <p className="text-sm text-text-secondary">{preview.feed.author || 'Independent publisher'} · {preview.episodeCount} episodes in the feed</p>
          <p className="mt-2 text-sm text-text-secondary">{preview.feed.description}</p>
          <button type="button" disabled={working} onClick={() => void subscribe()} className="mt-4 inline-flex items-center gap-2 rounded-lg bg-brand px-4 py-2 font-semibold text-surface-0 disabled:opacity-50">
            <Plus size={16}/> Follow podcast
          </button>
        </div>}
      </section>

      <section aria-label="Import podcasts from OPML" className="rounded-xl border border-surface-3 bg-surface-2 p-5">
        <h2 className="mb-2 text-lg font-semibold text-text-main">Import podcast subscriptions</h2>
        <p className="mb-3 text-sm text-text-secondary">Choose an OPML 2.0 file from another podcast app. Preview happens without contacting publishers; following each feed begins only when you confirm.</p>
        <label htmlFor="podcast-opml-file" className="mb-2 block text-sm font-semibold text-text-main">OPML file</label>
        <input id="podcast-opml-file" type="file" accept=".opml,.xml,application/xml,text/xml"
          disabled={importMode !== 'idle'} aria-label="Choose OPML file"
          onChange={event => {
            const file = event.target.files?.[0];
            event.target.value = '';
            void previewOPMLFile(file);
          }} className="block max-w-full text-sm text-text-secondary file:mr-3 file:rounded-md file:border file:border-surface-3 file:bg-surface-1 file:px-3 file:py-2 file:text-text-main" />
        {importMode === 'preview' && <p role="status" className="mt-3 text-sm text-text-secondary">Checking OPML file…</p>}
        {opml && <div className="mt-4 space-y-3 rounded-lg border border-surface-3 bg-surface-1 p-4">
          <p className="text-sm font-semibold text-text-main">{opml.feeds.length} supported feeds, {opml.feeds.filter(feed => feed.alreadyFollowed).length} already followed</p>
          {(opml.invalidCount > 0 || opml.duplicateCount > 0) &&
            <p className="text-xs text-text-secondary">{opml.invalidCount} unsupported or private feeds skipped; {opml.duplicateCount} duplicates skipped.</p>}
          <ul className="space-y-1 text-xs text-text-secondary">
            {opml.feeds.slice(0, 8).map(feed => <li key={feed.url}>{feed.title}{feed.alreadyFollowed ? ' · Following' : ''}</li>)}
          </ul>
          {opml.feeds.length > 8 && <p className="text-xs text-text-subtle">And {opml.feeds.length - 8} more feeds…</p>}
          <div className="flex flex-wrap items-center gap-3">
            <button type="button" disabled={importMode !== 'idle' || opml.feeds.every(feed => feed.alreadyFollowed)}
              onClick={() => void importOPMLFeeds()}
              className="rounded-md bg-brand px-4 py-2 text-sm font-semibold text-surface-0 disabled:opacity-50">
              Import {opml.feeds.filter(feed => !feed.alreadyFollowed).length} feed{opml.feeds.filter(feed => !feed.alreadyFollowed).length === 1 ? '' : 's'}
            </button>
            {importMode === 'import' && <button type="button" onClick={() => importController.current?.abort()}
              className="rounded-md border border-surface-3 px-3 py-2 text-sm text-text-main">Cancel import</button>}
            {importMode === 'import' && <span role="status" className="text-xs text-text-secondary">
              Processed {importDone} feeds…
            </span>}
          </div>
        </div>}
        {importSummary && <p role="status" className="mt-3 text-sm text-text-secondary">{importSummary}</p>}
        {importFailures.length > 0 && <div className="mt-3 text-xs text-text-secondary">
          <p className="font-semibold">Feeds that could not be followed</p>
          <ul className="mt-1 list-inside list-disc">{importFailures.map((message, index) => <li key={index}>{message}</li>)}</ul>
        </div>}
      </section>

      {continueEpisodes.length > 0 && <section aria-label="Continue listening">
        <h2 className="mb-3 text-xl font-bold text-text-main">Continue Listening</h2>
        <div className="grid gap-3 md:grid-cols-2">
          {continueEpisodes.map(episode => <Link key={episode.id} to={'/podcasts/show/' + encodeURIComponent(episode.feedId)}
            className="rounded-xl border border-surface-3 bg-surface-2 p-4 hover:bg-surface-highlight">
            <p className="text-xs font-semibold text-brand">{episode.feedTitle}</p>
            <h3 className="mt-1 font-semibold text-text-main">{episode.title}</h3>
            <p className="mt-1 text-xs text-text-secondary">Saved at {Math.floor(episode.positionMs / 60000)} min · View episode</p>
          </Link>)}
        </div>
      </section>}

      <section aria-label="Followed podcasts">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-xl font-semibold text-text-main">Following</h2>
          <div className="flex flex-1 items-center justify-end gap-2">
            <input type="search" aria-label="Search followed podcasts"
              placeholder="Search your podcasts" value={followingQuery}
              onChange={event => setFollowingQuery(event.target.value)}
              className="w-full max-w-xs rounded-md border border-surface-3 bg-surface-1 px-3 py-2 text-sm text-text-main focus-visible:outline-2 focus-visible:outline-brand" />
            <button type="button" onClick={() => void exportFollowing()} disabled={loading || feeds.length === 0 || exporting}
              className="inline-flex items-center gap-2 rounded-md border border-surface-3 px-3 py-2 text-sm font-semibold text-text-main hover:bg-surface-highlight disabled:opacity-50"
              aria-label="Export followed podcasts as OPML">
              <Download size={16} aria-hidden="true"/> {exporting ? 'Exporting…' : 'Export OPML'}
            </button>
            <button type="button" aria-label="Refresh subscribed podcasts" onClick={() => setRevision(value => value + 1)} className="rounded-md p-2 text-text-secondary hover:bg-surface-2"><RefreshCw size={18}/></button>
          </div>
        </div>
        {loading && <p role="status" className="text-text-secondary">Loading subscriptions…</p>}
        {!loading && feeds.length === 0 && <p className="rounded-xl border border-surface-3 bg-surface-2 p-8 text-center text-text-secondary">No podcasts followed yet. Add an RSS feed above to get started.</p>}
        {!loading && feeds.length > 0 && (
          <>
          {feeds.filter(feed => (feed.title + ' ' + feed.author).toLowerCase().includes(followingQuery.trim().toLowerCase())).length === 0 &&
            <p className="rounded-lg border border-surface-3 bg-surface-2 p-5 text-sm text-text-secondary">No followed podcasts match this search.</p>}
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {feeds.filter(feed => (feed.title + ' ' + feed.author).toLowerCase().includes(followingQuery.trim().toLowerCase())).map(feed => <Link key={feed.id} to={'/podcasts/show/' + encodeURIComponent(feed.id)}
              className="flex items-start gap-4 rounded-xl border border-surface-3 bg-surface-2 p-4 transition-colors hover:bg-surface-hover focus-visible:outline-2 focus-visible:outline-brand">
              <PodcastArtwork feedId={feed.id} title={feed.title} imageURL={feed.imageUrl}/>
              <span className="min-w-0">
                <span className="block truncate font-semibold text-text-main">{feed.title}</span>
                <span className="block truncate text-xs text-text-secondary">{feed.author || 'Independent publisher'}</span>
                <span className="mt-2 block text-xs text-text-subtle">{feed.episodeCount} available episodes</span>
              </span>
            </Link>)}
          </div>
          </>
        )}
      </section>
    </main>
  );
};

export const PodcastShow: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const [feed, setFeed] = useState<PodcastFeed | null>(null);
  const [episodes, setEpisodes] = useState<PodcastEpisode[]>([]);

  const [episodeFilter, setEpisodeFilter] = useState('');
  const [playedFilter, setPlayedFilter] = useState<'all' | 'played' | 'unplayed'>('all');
  const [updatingEpisode, setUpdatingEpisode] = useState<string | null>(null);
  const [nextOffset, setNextOffset] = useState<number | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [working, setWorking] = useState(false);
  const [refreshPending, setRefreshPending] = useState(false);
  const [refreshMessage, setRefreshMessage] = useState('');
  const [refreshStatus, setRefreshStatus] = useState<PodcastRefreshStatus | null>(null);
  const [autoRefreshEnabled, setAutoRefreshEnabled] = useState(false);
  const [autoRefreshSaving, setAutoRefreshSaving] = useState(false);
  const [revision, setRevision] = useState(0);
  const lastCheckedRef = useRef<number | null>(null);
  const navigate = useNavigate();

  useEffect(() => {
    let mounted = true;
    void api.getSetting('podcasts_preview_enabled')
      .then(value => { if (mounted) setAutoRefreshEnabled(value === 'true'); })
      .catch(() => { if (mounted) setAutoRefreshEnabled(false); });
    return () => { mounted = false; };
  }, []);

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
    ]).then(([details, result]) => {
      if (controller.signal.aborted) return;
      setFeed(details.feed);
      setEpisodes(result.episodes);
      setNextOffset(result.nextOffset ?? null);
    }).catch(err => {
      if (!controller.signal.aborted) { setFeed(null); setError(readableError(err)); }
    })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [id, revision]);

  useEffect(() => {
    if (!id) return;
    lastCheckedRef.current = null;
    const controller = new AbortController();
    const updateStatus = () => {
      void podcastsAPI.refreshStatus(id, controller.signal).then(status => {
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
      }).catch(() => { /* Episode browsing remains usable without status API. */ });
    };
    updateStatus();
    const timer = window.setInterval(updateStatus, 10000);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, [id]);

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
    if (updatingEpisode) return;
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
      setEpisodes(current => current.map(row => row.id === episode.id
        ? { ...row, isPlayed: saved.isPlayed, positionMs: saved.positionMs,
            listenedMs: saved.listenedMs, progressRevision: saved.revision }
        : row));
    } catch (err) {
      setError(readableError(err));
    } finally {
      setUpdatingEpisode(null);
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
  return <main className="mx-auto w-full max-w-5xl space-y-6 p-5 pb-28 sm:p-8">
    <Link to="/podcasts" className="inline-flex items-center gap-2 text-sm text-text-secondary hover:text-text-main"><ArrowLeft size={17}/> Back to Podcasts</Link>
    {loading && <p role="status" className="text-text-secondary">Loading episodes…</p>}
    {error && <p role="alert" className="rounded-lg border border-error/30 p-3 text-error">{error}</p>}
    {!loading && feed && <>
      <header className="flex items-start gap-5 rounded-xl border border-surface-3 bg-surface-2 p-6">
        <PodcastArtwork feedId={feed.id} title={feed.title} imageURL={feed.imageUrl} large/>
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-bold text-text-main">{feed.title}</h1>
          <p className="mt-1 text-sm text-text-secondary">{feed.author}</p>
          <p className="mt-3 text-sm text-text-secondary">{feed.description}</p>
          <div className="mt-4 flex flex-wrap items-center gap-3">
            <button type="button" onClick={() => void requestRefresh()} disabled={!autoRefreshEnabled || autoRefreshSaving || refreshPending || refreshStatus?.refreshing}
              className="inline-flex items-center gap-2 rounded-md border border-surface-3 px-3 py-2 text-sm font-semibold text-text-main hover:bg-surface-highlight disabled:opacity-50">
              <RefreshCw size={16} className={refreshPending || refreshStatus?.refreshing ? 'animate-spin' : ''}/>
              {refreshPending || refreshStatus?.refreshing ? 'Refreshing…' : 'Refresh feed'}
            </button>
            {refreshStatus?.lastSuccessAt ? <span className="text-xs text-text-subtle">Last updated {new Date(refreshStatus.lastSuccessAt).toLocaleString()}</span> : null}
          </div>
          <label className="mt-3 inline-flex cursor-pointer items-center gap-2 text-sm text-text-secondary">
            <input type="checkbox" checked={autoRefreshEnabled} disabled={autoRefreshSaving}
              onChange={event => void changeAutoRefresh(event.target.checked)} />
            Enable feed refresh (early access)
          </label>
          <p className="mt-1 text-xs text-text-subtle">Feed refreshing is optional. You can browse and follow RSS podcasts without enabling it.</p>
          {refreshMessage && <p role="status" className="mt-2 text-sm text-text-secondary">{refreshMessage}</p>}
          {!confirmRemove ? <button type="button" onClick={() => setConfirmRemove(true)} className="mt-4 inline-flex items-center gap-2 text-sm text-text-secondary hover:text-error"><Trash2 size={16}/> Unfollow</button>
            : <div className="mt-4 flex flex-wrap items-center gap-3 text-sm">
              <span className="text-text-secondary">Unfollow? Episode history is retained.</span>
              <button type="button" disabled={working} onClick={() => void unsubscribe()} className="rounded-md border border-error px-3 py-1 font-semibold text-error">Confirm</button>
              <button type="button" onClick={() => setConfirmRemove(false)} className="rounded-md border border-surface-3 px-3 py-1 text-text-secondary">Cancel</button>
            </div>}
        </div>
      </header>

      <section>
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-lg font-bold text-text-main">Episodes ({episodes.length} loaded)</h2>
          <label htmlFor="podcast-played-filter" className="text-sm text-text-secondary">Played status</label>
          <select id="podcast-played-filter" aria-label="Filter episodes by played status"
            value={playedFilter} onChange={event => setPlayedFilter(event.target.value as 'all' | 'played' | 'unplayed')}
            className="rounded-md border border-surface-3 bg-surface-1 px-3 py-2 text-sm text-text-main">
            <option value="all">All episodes</option>
            <option value="unplayed">Unplayed</option>
            <option value="played">Played</option>
          </select>
          <input type="search" aria-label="Filter loaded episodes" placeholder="Filter loaded episodes"
            value={episodeFilter} onChange={event => setEpisodeFilter(event.target.value)}
            className="w-full max-w-xs rounded-md border border-surface-3 bg-surface-1 px-3 py-2 text-sm text-text-main focus-visible:outline-2 focus-visible:outline-brand" />
        </div>
        <p className="mb-3 text-xs text-text-subtle">Episode filtering applies to loaded pages only. Load more to browse the full back catalog.</p>
        {episodes.length === 0 && <p className="text-text-secondary">No episodes are available in this feed.</p>}
        <div className="space-y-2">
          {episodes.filter(episode => (episode.title + ' ' + episode.description).toLowerCase().includes(episodeFilter.trim().toLowerCase()) &&
          (playedFilter === 'all' || (playedFilter === 'played') === Boolean(episode.isPlayed))).map(episode => <article key={episode.id} className="rounded-lg border border-surface-3 bg-surface-2 p-4">
            <h3 className="font-semibold text-text-main">{episode.title}</h3>
            <p className="mt-1 text-xs text-text-subtle">{episode.publishedAt ? new Date(episode.publishedAt).toLocaleDateString() : 'Date unavailable'}{episode.durationMs ? ` · ${Math.round(episode.durationMs / 60000)} min` : ''}</p>
            {episode.description && <p className="mt-2 line-clamp-3 text-sm text-text-secondary">{episode.description}</p>}
            <div className="mt-3 flex flex-wrap items-center gap-3">
              {episode.positionMs ? <span className="text-xs text-text-subtle">Position saved at {Math.floor(episode.positionMs / 60000)} min</span> : null}
              <button type="button" onClick={() => void setEpisodePlayed(episode)} disabled={updatingEpisode !== null}
                aria-label={episode.isPlayed ? `Mark ${episode.title} unplayed` : `Mark ${episode.title} played`}
                className="rounded-md border border-surface-3 px-3 py-1.5 text-xs font-semibold text-text-secondary hover:text-text-main disabled:opacity-50">
                {updatingEpisode === episode.id ? 'Saving…' : episode.isPlayed ? 'Mark unplayed' : 'Mark played'}
              </button>
              {episode.enclosureUrl && <button type="button"
                onClick={() => {
                  // The same persistent Player owns music, Plex and podcasts.
                  // The loaded show page supplies an episode queue without
                  // ever adding podcast items to the music catalog.
                  const queue = episodes.filter(item => Boolean(item.enclosureUrl))
                    .map(item => podcastEpisodeToTrack(item, feed));
                  const selected = queue.findIndex(item => item.podcastEpisodeId === episode.id);
                  if (selected < 0) return;
                  void useStore.getState().playSong(queue[selected], queue, 'podcast', selected);
                }}
                aria-label={'Listen to ' + episode.title}
                className="rounded-md bg-brand px-3 py-1.5 text-xs font-semibold text-surface-0">
                Listen
              </button>}
              <span className="text-xs text-text-subtle">Play in the ViiB music player. Offline downloads are planned.</span>
            </div>
          </article>)}
        </div>
        {nextOffset !== null && <button type="button" disabled={loadingMore}
          onClick={() => void loadMoreEpisodes()}
          className="mt-5 w-full rounded-lg border border-surface-3 bg-surface-2 px-4 py-3 text-sm font-semibold text-text-main hover:bg-surface-highlight disabled:opacity-50">
          {loadingMore ? 'Loading more episodes…' : 'Load more episodes'}
        </button>}
      </section>
    </>}
  </main>;
};
