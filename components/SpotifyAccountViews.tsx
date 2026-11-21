/** Displays Spotify account views available to the active session. */

import { useEffect, useState } from 'react';
import { Link } from 'react-router';
import { SpotifyRateLimitError } from '../lib/spotifyErrors';
import { backendSpotifyFetch } from '../services/spotifyBackend';
import { SpotifyArtistLinks } from './SpotifyArtistLinks';
import { Button } from './ui/Button';

const views = [
    ['liked', 'Liked Songs', '/me/tracks'],
    ['top-tracks', 'Top Tracks', '/me/top/tracks'],
    ['top-artists', 'Top Artists', '/me/top/artists'],
    ['devices', 'Connect Devices', '/me/player/devices'],
    ['queue', 'Spotify Queue', '/me/player/queue'],
] as const;

type Entry = {
    id?: string;
    name: string;
    type?: string;
    is_active?: boolean;
    typeLabel?: string;
    artists?: Array<{ id?: string; name: string }>;
    album?: { id?: string; name?: string };
    genres?: string[];
    followers?: { total?: number };
};
type AccountResponse = {
    items?: Array<Entry | {track: Entry | null}>;
    devices?: Entry[];
    queue?: Entry[];
    currently_playing?: Entry | null;
    next?: string | null;
    offset?: number;
    limit?: number;
};

