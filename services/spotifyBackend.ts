/** Sends validated resource paths to the backend and fences requests and responses against session changes. */

import { SpotifyAuthError, SpotifyRateLimitError } from '../lib/spotifyErrors';
import { useStore } from '../store';

let cooldownUntil = 0;
let cooldownGeneration = -1;

/** Remaining shared provider cooldown, in milliseconds. The backend remains authoritative. */
export function spotifyCooldownRemaining(): number {
    return cooldownGeneration === useStore.getState().spotifySessionGeneration
        ? Math.max(0, cooldownUntil - Date.now()) : 0;
}

export class SpotifySessionChangedError extends Error {
    constructor() { super('Spotify session changed'); this.name = 'SpotifySessionChangedError'; }
}

/** Handles cancellation on WebKit versions that lack AbortSignal.throwIfAborted. */
export function throwIfSpotifyRequestAborted(signal?: AbortSignal): void {
    if (!signal?.aborted) return;
    throw signal.reason === undefined ? new DOMException('The operation was aborted.', 'AbortError') : signal.reason;
}
export function assertSpotifySession(generation: number): void {
    const state = useStore.getState();
    if (state.spotifySessionGeneration !== generation) throw new SpotifySessionChangedError();
    if (!state.spotifyConnected) throw new SpotifyAuthError('Reconnect to Spotify', 401);
}

/** Renderer requests contain only resource paths; the backend owns bearer tokens. */
export async function backendSpotifyFetch(resource: string, signal?: AbortSignal, accountView?: string): Promise<Response> {
    if (!resource.startsWith('/') || resource.startsWith('//') || resource.includes('..') || resource.includes('\\')) {
        throw new Error('Invalid Spotify resource');
    }
    const generation = useStore.getState().spotifySessionGeneration;
    assertSpotifySession(generation);
    throwIfSpotifyRequestAborted(signal);
    const remaining = spotifyCooldownRemaining();
    if (remaining > 0) {
        throw new SpotifyRateLimitError('Spotify is temporarily rate limited', Math.ceil(remaining / 1000));
    }
    const url = new URL(resource, 'https://resource.invalid');
    const query = new URLSearchParams(url.search);
    query.set('path', url.pathname.slice(1));
    const controller = new AbortController();
    const abort = () => controller.abort();
    throwIfSpotifyRequestAborted(signal);
    signal?.addEventListener('abort', abort, {once: true});
    const unsubscribe = useStore.subscribe(state => {
        if (state.spotifySessionGeneration !== generation) controller.abort();
    });
    let response: Response;
    try {
        response = await fetch((accountView ? '/api/spotify/account/' + encodeURIComponent(accountView) : '/api/spotify/proxy') + '?' + query.toString(), {signal: controller.signal});
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
        throwIfSpotifyRequestAborted(signal);
    }
    assertSpotifySession(generation);
    if (response.status === 401) {
        useStore.getState().markSpotifyAuthRequired();
        throw new SpotifyAuthError('Reconnect to Spotify', 401);
    }
    if (response.status === 429) {
        const header = response.headers.get('Retry-After');
        const seconds = header?.trim() ? Number(header) : NaN;
        const duration = Number.isFinite(seconds) && seconds >= 0
            ? Math.max(1, seconds) * 1000
            : Math.max(1000, Date.parse(header || '') - Date.now()) || 60000;
        cooldownUntil = Math.max(spotifyCooldownRemaining() + Date.now(), Date.now() + duration);
        cooldownGeneration = generation;
        throw new SpotifyRateLimitError('Spotify is temporarily rate limited', Math.ceil(spotifyCooldownRemaining() / 1000));
    }
    // Headers can arrive before account replacement and the body afterwards.
    const readJSON = response.json.bind(response);
    response.json = async () => {
        try {
            throwIfSpotifyRequestAborted(signal);
            assertSpotifySession(generation);
            const value = await readJSON();
            throwIfSpotifyRequestAborted(signal);
            assertSpotifySession(generation);
            return value;
        } finally { cleanup(); }
    };
    return response;
}
