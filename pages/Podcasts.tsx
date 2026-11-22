
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { ArrowLeft, Download, Headphones, Loader2, Plus, RefreshCw, Rss, Trash2 } from 'lucide-react';
import { api } from '../services/api';
import { podcastEpisodeToTrack, queuePodcastEpisode, type PodcastQueuePosition } from '../lib/podcastPlayback';
import { useStore } from '../store';
import { podcastsAPI, PodcastAPIError, type PodcastFeed, type PodcastEpisode, type PodcastPreview, type PodcastRefreshStatus, type PodcastOPMLImportPreview, type PodcastContinueEpisode, type PodcastDownload, type PodcastStorage, type PodcastAutoDownloadPolicy, type PodcastShowPlaybackSettings } from '../services/podcasts';

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

/** Public RSS subscriptions, shared playback and early-access offline downloads. */
function queueEpisodeAction(episode: PodcastEpisode, feed: PodcastFeed, position: PodcastQueuePosition) {
  const player = useStore.getState();
  if (!queuePodcastEpisode(episode, feed, position, player)) return;
  player.showToast({
    type: 'success',
    message: player.currentSong
      ? (position === 'next' ? 'Episode added to Play Next.' : 'Episode added to the queue.')
      : 'Started podcast playback in the shared player.',
    duration: 3500,
  });
}

