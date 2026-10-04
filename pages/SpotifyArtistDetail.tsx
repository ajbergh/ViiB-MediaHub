/** Browses Spotify artist top tracks and bounded discography with account-isolated loading and playback actions. */
import React, { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router';
import {
    ArrowLeft,
    Download,
    ExternalLink,
    ListPlus,
    Loader2,
    Mic2,
    Play,
    Shuffle,
} from 'lucide-react';
import { useStore } from '../store';
import {
    spotifyArtist,
    type SpotifyArtistProfile,
    type SpotifyArtistTrack,
    type SpotifyArtistDiscography,
} from '../services/spotifyArtist';
import { assertSpotifySession } from '../services/spotifyBackend';
import { spotifyTracksToSongs } from '../lib/spotifyHelpers';
import { api } from '../services/api';
import { SpotifyArtistLinks } from '../components/SpotifyArtistLinks';
import { SpotifySessionConnect } from '../components/SpotifySessionConnect';
import { Button } from '../components/ui/Button';
import { formatTime } from '../utils';

type Loaded<T> = { data?: T; error?: string; loading: boolean };
type ArtistView = {
    id: string;
    generation: number;
    profile: Loaded<SpotifyArtistProfile>;
    top: Loaded<SpotifyArtistTrack[]>;
    releases: Loaded<SpotifyArtistDiscography>;
};
const pending = { loading: true };
const message = (error: unknown) =>
    error instanceof Error
        ? error.message
        : 'Unable to load artist information.';

export const SpotifyArtistDetail: React.FC = () => {
    const { id = '' } = useParams<{ id: string }>();
    const connected = useStore((state) => state.spotifyConnected);
    const generation = useStore((state) => state.spotifySessionGeneration);
    const { playSong, addToQueue, showToast } = useStore();
    const [retry, setRetry] = useState(0);
    const [view, setView] = useState<ArtistView>({
        id,
        generation,
        profile: pending,
        top: pending,
        releases: pending,
    });
    const [downloads, setDownloads] = useState<Set<string>>(new Set());
    useEffect(() => {
        const controller = new AbortController();
        setView({
            id,
            generation,
            profile: pending,
            top: pending,
            releases: pending,
        });
        setDownloads(new Set());
        if (!connected) return () => controller.abort();
        const current = () =>
            !controller.signal.aborted &&
            useStore.getState().spotifySessionGeneration === generation;
        const load = <T,>(
            key: 'profile' | 'top' | 'releases',
            request: Promise<T>,
        ) => {
            void request
                .then((data) => {
                    if (current())
                        setView((previous) => ({
                            ...previous,
                            [key]: { data, loading: false },
                        }));
                })
                .catch((error) => {
                    if (current())
                        setView((previous) => ({
                            ...previous,
                            [key]: { error: message(error), loading: false },
                        }));
                });
        };
        load('profile', spotifyArtist.profile(id, controller.signal));
        load(
            'top',
            spotifyArtist
                .topTracks(id, controller.signal)
                .then((value) => value.tracks),
        );
        load('releases', spotifyArtist.releases(id, controller.signal));
        return () => controller.abort();
    }, [id, connected, generation, retry]);
    const current = view.id === id && view.generation === generation;
    const artist = current ? view.profile.data : undefined;
    const tracks = current ? view.top.data || [] : [];
    const releases = current
        ? view.releases.data?.items.filter((item) => item?.id) || []
        : [];
    const songs = spotifyTracksToSongs(tracks);
    const play = (index = 0, shuffle = false) => {
        assertSpotifySession(generation);
        if (!songs.length) return;
        const context = [...songs];
        if (shuffle)
            for (let i = context.length - 1; i > 0; i--) {
                const j = Math.floor(Math.random() * (i + 1));
                [context[i], context[j]] = [context[j], context[i]];
            }
        playSong(context[shuffle ? 0 : index], context);
    };
    const queue = (index?: number) => {
        assertSpotifySession(generation);
        const selection = index === undefined ? songs : [songs[index]];
        if (!selection.length) return;
        addToQueue(selection);
        showToast({
            type: 'success',
            message: `Added ${selection.length} ${selection.length === 1 ? 'track' : 'tracks'} to queue`,
        });
    };
    const download = async (track: SpotifyArtistTrack) => {
        if (downloads.has(track.id)) return;
        setDownloads((previous) => new Set(previous).add(track.id));
        try {
            assertSpotifySession(generation);
            await api.downloadTrack(
                track.id,
                track.name,
                track.artists.map((artist) => artist.name).join(', '),
                track.album.name,
                Math.floor(track.duration_ms / 1000),
            );
            assertSpotifySession(generation);
            showToast({
                type: 'success',
                message: `Queued for download: ${track.name}`,
            });
        } catch (error) {
            if (useStore.getState().spotifySessionGeneration === generation)
                showToast({ type: 'error', message: message(error) });
        } finally {
            if (useStore.getState().spotifySessionGeneration === generation)
                setDownloads((previous) => {
                    const next = new Set(previous);
                    next.delete(track.id);
                    return next;
                });
        }
    };
    const failure = (text: string) => (
        <div role="alert" className="text-error py-4">
            {text}{' '}
            <Button
                variant="secondary"
                onClick={() => setRetry((value) => value + 1)}
            >
                Retry
            </Button>
        </div>
    );
    const releaseSection = (title: string, types: string[]) => {
        const items = releases.filter((item) =>
            types.includes(item.album_type),
        );
        if (!items.length) return null;
        return (
            <section className="mt-8" aria-label={title}>
                <h2 className="text-section font-bold mb-4">{title}</h2>
                <div className="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-5 gap-4">
                    {items.map((album) => (
                        <Link
                            key={album.id}
                            to={
                                '/spotify/album/' + encodeURIComponent(album.id)
                            }
                            className="bg-surface-1 hover:bg-surface-2 rounded-lg p-4 transition-colors"
                        >
                            {album.images[0]?.url ? (
                                <img
                                    src={album.images[0].url}
                                    alt=""
                                    className="aspect-square w-full object-cover rounded-lg mb-3"
                                />
                            ) : (
                                <div className="aspect-square bg-surface-3 rounded-lg mb-3 flex items-center justify-center">
                                    <Mic2 size={40} />
                                </div>
                            )}
                            <h3 className="font-bold truncate">{album.name}</h3>
                            <p className="text-sm text-text-secondary">
                                {album.release_date?.slice(0, 4)}
                                {album.release_date && ' · '}
                                {album.album_type === 'ep'
                                    ? 'EP'
                                    : album.album_type === 'single'
                                      ? 'Single'
                                      : album.album_type === 'compilation'
                                        ? 'Compilation'
                                        : 'Album'}
                            </p>
                        </Link>
                    ))}
                </div>
            </section>
        );
    };
    return (
        <div className="p-8 pb-32 text-text-main">
            <Link
                to="/spotify"
                className="inline-flex items-center gap-2 text-text-secondary hover:text-text-main mb-6"
            >
                <ArrowLeft size={18} />
                Back to Spotify search
            </Link>
            {!connected ? (
                <SpotifySessionConnect />
            ) : (
                <>
                    <header className="flex flex-col sm:flex-row gap-6 items-start sm:items-end mb-6">
                        {artist?.images[0]?.url ? (
                            <img
                                src={artist.images[0].url}
                                alt={artist.name}
                                className="w-48 h-48 object-cover rounded-full"
                            />
                        ) : (
                            <div className="w-48 h-48 rounded-full bg-surface-2 flex items-center justify-center">
                                <Mic2 size={64} />
                            </div>
                        )}
                        <div>
                            <p className="text-text-secondary">Artist</p>
                            <h1 className="text-display font-bold">
                                {artist?.name || 'Spotify artist'}
                            </h1>
                            {artist?.followers && (
                                <p className="text-text-secondary mt-2">
                                    {artist.followers.total.toLocaleString()}{' '}
                                    followers
                                </p>
                            )}
                            {(!current || view.profile.loading) && (
                                <p
                                    role="status"
                                    className="text-text-secondary"
                                >
                                    Loading artist…
                                </p>
                            )}
                            {current &&
                                view.profile.error &&
                                failure(view.profile.error)}
                        </div>
                    </header>
                    <div className="flex flex-wrap items-center gap-3 mb-6">
                        <Button
                            variant="primary"
                            accent="brand"
                            disabled={!songs.length}
                            onClick={() => play()}
                            leftIcon={<Play size={18} />}
                        >
                            Play top tracks
                        </Button>
                        <Button
                            variant="secondary"
                            disabled={!songs.length}
                            onClick={() => play(0, true)}
                            aria-label="Shuffle top tracks"
                        >
                            <Shuffle size={18} />
                        </Button>
                        <Button
                            variant="secondary"
                            disabled={!songs.length}
                            onClick={() => queue()}
                            aria-label="Queue top tracks"
                        >
                            <ListPlus size={18} />
                        </Button>
                        <a
                            href={
                                'https://open.spotify.com/artist/' +
                                encodeURIComponent(id)
                            }
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex items-center gap-2 text-text-secondary hover:underline"
                        >
                            Open in Spotify
                            <ExternalLink size={16} />
                        </a>
                    </div>
                    <section aria-label="Top tracks">
                        <h2 className="text-section font-bold mb-4">
                            Top tracks
                        </h2>
                        {(!current || view.top.loading) && (
                            <p
                                role="status"
                                className="flex gap-2 text-text-secondary"
                            >
                                <Loader2 size={18} className="animate-spin" />
                                Loading top tracks…
                            </p>
                        )}
                        {current && view.top.error && failure(view.top.error)}
                        {current &&
                            !view.top.loading &&
                            !view.top.error &&
                            !tracks.length && (
                                <p className="text-text-secondary">
                                    No top tracks available.
                                </p>
                            )}
                        {tracks.map((track, index) => (
                            <div
                                key={track.id + ':' + index}
                                className="flex items-center gap-4 p-3 rounded-lg hover:bg-surface-1"
                            >
                                <Button
                                    variant="ghost"
                                    aria-label={'Play ' + track.name}
                                    onClick={() => play(index)}
                                >
                                    <Play size={18} />
                                </Button>
                                <div className="flex-1 min-w-0">
                                    <button
                                        className="font-medium text-left hover:underline truncate max-w-full"
                                        onClick={() => play(index)}
                                    >
                                        {track.name}
                                    </button>
                                    <div className="text-sm text-text-secondary">
                                        <SpotifyArtistLinks
                                            artists={track.artists}
                                        />
                                    </div>
                                </div>
                                <Link
                                    to={
                                        '/spotify/album/' +
                                        encodeURIComponent(track.album.id)
                                    }
                                    className="hidden md:block text-text-secondary hover:underline truncate max-w-48"
                                >
                                    {track.album.name}
                                </Link>
                                <span className="text-text-secondary text-sm">
                                    {formatTime(track.duration_ms / 1000)}
                                </span>
                                <Button
                                    variant="ghost"
                                    aria-label={'Queue ' + track.name}
                                    onClick={() => queue(index)}
                                >
                                    <ListPlus size={18} />
                                </Button>
                                <Button
                                    variant="ghost"
                                    aria-label={'Download ' + track.name}
                                    disabled={downloads.has(track.id)}
                                    onClick={() => void download(track)}
                                >
                                    {downloads.has(track.id) ? (
                                        <Loader2
                                            size={18}
                                            className="animate-spin"
                                        />
                                    ) : (
                                        <Download size={18} />
                                    )}
                                </Button>
                            </div>
                        ))}
                    </section>
                    {(!current || view.releases.loading) && (
                        <p role="status" className="text-text-secondary mt-8">
                            Loading albums and singles…
                        </p>
                    )}
                    {current &&
                        view.releases.error &&
                        failure(view.releases.error)}
                    {releaseSection('Albums', ['album', 'compilation'])}
                    {releaseSection('Singles and EPs', ['single', 'ep'])}
                    {current &&
                        !view.releases.loading &&
                        !view.releases.error &&
                        !releases.length && (
                            <p className="text-text-secondary mt-8">
                                No releases available.
                            </p>
                        )}
                    {current && view.releases.data?.truncated && (
                        <p className="text-text-secondary mt-6">
                            Showing the first 100 release groups.{' '}
                            <a
                                href={
                                    'https://open.spotify.com/artist/' +
                                    encodeURIComponent(id) +
                                    '/discography/all'
                                }
                                target="_blank"
                                rel="noopener noreferrer"
                                className="underline"
                            >
                                See all releases in Spotify
                            </a>
                            .
                        </p>
                    )}
                </>
            )}
        </div>
    );
};
