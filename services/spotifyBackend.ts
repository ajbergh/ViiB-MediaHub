/** Sends validated resource paths to the backend and fences requests and responses against session changes. */

import { SpotifyAuthError, SpotifyRateLimitError } from '../lib/spotifyErrors';
import { useStore } from '../store';

export class SpotifySessionChangedError extends Error {
    constructor() { super('Spotify session changed'); this.name = 'SpotifySessionChangedError'; }
}
export function assertSpotifySession(generation: number): void {
    const state = useStore.getState();
    if (state.spotifySessionGeneration !== generation) throw new SpotifySessionChangedError();
    if (!state.spotifyConnected) throw new SpotifyAuthError('Reconnect to Spotify', 401);
}

/** Renderer requests contain only resource paths; the backend owns bearer tokens. */
export async function backendSpotifyFetch(resource: string, signal?: AbortSignal): Promise<Response> {
    if (!resource.startsWith('/') || resource.startsWith('//') || resource.includes('..') || resource.includes('\\')) {
        throw new Error('Invalid Spotify resource');
    }
    const generation = useStore.getState().spotifySessionGeneration;
    assertSpotifySession(generation);
    const url = new URL(resource, 'https://resource.invalid');
    const query = new URLSearchParams(url.search);
    query.set('path', url.pathname.slice(1));
    const controller = new AbortController();
    const abort = () => controller.abort();
    signal?.throwIfAborted();
    signal?.addEventListener('abort', abort, {once: true});
    const unsubscribe = useStore.subscribe(state => {
        if (state.spotifySessionGeneration !== generation) controller.abort();
    });
    let response: Response;
    try {
        response = await fetch('/api/spotify/proxy?' + query.toString(), {signal: controller.signal});
    } catch (error) {
        unsubscribe();
        signal?.removeEventListener('abort', abort);
        throw error;
    }
    const cleanup = () => {
        unsubscribe();
        signal?.removeEventListener('abort', abort);
    };
    if (!response.ok) cleanup();
    if (useStore.getState().spotifySessionGeneration !== generation || signal?.aborted) {
        cleanup();
        assertSpotifySession(generation);
        signal?.throwIfAborted();
    }
    assertSpotifySession(generation);
    if (response.status === 401) {
        useStore.getState().markSpotifyAuthRequired();
        throw new SpotifyAuthError('Reconnect to Spotify', 401);
    }
    if (response.status === 429) {
        const seconds = Number(response.headers.get('Retry-After'));
        throw new SpotifyRateLimitError('Spotify is temporarily rate limited', Number.isFinite(seconds) && seconds > 0 ? seconds : 60);
    }
    // Headers can arrive before account replacement and the body afterwards.
    const readJSON = response.json.bind(response);
    response.json = async () => {
        try {
            signal?.throwIfAborted();
            assertSpotifySession(generation);
            const value = await readJSON();
            signal?.throwIfAborted();
            assertSpotifySession(generation);
            return value;
        } finally { cleanup(); }
    };
    return response;
}
