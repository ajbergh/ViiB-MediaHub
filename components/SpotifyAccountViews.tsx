import { useEffect, useState } from 'react';
import { Link } from 'react-router';
import { SpotifyRateLimitError } from '../lib/spotifyErrors';
import { backendSpotifyFetch } from '../services/spotifyBackend';
import { Button } from './ui/Button';

const views = [
    ['liked', 'Liked Songs', '/me/tracks'],
    ['top-tracks', 'Top Tracks', '/me/top/tracks'],
    ['top-artists', 'Top Artists', '/me/top/artists'],
    ['devices', 'Connect Devices', '/me/player/devices'],
    ['queue', 'Spotify Queue', '/me/player/queue'],
] as const;

type Entry = {id?: string; name: string; type?: string; is_active?: boolean; typeLabel?: string};
type AccountResponse = {
    items?: Array<Entry | {track: Entry | null}>;
    devices?: Entry[];
    queue?: Entry[];
    currently_playing?: Entry | null;
    next?: string | null;
    offset?: number;
    limit?: number;
};

export function SpotifyAccountViews({generation}: {generation: number}) {
    const [view, setView] = useState<(typeof views)[number][0]>('liked');
    const [range, setRange] = useState('medium_term');
    const [offset, setOffset] = useState(0);
    const [retry, setRetry] = useState(0);
    const [page, setPage] = useState<AccountResponse | null>(null);
    const [error, setError] = useState('');
    const isTop = view === 'top-tracks' || view === 'top-artists';
    const [, label, path] = views.find(entry => entry[0] === view)!;

    useEffect(() => {
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
    }, [generation, isTop, offset, path, range, retry, view]);

    const entries = view === 'devices' ? page?.devices
        : view === 'queue' ? page?.queue
        : page?.items?.map(item => item && 'track' in item ? item.track : item);
    return <section className="space-y-4" aria-labelledby="spotify-account-view">
        <div className="flex flex-wrap items-center gap-4">
            <label className="flex items-center gap-2">
                View
                <select className="rounded bg-surface-1 p-2 text-text-main" value={view} onChange={event => {
                    const next = views.find(entry => entry[0] === event.target.value);
                    if (next) { setPage(null); setOffset(0); setView(next[0]); }
                }}>
                    {views.map(([id, name]) => <option key={id} value={id}>{name}</option>)}
                </select>
            </label>
            {isTop && <label className="flex items-center gap-2">
                Time range
                <select className="rounded bg-surface-1 p-2 text-text-main" value={range} onChange={event => {
                    setPage(null); setOffset(0); setRange(event.target.value);
                }}>
                    <option value="short_term">Last 4 weeks</option>
                    <option value="medium_term">Last 6 months</option>
                    <option value="long_term">Last year</option>
                </select>
            </label>}
            <Button onClick={() => setRetry(value => value + 1)}>Refresh</Button>
        </div>
        <h2 id="spotify-account-view" className="text-lg font-semibold">{label}</h2>
        {error ? <p role="alert">{error}</p> : !page ? <p role="status">Loading {label}…</p> : <>
            {view === 'queue' && page.currently_playing && <p>Now playing: {page.currently_playing.name}</p>}
            {entries?.length ? <ol className="divide-y divide-surface-border">
                {entries.map((entry, index) => <li key={`${index}:${entry?.id ?? ''}`} className="px-3 py-2">
                    {view === 'top-artists' && entry?.id ? <Link to={`/spotify/artist/${entry.id}`} className="text-brand">{entry.name}</Link>
                        : entry?.id && view !== 'devices' && /^[A-Za-z0-9]{22}$/.test(entry.id)
                            ? <a className="text-brand" href={`https://open.spotify.com/${entry.type === 'episode' ? 'episode' : 'track'}/${entry.id}`} target="_blank" rel="noopener noreferrer">{entry.name}</a>
                            : entry?.name ?? 'Unavailable item'}
                    {view === 'devices' && entry?.is_active && <span className="ml-2 text-text-secondary">Active</span>}
                </li>)}
            </ol> : <p className="text-text-secondary">No items to show.</p>}
            {(offset > 0 || page.next) && <div className="flex gap-2">
                <Button disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 20))}>Previous page</Button>
                <Button disabled={!page.next} onClick={() => setOffset(offset + 20)}>Next page</Button>
            </div>}
        </>}
    </section>;
}
