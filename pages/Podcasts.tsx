
import React, { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { ArrowLeft, Headphones, Loader2, Plus, RefreshCw, Rss, Trash2 } from 'lucide-react';
import { podcastsAPI, type PodcastFeed, type PodcastEpisode, type PodcastPreview } from '../services/podcasts';

const inputClass = 'w-full rounded-lg border border-surface-3 bg-surface-1 px-4 py-3 text-text-main outline-none focus-visible:ring-2 focus-visible:ring-brand';

function readableError(error: unknown): string {
  return error instanceof Error ? error.message : 'The podcast service is unavailable.';
}

/** Early RSS subscription experience; deliberately has no fake player or download actions. */
export const Podcasts: React.FC = () => {
  const [feeds, setFeeds] = useState<PodcastFeed[]>([]);
  const [url, setURL] = useState('');
  const [preview, setPreview] = useState<PodcastPreview | null>(null);
  const [loading, setLoading] = useState(true);
  const [working, setWorking] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [revision, setRevision] = useState(0);
  const navigate = useNavigate();

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
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
      const result = await podcastsAPI.subscribe(preview.feed.url);
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
  }, [preview, navigate]);

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

      <section aria-label="Followed podcasts">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-xl font-semibold text-text-main">Following</h2>
          <button type="button" aria-label="Refresh subscribed podcasts" onClick={() => setRevision(value => value + 1)} className="rounded-md p-2 text-text-secondary hover:bg-surface-2"><RefreshCw size={18}/></button>
        </div>
        {loading && <p role="status" className="text-text-secondary">Loading subscriptions…</p>}
        {!loading && feeds.length === 0 && <p className="rounded-xl border border-surface-3 bg-surface-2 p-8 text-center text-text-secondary">No podcasts followed yet. Add an RSS feed above to get started.</p>}
        {!loading && feeds.length > 0 && (
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {feeds.map(feed => <Link key={feed.id} to={'/podcasts/show/' + encodeURIComponent(feed.id)}
              className="flex items-start gap-4 rounded-xl border border-surface-3 bg-surface-2 p-4 transition-colors hover:bg-surface-hover focus-visible:outline-2 focus-visible:outline-brand">
              <span className="flex size-14 shrink-0 items-center justify-center rounded-lg bg-surface-highlight text-brand"><Headphones size={25}/></span>
              <span className="min-w-0">
                <span className="block truncate font-semibold text-text-main">{feed.title}</span>
                <span className="block truncate text-xs text-text-secondary">{feed.author || 'Independent publisher'}</span>
                <span className="mt-2 block text-xs text-text-subtle">{feed.episodeCount} available episodes</span>
              </span>
            </Link>)}
          </div>
        )}
      </section>
    </main>
  );
};

export const PodcastShow: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const [feed, setFeed] = useState<PodcastFeed | null>(null);
  const [episodes, setEpisodes] = useState<PodcastEpisode[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [working, setWorking] = useState(false);
  const navigate = useNavigate();
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    void Promise.all([
      podcastsAPI.listSubscriptions(controller.signal),
      podcastsAPI.listEpisodes(id ?? '', controller.signal),
    ]).then(([followed, result]) => {
      if (controller.signal.aborted) return;
      setFeed(followed.feeds.find(item => item.id === id) ?? null);
      setEpisodes(result.episodes);
    }).catch(err => { if (!controller.signal.aborted) setError(readableError(err)); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [id]);

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
        <span className="flex size-20 shrink-0 items-center justify-center rounded-xl bg-surface-highlight text-brand"><Headphones size={40}/></span>
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-bold text-text-main">{feed.title}</h1>
          <p className="mt-1 text-sm text-text-secondary">{feed.author}</p>
          <p className="mt-3 text-sm text-text-secondary">{feed.description}</p>
          {!confirmRemove ? <button type="button" onClick={() => setConfirmRemove(true)} className="mt-4 inline-flex items-center gap-2 text-sm text-text-secondary hover:text-error"><Trash2 size={16}/> Unfollow</button>
            : <div className="mt-4 flex flex-wrap items-center gap-3 text-sm">
              <span className="text-text-secondary">Unfollow? Episode history is retained.</span>
              <button type="button" disabled={working} onClick={() => void unsubscribe()} className="rounded-md border border-error px-3 py-1 font-semibold text-error">Confirm</button>
              <button type="button" onClick={() => setConfirmRemove(false)} className="rounded-md border border-surface-3 px-3 py-1 text-text-secondary">Cancel</button>
            </div>}
        </div>
      </header>
      <section>
        <h2 className="mb-4 text-lg font-bold text-text-main">Episodes ({episodes.length})</h2>
        {episodes.length === 0 && <p className="text-text-secondary">No episodes are available in this feed.</p>}
        <div className="space-y-2">
          {episodes.map(episode => <article key={episode.id} className="rounded-lg border border-surface-3 bg-surface-2 p-4">
            <h3 className="font-semibold text-text-main">{episode.title}</h3>
            <p className="mt-1 text-xs text-text-subtle">{episode.publishedAt ? new Date(episode.publishedAt).toLocaleDateString() : 'Date unavailable'}{episode.durationMs ? ` · ${Math.round(episode.durationMs / 60000)} min` : ''}</p>
            {episode.description && <p className="mt-2 line-clamp-3 text-sm text-text-secondary">{episode.description}</p>}
            <p className="mt-2 text-xs text-text-subtle">Streaming and downloads are coming in a later phase.</p>
          </article>)}
        </div>
      </section>
    </>}
  </main>;
};