export const Podcasts: React.FC = () => {
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
  const selectedPodcastId = useStore(state => state.currentSong?.podcastEpisodeId);
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

  // Central Downloads view reads only subscription-scoped, path-free local
  // state. Background activity stays in Go, independent of this component.
  useEffect(() => {
    const controller = new AbortController();
    const refresh = () => {
      void podcastsAPI.listDownloads(controller.signal)
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
    const timer = window.setInterval(refresh, 5000);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, [revision]);

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
        const { download } = action === 'cancel'
          ? await podcastsAPI.cancelDownload(row.episodeId)
          : await podcastsAPI.queueDownload(row.episodeId);
        setDownloads(current => current.map(item => item.episodeId === row.episodeId
          ? { ...item, ...download } : item));
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

  const filteredDownloads = downloads.filter(row =>
    downloadsFilter === 'all'
    || (downloadsFilter === 'active' && (row.status === 'queued' || row.status === 'downloading' || row.status === 'paused'))
    || (downloadsFilter === 'completed' && row.status === 'completed')
    || (downloadsFilter === 'attention' && ['failed', 'canceled', 'paused'].includes(row.status)));

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

  const getContinueFeed = (episode: PodcastContinueEpisode): PodcastFeed =>
    feeds.find(item => item.id === episode.feedId) ?? {
      id: episode.feedId, url: '', title: episode.feedTitle, author: '',
      description: '', imageUrl: '', language: '', episodeCount: 0,
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
        <p className="mt-2 max-w-3xl text-text-secondary">Follow public RSS podcasts, use the shared ViiB player, manage offline downloads, and resume where you left off. Directory discovery and advanced download policies are still under development.</p>
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
          {continueEpisodes.map(episode => <article key={episode.id}
            className="rounded-xl border border-surface-3 bg-surface-2 p-4">
            <p className="text-xs font-semibold text-brand">{episode.feedTitle}</p>
            <h3 className="mt-1 font-semibold text-text-main">{episode.title}</h3>
            <p className="mt-1 text-xs text-text-secondary">Saved at {Math.floor(episode.positionMs / 60000)} min</p>
            <div className="mt-3 flex flex-wrap items-center gap-3">
              {episode.hasAudio ? <button type="button" onClick={() => resumeEpisode(episode)}
                aria-label={'Resume ' + episode.title}
                className="rounded-md bg-brand px-3 py-1.5 text-xs font-semibold text-surface-0">
                Resume playback
              </button> : <span className="text-xs text-text-subtle">Audio unavailable</span>}
              {episode.hasAudio && <>
                <button type="button" onClick={() => queueEpisodeAction(episode, getContinueFeed(episode), 'next')}
                  aria-label={'Play ' + episode.title + ' next'}
                  className="rounded-md border border-surface-3 px-3 py-1.5 text-xs font-semibold text-text-secondary hover:text-text-main">
                  Play Next
                </button>
                <button type="button" onClick={() => queueEpisodeAction(episode, getContinueFeed(episode), 'end')}
                  aria-label={'Add ' + episode.title + ' to queue'}
                  className="rounded-md border border-surface-3 px-3 py-1.5 text-xs font-semibold text-text-secondary hover:text-text-main">
                  Add to Queue
                </button>
              </>}
              <Link to={'/podcasts/show/' + encodeURIComponent(episode.feedId)}
                className="text-xs font-semibold text-text-secondary hover:text-text-main">
                View show
              </Link>
            </div>
          </article>)}
        </div>
      </section>}

      <section aria-label="Podcast downloads" className="rounded-xl border border-surface-3 bg-surface-2 p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-xl font-semibold text-text-main">Downloads</h2>
            <p className="mt-1 text-xs text-text-secondary">
              Recent download activity from followed podcasts (up to 100). Offline copies play through your shared player.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <label htmlFor="podcast-download-filter" className="text-xs text-text-subtle">Show</label>
            <select id="podcast-download-filter" aria-label="Filter podcast downloads" value={downloadsFilter}
              onChange={event => { setDownloadsFilter(event.target.value as typeof downloadsFilter); setConfirmRemoveDownloadId(null); }}
              className="rounded-md border border-surface-3 bg-surface-1 px-3 py-2 text-sm text-text-main">
              <option value="all">All</option>
              <option value="active">In progress</option>
              <option value="completed">Available offline</option>
              <option value="attention">Needs attention</option>
            </select>
            <button type="button" onClick={() => setRevision(value => value + 1)}
              aria-label="Refresh podcast downloads" className="rounded-md p-2 text-text-secondary hover:bg-surface-highlight">
              <RefreshCw size={17}/>
            </button>
          </div>
        </div>
        {downloadStorage && <p className="mt-2 text-xs text-text-subtle">
          {(downloadStorage.usedBytes / (1024 * 1024)).toFixed(0)} MiB of
          {' '}{(downloadStorage.maxBytes / (1024 * 1024 * 1024)).toFixed(1)} GiB reserved
        </p>}
        {downloadsError && <p role="alert" className="mt-3 text-sm text-error">{downloadsError}</p>}
        {downloadsLoading && <p role="status" className="mt-3 text-sm text-text-secondary">Loading podcast downloads…</p>}
        {!downloadsLoading && filteredDownloads.length === 0 && <p className="mt-4 text-sm text-text-secondary">
          {downloads.length ? 'No downloads match this filter.' : 'No podcast downloads yet. Open a followed show to save an episode for offline listening.'}
        </p>}
        {filteredDownloads.length > 0 && <div className="mt-4 space-y-2">
          {filteredDownloads.map(row => {
            const isActive = row.status === 'queued' || row.status === 'downloading' || row.status === 'paused';
            const completed = row.status === 'completed';
            const playableElsewhere = selectedPodcastId === row.episodeId;
            const progress = row.expectedBytes ? Math.min(100, Math.floor(row.bytesReceived * 100 / row.expectedBytes)) : null;
            const name = row.episodeTitle || 'Podcast episode';
            return <article key={row.episodeId} className="rounded-lg border border-surface-3 bg-surface-1 p-4">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <p className="text-xs font-semibold text-brand">{row.feedTitle || 'Followed show'}</p>
                  <h3 className="mt-1 break-words text-sm font-semibold text-text-main">{name}</h3>
                  <p className="mt-1 text-xs text-text-secondary">
                    {completed ? 'Available offline' : row.status === 'downloading'
                      ? `Downloading${progress === null ? '' : ' ' + progress + '%'}`
                      : row.status === 'paused' && row.retryAfter
                      ? `Retry scheduled for ${new Date(row.retryAfter).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}`
                      : row.status === 'queued' ? 'Queued'
                      : row.status === 'failed' ? `Failed: ${row.errorCode || 'unavailable'}`
                      : row.status === 'canceled' ? 'Canceled' : 'Paused'}
                    {completed && row.verifiedSize > 0 ? ` · ${(row.verifiedSize / (1024 * 1024)).toFixed(1)} MiB` : ''}
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  {row.feedId && <Link to={'/podcasts/show/' + encodeURIComponent(row.feedId)}
                    className="rounded-md border border-surface-3 px-3 py-1.5 text-xs font-semibold text-text-secondary hover:text-text-main">
                    View show
                  </Link>}
                  {isActive ? <button type="button" disabled={workingDownloadId !== null}
                    onClick={() => void changeLibraryDownload(row, 'cancel')}
                    aria-label={'Cancel download of ' + name}
                    className="rounded-md border border-surface-3 px-3 py-1.5 text-xs font-semibold text-text-main disabled:opacity-50">
                    Cancel
                  </button> : completed ? <button type="button" disabled={workingDownloadId !== null || playableElsewhere}
                    onClick={() => setConfirmRemoveDownloadId(row.episodeId)}
                    aria-label={'Remove offline download of ' + name}
                    className="rounded-md border border-surface-3 px-3 py-1.5 text-xs font-semibold text-text-main disabled:opacity-50">
                    Remove download
                  </button> : <button type="button" disabled={workingDownloadId !== null}
                    onClick={() => void changeLibraryDownload(row, 'retry')}
                    aria-label={'Retry download of ' + name}
                    className="rounded-md border border-surface-3 px-3 py-1.5 text-xs font-semibold text-text-main disabled:opacity-50">
                    Retry
                  </button>}
                </div>
              </div>
              {completed && playableElsewhere && <p className="mt-2 text-xs text-text-subtle">Switch away from this episode to remove its offline audio.</p>}
              {completed && confirmRemoveDownloadId === row.episodeId && <div role="group"
                aria-label={'Confirm removing download of ' + name}
                className="mt-3 flex flex-wrap items-center gap-2 text-xs text-text-secondary">
                <span>Remove offline audio? Your listening history will remain.</span>
                <button type="button" disabled={workingDownloadId !== null}
                  onClick={() => void changeLibraryDownload(row, 'remove')}
                  aria-label={'Confirm remove download of ' + name}
                  className="rounded-md bg-brand px-3 py-1.5 font-semibold text-surface-0">Remove copy</button>
                <button type="button" onClick={() => setConfirmRemoveDownloadId(null)}
                  className="rounded-md border border-surface-3 px-3 py-1.5">Keep</button>
              </div>}
            </article>;
          })}
        </div>}
      </section>

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
  const [statusFilter, setStatusFilter] = useState<'all' | 'played' | 'unplayed' | 'in_progress' | 'not_started'>('all');
  const [updatingEpisode, setUpdatingEpisode] = useState<string | null>(null);
  const [downloads, setDownloads] = useState<Record<string, PodcastDownload>>({});
  const [downloadStorage, setDownloadStorage] = useState<PodcastStorage | null>(null);
  const [confirmDeleteDownload, setConfirmDeleteDownload] = useState<string | null>(null);
  const selectedPodcastId = useStore(state => state.currentSong?.podcastEpisodeId);
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
  const [autoRefreshEnabled, setAutoRefreshEnabled] = useState(false);
  const [autoRefreshSaving, setAutoRefreshSaving] = useState(false);
  const [autoDownload, setAutoDownload] = useState<PodcastAutoDownloadPolicy | null>(null);
  const [autoDownloadSaving, setAutoDownloadSaving] = useState(false);
  const [autoDownloadMessage, setAutoDownloadMessage] = useState('');
  // A settings request for show A must not overwrite show B after navigation.
  const autoDownloadFeedRef = useRef(id);
  autoDownloadFeedRef.current = id;
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

  useEffect(() => {
    let mounted = true;
    void api.getSetting('podcasts_preview_enabled')
      .then(value => { if (mounted) setAutoRefreshEnabled(value === 'true'); })
      .catch(() => { if (mounted) setAutoRefreshEnabled(false); });
    return () => { mounted = false; };
  }, []);

  useEffect(() => {
    if (!id) return;
    const controller = new AbortController();
    setShowPlayback(null);
    setSkipIntroInput('0');
    setSkipOutroInput('0');
    setShowPlaybackSaving(false);
    setShowPlaybackMessage('');
    void podcastsAPI.getShowPlaybackSettings(id, controller.signal)
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
      setFeed(current => current ? {
        ...current,
        defaultSpeed: settings.defaultSpeed,
        skipIntroSeconds: settings.skipIntroSeconds,
        skipOutroSeconds: settings.skipOutroSeconds,
      } : current);
      window.dispatchEvent(new CustomEvent('viib:podcast-show-settings-changed', {
        detail: { feedId: feedID, settings },
      }));
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
    void podcastsAPI.getAutoDownloadPolicy(id, controller.signal)
      .then(({ policy }) => { if (!controller.signal.aborted) setAutoDownload(policy); })
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
      setAutoDownloadMessage(updated.policy.enabled
        ? 'Automatic downloads enabled for new episodes discovered on future feed refreshes.'
        : 'Automatic downloads turned off. Existing downloads are unchanged.');
    } catch (err) {
      if (autoDownloadFeedRef.current !== feedID) return;
      setAutoDownload(previous);
      setAutoDownloadMessage(readableError(err));
    } finally {
      if (autoDownloadFeedRef.current === feedID) setAutoDownloadSaving(false);
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

  // Download status is polled through one bounded subscription-scoped list.
  // Never fetch or render remote signed enclosure URLs in the browser.
  useEffect(() => {
    if (!id) return;
    const controller = new AbortController();
    const poll = () => {
      void podcastsAPI.listDownloads(controller.signal)
        .then(({ downloads: rows, storage }) => {
          if (controller.signal.aborted) return;
          setDownloads(Object.fromEntries(rows.map(row => [row.episodeId, row])));
          if (storage) setDownloadStorage(storage);
        })
        .catch(() => { /* Streaming remains available on older backend builds. */ });
    };
    poll();
    const timer = window.setInterval(poll, 3500);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, [id]);

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
        const { download } = action === 'cancel'
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

  const applyBulkPlayed = async () => {
    if (bulkTarget === null || bulkRunning || updatingEpisode) return;
    const controller = new AbortController();
    bulkController.current = controller;
    const target = bulkTarget;
    const candidates = episodes.filter(episode => Boolean(episode.isPlayed) !== target);
    let updated = 0, failed = 0, processed = 0;
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
            const saved = await podcastsAPI.setProgress(episode.id, {
              positionMs: progress.positionMs,
              listenedMs: progress.listenedMs,
              isPlayed: target,
              expectedRevision: progress.revision,
            }, controller.signal);
            setEpisodes(current => current.map(item => item.id === episode.id
              ? { ...item, isPlayed: saved.isPlayed, positionMs: saved.positionMs,
                  listenedMs: saved.listenedMs, progressRevision: saved.revision }
              : item));
            updated++;
          } else {
            // Another session already applied the requested state.
            setEpisodes(current => current.map(item => item.id === episode.id
              ? { ...item, isPlayed: progress.isPlayed, positionMs: progress.positionMs,
                  listenedMs: progress.listenedMs, progressRevision: progress.revision }
              : item));
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
      setBulkSummary(`Updated ${updated} of ${candidates.length} loaded episodes; ${failed} failed.${controller.signal.aborted ? ' Stopped early.' : ''}`);
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
      .toLowerCase().includes(episodeFilter.trim().toLowerCase());
    return matchesText && (
      statusFilter === 'all' ||
      (statusFilter === 'played' && played) ||
      (statusFilter === 'unplayed' && !played) ||
      (statusFilter === 'in_progress' && !played && hasPosition) ||
      (statusFilter === 'not_started' && !played && !hasPosition)
    );
  });
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
          <div className="mt-4 rounded-lg border border-surface-3 bg-surface-1 p-4">
            <h2 className="text-sm font-semibold text-text-main">Automatic episode downloads</h2>
            <label className="mt-2 inline-flex items-center gap-2 text-sm text-text-secondary">
              <input type="checkbox" aria-label="Automatically download new podcast episodes"
                checked={autoDownload?.enabled ?? false}
                disabled={!autoDownload || autoDownloadSaving}
                onChange={event => void saveAutoDownload({
                  enabled: event.target.checked, newestCount: autoDownload?.newestCount ?? 1,
                })} />
              Download newly discovered episodes for this show
            </label>
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <label htmlFor="podcast-auto-download-count" className="text-xs text-text-secondary">
                Maximum new episodes per refresh
              </label>
              <select id="podcast-auto-download-count" aria-label="Maximum auto-download episodes per refresh"
                value={autoDownload?.newestCount ?? 1}
                disabled={!autoDownload || autoDownloadSaving}
                onChange={event => void saveAutoDownload({
                  enabled: autoDownload?.enabled ?? false, newestCount: Number(event.target.value),
                })}
                className="rounded-md border border-surface-3 bg-surface-2 px-2 py-1.5 text-sm text-text-main">
                <option value={1}>1 episode</option>
                <option value={2}>2 episodes</option>
                <option value={3}>3 episodes</option>
              </select>
            </div>
            <p className="mt-2 text-xs text-text-subtle">
              Off by default. Only episodes first discovered on future successful RSS refreshes are eligible.
              No old episodes are downloaded automatically. A global limit of 12 pending downloads applies.
            </p>
            {!autoRefreshEnabled && <p className="mt-2 text-xs text-text-subtle">
              Feed refresh is currently off. Enable it above for automatic downloads to run.
            </p>}
            {autoDownloadMessage && <p role="status" className="mt-2 text-xs text-text-secondary">{autoDownloadMessage}</p>}
          </div>
          <form onSubmit={event => {
            event.preventDefault();
            if (!showPlayback) return;
            const parsedIntro = Math.max(0, Math.min(600, Math.trunc(Number(skipIntroInput) || 0)));
            const parsedOutro = Math.max(0, Math.min(600, Math.trunc(Number(skipOutroInput) || 0)));
            void saveShowPlayback({
              defaultSpeed: showPlayback.defaultSpeed,
              skipIntroSeconds: parsedIntro,
              skipOutroSeconds: parsedOutro,
            });
          }} className="mt-4 rounded-lg border border-surface-3 bg-surface-1 p-4">
            <h2 className="text-sm font-semibold text-text-main">Show playback defaults</h2>
            <div className="mt-3 flex flex-wrap items-end gap-4">
              <div>
                <label htmlFor="podcast-show-default-speed" className="block text-xs text-text-secondary">
                  Default playback speed
                </label>
                <select id="podcast-show-default-speed" aria-label="Show default playback speed"
                  value={showPlayback?.defaultSpeed ?? 0}
                  disabled={!showPlayback || showPlaybackSaving}
                  onChange={event => {
                    if (!showPlayback) return;
                    void saveShowPlayback({
                      ...showPlayback,
                      defaultSpeed: Number(event.target.value),
                    });
                  }}
                  className="mt-1 rounded-md border border-surface-3 bg-surface-2 px-2 py-1.5 text-sm text-text-main">
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
                <label htmlFor="podcast-show-skip-intro" className="block text-xs text-text-secondary">
                  Skip intro (seconds)
                </label>
                <input id="podcast-show-skip-intro" type="number" min={0} max={600} step={5}
                  aria-label="Show skip intro seconds"
                  value={skipIntroInput}
                  disabled={!showPlayback || showPlaybackSaving}
                  onChange={event => setSkipIntroInput(event.target.value)}
                  className="mt-1 w-24 rounded-md border border-surface-3 bg-surface-2 px-2 py-1.5 text-sm text-text-main" />
              </div>
              <div>
                <label htmlFor="podcast-show-skip-outro" className="block text-xs text-text-secondary">
                  Skip outro (seconds)
                </label>
                <input id="podcast-show-skip-outro" type="number" min={0} max={600} step={5}
                  aria-label="Show skip outro seconds"
                  value={skipOutroInput}
                  disabled={!showPlayback || showPlaybackSaving}
                  onChange={event => setSkipOutroInput(event.target.value)}
                  className="mt-1 w-24 rounded-md border border-surface-3 bg-surface-2 px-2 py-1.5 text-sm text-text-main" />
              </div>
              <button type="submit" disabled={!showPlayback || showPlaybackSaving}
                className="rounded-md border border-surface-3 bg-surface-2 px-3 py-1.5 text-sm font-semibold text-text-main hover:bg-surface-highlight disabled:opacity-50">
                Save skip timings
              </button>
            </div>
            <p className="mt-2 text-xs text-text-subtle">
              Applies when starting episodes of this show in the shared player. Saved resume positions are not overwritten by skip intro.
            </p>
            {showPlaybackMessage && <p role="status" className="mt-2 text-xs text-text-secondary">{showPlaybackMessage}</p>}
          </form>
          {!confirmRemove ? <button type="button" onClick={() => setConfirmRemove(true)} className="mt-4 inline-flex items-center gap-2 text-sm text-text-secondary hover:text-error"><Trash2 size={16}/> Unfollow</button>
            : <div className="mt-4 flex flex-wrap items-center gap-3 text-sm">
              <span className="text-text-secondary">Unfollow? Episode history is retained.</span>
              <button type="button" disabled={working} onClick={() => void unsubscribe()} className="rounded-md border border-error px-3 py-1 font-semibold text-error">Confirm</button>
              <button type="button" onClick={() => setConfirmRemove(false)} className="rounded-md border border-surface-3 px-3 py-1 text-text-secondary">Cancel</button>
            </div>}
        </div>
      </header>

      <section>
        {downloadError && <p role="alert" className="mb-3 rounded-lg border border-error/30 p-3 text-sm text-error">{downloadError}</p>}
        {downloadStorage && <p className="mb-3 text-xs text-text-subtle">
          Offline podcast storage: {(downloadStorage.usedBytes / (1024 * 1024)).toFixed(0)} MiB
          {' / '}{(downloadStorage.maxBytes / (1024 * 1024 * 1024)).toFixed(1)} GiB reserved
        </p>}
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-lg font-bold text-text-main">Episodes ({episodes.length} loaded)</h2>
          <div aria-label="Loaded episode bulk actions" className="flex flex-wrap items-center gap-2">
            <button type="button" disabled={bulkRunning || updatingEpisode !== null || episodes.length === 0}
              onClick={() => { setBulkTarget(true); setBulkSummary(''); }}
              aria-label="Mark loaded episodes played"
              className="rounded-md border border-surface-3 px-3 py-2 text-xs font-semibold text-text-secondary disabled:opacity-50">
              Mark loaded played
            </button>
            <button type="button" disabled={bulkRunning || updatingEpisode !== null || episodes.length === 0}
              onClick={() => { setBulkTarget(false); setBulkSummary(''); }}
              aria-label="Mark loaded episodes unplayed"
              className="rounded-md border border-surface-3 px-3 py-2 text-xs font-semibold text-text-secondary disabled:opacity-50">
              Mark loaded unplayed
            </button>
          </div>
          <label htmlFor="podcast-status-filter" className="text-sm text-text-secondary">Episode status</label>
          <select id="podcast-status-filter" aria-label="Filter episodes by status"
            value={statusFilter} onChange={event => setStatusFilter(event.target.value as typeof statusFilter)}
            className="rounded-md border border-surface-3 bg-surface-1 px-3 py-2 text-sm text-text-main">
            <option value="all">All episodes</option>
            <option value="unplayed">Unplayed</option>
            <option value="in_progress">In progress</option>
            <option value="not_started">Not started</option>
            <option value="played">Played</option>
          </select>
          <input type="search" aria-label="Filter loaded episodes" placeholder="Filter loaded episodes"
            value={episodeFilter} onChange={event => setEpisodeFilter(event.target.value)}
            className="w-full max-w-xs rounded-md border border-surface-3 bg-surface-1 px-3 py-2 text-sm text-text-main focus-visible:outline-2 focus-visible:outline-brand" />
        </div>
        <p className="mb-3 text-xs text-text-subtle">Episode filtering and bulk played controls apply only to loaded pages. Load more to browse the full back catalog.</p>
        {bulkTarget !== null && !bulkRunning && <div role="group" aria-label="Confirm bulk episode change"
          className="mb-3 flex flex-wrap items-center gap-3 rounded-lg border border-surface-3 bg-surface-1 p-3 text-sm text-text-secondary">
          <span>Mark up to {episodes.filter(row => Boolean(row.isPlayed) !== bulkTarget).length} loaded episodes {bulkTarget ? 'played' : 'unplayed'}? Saved positions will be kept.</span>
          <button type="button" onClick={() => void applyBulkPlayed()}
            aria-label={'Confirm mark loaded ' + (bulkTarget ? 'played' : 'unplayed')}
            className="rounded-md bg-brand px-3 py-1.5 font-semibold text-surface-0">Confirm</button>
          <button type="button" onClick={() => setBulkTarget(null)}
            className="rounded-md border border-surface-3 px-3 py-1.5">Cancel</button>
        </div>}
        {bulkRunning && <div role="status" className="mb-3 flex items-center gap-3 text-sm text-text-secondary">
          Processed {bulkProcessed} loaded episodes.
          <button type="button" onClick={() => bulkController.current?.abort()}
            className="rounded-md border border-surface-3 px-3 py-1.5">Stop bulk update</button>
        </div>}
        {bulkSummary && <p role="status" className="mb-3 text-sm text-text-secondary">{bulkSummary}</p>}
        {episodes.length === 0 && <p className="text-text-secondary">No episodes are available in this feed.</p>}
        {episodes.length > 0 && <p role="status" className="mb-3 text-xs text-text-subtle">
          Showing {visibleEpisodes.length} of {episodes.length} loaded episodes.
        </p>}
        {episodes.length > 0 && visibleEpisodes.length === 0 && <p className="mb-3 text-sm text-text-secondary">
          No loaded episodes match these filters. Clear the filters or load more episodes.
        </p>}
        <div className="space-y-2">
          {visibleEpisodes.map(episode => <article key={episode.id} className="rounded-lg border border-surface-3 bg-surface-2 p-4">
            <h3 className="font-semibold text-text-main">{episode.title}</h3>
            <p className="mt-1 text-xs text-text-subtle">{episode.publishedAt ? new Date(episode.publishedAt).toLocaleDateString() : 'Date unavailable'}{episode.durationMs ? ` · ${Math.round(episode.durationMs / 60000)} min` : ''}</p>
            {episode.description && <p className="mt-2 line-clamp-3 text-sm text-text-secondary">{episode.description}</p>}
            <div className="mt-3 flex flex-wrap items-center gap-3">
              {episode.positionMs ? <span className="text-xs text-text-subtle">Position saved at {Math.floor(episode.positionMs / 60000)} min</span> : null}
              <button type="button" onClick={() => void setEpisodePlayed(episode)} disabled={updatingEpisode !== null || bulkRunning}
                aria-label={episode.isPlayed ? `Mark ${episode.title} unplayed` : `Mark ${episode.title} played`}
                className="rounded-md border border-surface-3 px-3 py-1.5 text-xs font-semibold text-text-secondary hover:text-text-main disabled:opacity-50">
                {updatingEpisode === episode.id ? 'Saving…' : episode.isPlayed ? 'Mark unplayed' : 'Mark played'}
              </button>
              {episode.hasAudio && <button type="button"
                onClick={() => {
                  // The same persistent Player owns music, Plex and podcasts.
                  // The loaded show page supplies an episode queue without
                  // ever adding podcast items to the music catalog.
                  const queue = episodes.filter(item => Boolean(item.hasAudio))
                    .map(item => podcastEpisodeToTrack(item, feed));
                  const selected = queue.findIndex(item => item.podcastEpisodeId === episode.id);
                  if (selected < 0) return;
                  void useStore.getState().playSong(queue[selected], queue, 'podcast', selected);
                }}
                aria-label={'Listen to ' + episode.title}
                className="rounded-md bg-brand px-3 py-1.5 text-xs font-semibold text-surface-0">
                Listen
              </button>}
              {episode.hasAudio && <>
                <button type="button"
                  onClick={() => queueEpisodeAction(episode, feed, 'next')}
                  aria-label={'Play ' + episode.title + ' next'}
                  className="rounded-md border border-surface-3 px-3 py-1.5 text-xs font-semibold text-text-secondary hover:text-text-main">
                  Play Next
                </button>
                <button type="button"
                  onClick={() => queueEpisodeAction(episode, feed, 'end')}
                  aria-label={'Add ' + episode.title + ' to queue'}
                  className="rounded-md border border-surface-3 px-3 py-1.5 text-xs font-semibold text-text-secondary hover:text-text-main">
                  Add to Queue
                </button>
              </>}
              {episode.hasAudio && (() => {
                const status = downloads[episode.id];
                const active = status?.status === 'queued' || status?.status === 'downloading' || status?.status === 'paused';
                const completed = status?.status === 'completed';
                const label = active ? 'Cancel download' :
                  status?.status === 'failed' || status?.status === 'canceled' ? 'Retry download' : 'Download episode';
                const percent = status?.expectedBytes ? Math.min(100,
                  Math.floor(100 * status.bytesReceived / status.expectedBytes)) : null;
                return <div className="flex flex-wrap items-center gap-2">
                  {completed ? <>
                    <button type="button" disabled={workingDownload !== null || selectedPodcastId === episode.id}
                      onClick={() => setConfirmDeleteDownload(episode.id)}
                      aria-label={'Remove offline copy of ' + episode.title}
                      className="rounded-md border border-surface-3 px-3 py-1.5 text-xs font-semibold text-text-secondary disabled:opacity-50">
                      Remove download
                    </button>
                    {selectedPodcastId === episode.id && <span className="text-xs text-text-subtle">Switch away before removing</span>}
                    {confirmDeleteDownload === episode.id && <span role="group" aria-label={'Confirm remove download ' + episode.title}
                      className="flex items-center gap-2 text-xs text-text-secondary">
                      <span>Remove offline audio? Listening history remains.</span>
                      <button type="button" disabled={workingDownload !== null}
                        onClick={() => void changeDownload(episode, 'remove')}
                        aria-label={'Confirm remove offline copy of ' + episode.title}
                        className="rounded-md bg-brand px-2 py-1.5 font-semibold text-surface-0">Remove copy</button>
                      <button type="button" onClick={() => setConfirmDeleteDownload(null)}
                        className="rounded-md border border-surface-3 px-2 py-1.5">Keep</button>
                    </span>}
                  </> : <button type="button" disabled={workingDownload !== null}
                    onClick={() => void changeDownload(episode, active ? 'cancel' : 'queue')}
                    aria-label={label + ' ' + episode.title}
                    className="inline-flex items-center gap-1.5 rounded-md border border-surface-3 px-3 py-1.5 text-xs font-semibold text-text-secondary hover:text-text-main disabled:opacity-50">
                    <Download size={14} aria-hidden="true" />
                    {workingDownload === episode.id ? 'Working…' : label}
                  </button>}
                  {status && <span role="status" className="text-xs text-text-subtle">
                    {completed ? 'Available offline' : status.status === 'downloading'
                      ? `Downloading${percent === null ? '' : ' ' + percent + '%'}`
                      : status.status === 'failed' ? `Download failed: ${status.errorCode || 'unavailable'}`
                      : status.status === 'canceled' ? 'Download canceled'
                      : status.status === 'paused' && status.retryAfter
                        ? `Retry scheduled for ${new Date(status.retryAfter).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}`
                      : 'Download queued'}
                  </span>}
                </div>;
              })()}
              <span className="text-xs text-text-subtle">Uses the shared ViiB music/Plex player and queue.</span>
            </div>
          </article>)}
        </div>
        {nextOffset !== null && <button type="button" disabled={loadingMore || bulkRunning}
          onClick={() => void loadMoreEpisodes()}
          className="mt-5 w-full rounded-lg border border-surface-3 bg-surface-2 px-4 py-3 text-sm font-semibold text-text-main hover:bg-surface-highlight disabled:opacity-50">
          {loadingMore ? 'Loading more episodes…' : 'Load more episodes'}
        </button>}
      </section>
    </>}
  </main>;
};