/** Loads the selected account view while guarding against changes to the active session. */
export function SpotifyAccountViews({generation}: {generation: number}) {
    const [oauthViews, setOAuthViews] = useState<boolean | 'unavailable' | null>(null);
    const [view, setView] = useState<(typeof views)[number][0]>('liked');
    const [range, setRange] = useState('medium_term');
    const [offset, setOffset] = useState(0);
    const [retry, setRetry] = useState(0);
    const [page, setPage] = useState<AccountResponse | null>(null);
    const [error, setError] = useState('');
    const canRead = view === 'liked' || oauthViews === true;
    const isTop = view === 'top-tracks' || view === 'top-artists';
    const [, label, path] = views.find(entry => entry[0] === view)!;

    useEffect(() => {
        const controller = new AbortController();
        void fetch('/api/spotify/account', {signal: controller.signal}).then(async response => {
            if (!response.ok) throw new Error('Could not load Spotify capabilities');
            const capabilities = await response.json();
            if (!controller.signal.aborted) setOAuthViews(capabilities.oauthViews === true);
        }).catch(() => {
            if (!controller.signal.aborted) setOAuthViews('unavailable');
        });
        return () => controller.abort();
    }, [generation, retry]);

    useEffect(() => {
        if (!canRead) return;
        const controller = new AbortController();
        setPage(null);
        setError('');
        const params = new URLSearchParams({offset: String(offset)});
        if (isTop) params.set('time_range', range);
        async function read() {
            try {
                const response = await backendSpotifyFetch(path + '?' + params, controller.signal, view);
                if (response.status === 403 || response.status === 404) {
                    setError('Spotify does not expose this view to the current connection. Check your Spotify permissions or open the Spotify app.');
                    return;
                }
                if (!response.ok) throw new Error('Spotify request failed');
                const result = response.status === 204 ? {} : await response.json() as AccountResponse;
                if (!controller.signal.aborted) setPage(result);
            } catch (failure) {
                if (controller.signal.aborted) return;
                setError(failure instanceof SpotifyRateLimitError
                    ? `Spotify is rate limiting requests. Retry in ${failure.retryAfter} seconds.`
                    : 'Could not load this Spotify view.');
            }
        }
        void read();
        return () => controller.abort();
    }, [canRead, generation, isTop, offset, path, range, retry, view]);

    const entries = view === 'devices' ? page?.devices
        : view === 'queue' ? page?.queue
        : page?.items?.map(item => item && 'track' in item ? item.track : item);
    return <section className="space-y-4" aria-labelledby="spotify-account-view">
        <div className="flex flex-wrap items-center gap-4">
            <label className="flex items-center gap-2 text-sm text-text-secondary">
                View
                <select className="border border-surface-border rounded-lg px-3 py-1.5 text-sm bg-surface-1 text-text-main" value={view} onChange={event => {
                    const next = views.find(entry => entry[0] === event.target.value);
                    if (next && (next[0] === 'liked' || oauthViews === true)) { setPage(null); setOffset(0); setView(next[0]); }
                }}>
                    {views.map(([id, name]) => <option key={id} value={id} disabled={id !== 'liked' && oauthViews !== true}>{name}</option>)}
                </select>
            </label>
            {isTop && <label className="flex items-center gap-2 text-sm text-text-secondary">
                Time range
                <select className="border border-surface-border rounded-lg px-3 py-1.5 text-sm bg-surface-1 text-text-main" value={range} onChange={event => {
                    setPage(null); setOffset(0); setRange(event.target.value);
                }}>
                    <option value="short_term">Last 4 weeks</option>
                    <option value="medium_term">Last 6 months</option>
                    <option value="long_term">Last year</option>
                </select>
            </label>}
            <Button onClick={() => setRetry(value => value + 1)}>Refresh</Button>
        </div>
        {oauthViews === 'unavailable' && <p role="status" className="text-text-secondary">Could not check Spotify account features. Try Refresh.</p>}
        {oauthViews === false && <p className="text-text-secondary">Top items, Connect devices, and the Spotify queue require an OAuth connection.</p>}
        <h2 id="spotify-account-view" className="text-lg font-semibold">{label}</h2>
        {!canRead ? null : error ? <p role="alert">{error}</p> : !page ? <p role="status">Loading {label}…</p> : <>
            {view === 'queue' && page.currently_playing && <p>Now playing: {page.currently_playing.name}</p>}
            {entries?.length ? <ol className="divide-y divide-surface-border rounded-xl bg-surface-1 overflow-hidden">
                {entries.map((entry, index) => {
                    const subtitle = entry?.artists?.length ? (
                        <>
                            <SpotifyArtistLinks artists={entry.artists} />
                            {entry.album?.name ? ` · ${entry.album.name}` : ''}
                        </>
                    ) : view === 'top-artists' ? (
                        entry?.genres?.length
                            ? entry.genres.slice(0, 2).join(', ')
                            : entry?.followers?.total != null
                              ? `${entry.followers.total.toLocaleString()} followers`
                              : null
                    ) : view === 'devices' ? (
                        entry?.typeLabel || entry?.type || null
                    ) : null;

                    return (
                        <li key={`${index}:${entry?.id ?? ''}`} className="flex items-center gap-3 px-3 py-2 min-h-[36px] hover:bg-surface-hover transition-colors">
                            <span className="w-6 shrink-0 text-right text-xs font-mono text-text-subtle">
                                {offset + index + 1}
                            </span>
                            <div className="min-w-0 flex-1">
                                <div className="truncate font-medium text-text-main">
                                    {view === 'top-artists' && entry?.id ? (
                                        <Link to={`/spotify/artist/${entry.id}`} className="text-brand hover:underline">{entry.name}</Link>
                                    ) : entry?.id && view !== 'devices' && /^[A-Za-z0-9]{22}$/.test(entry.id) ? (
                                        <a className="text-brand hover:underline" href={`https://open.spotify.com/${entry.type === 'episode' ? 'episode' : 'track'}/${entry.id}`} target="_blank" rel="noopener noreferrer">{entry.name}</a>
                                    ) : (
                                        entry?.name ?? 'Unavailable item'
                                    )}
                                </div>
                                {subtitle && (
                                    <div className="truncate text-xs text-text-secondary">
                                        {subtitle}
                                    </div>
                                )}
                            </div>
                            {view === 'devices' && entry?.is_active && (
                                <span className="shrink-0 rounded-full bg-brand/15 px-2.5 py-0.5 text-xs font-semibold text-brand">Active</span>
                            )}
                        </li>
                    );
                })}
            </ol> : <p className="text-text-secondary">No items to show.</p>}
            {(offset > 0 || page.next) && <div className="flex gap-2">
                <Button disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 20))}>Previous page</Button>
                <Button disabled={!page.next} onClick={() => setOffset(offset + 20)}>Next page</Button>
            </div>}
        </>}
    </section>;
}
