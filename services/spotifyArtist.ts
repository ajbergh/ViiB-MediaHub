/** Loads artist profile, top tracks, and bounded discography through the session-fenced backend client. */
import { backendSpotifyFetch } from './spotifyBackend';

export interface SpotifyArtistSummary {
    id: string;
    name: string;
}
export interface SpotifyArtistProfile extends SpotifyArtistSummary {
    images: Array<{ url: string }>;
    followers: { total: number } | null;
}
export interface SpotifyArtistTrack {
    id: string;
    name: string;
    duration_ms: number;
    artists: SpotifyArtistSummary[];
    album: { id: string; name: string; images: Array<{ url: string }> };
}
export interface SpotifyArtistRelease {
    id: string;
    name: string;
    album_type: string;
    release_date: string | null;
    images: Array<{ url: string }>;
}
export interface SpotifyArtistDiscography {
    items: Array<SpotifyArtistRelease | null>;
    truncated?: boolean;
}
async function read<T>(
    id: string,
    suffix: string,
    signal: AbortSignal,
): Promise<T> {
    if (!/^[A-Za-z0-9]{22}$/.test(id))
        throw new Error('Invalid Spotify artist');
    const response = await backendSpotifyFetch(
        '/artists/' + id + suffix,
        signal,
    );
    if (!response.ok)
        throw new Error(
            response.status === 404
                ? 'Artist is unavailable on Spotify.'
                : 'Unable to load artist information. Please try again.',
        );
    return response.json() as Promise<T>;
}
export const spotifyArtist = {
    profile: (id: string, signal: AbortSignal) =>
        read<SpotifyArtistProfile>(id, '', signal),
    topTracks: (id: string, signal: AbortSignal) =>
        read<{ tracks: SpotifyArtistTrack[] }>(id, '/top-tracks', signal),
    releases: (id: string, signal: AbortSignal) =>
        read<SpotifyArtistDiscography>(id, '/albums?limit=100', signal),
};
