// @vitest-environment jsdom
/** Tests Spotify browsing, search, paging, and account/session changes. */

import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { SpotifySearchOptions } from '../services/spotifyService';
import { useStore } from '../store';
import { Spotify } from './Spotify';

const mocks = vi.hoisted(() => ({
    search: vi.fn(), profile: vi.fn(), status: vi.fn(), songs: vi.fn(),
    recent: vi.fn(), albums: vi.fn(), playlists: vi.fn(),
}));
vi.mock('../services/spotifyService', () => ({
    SpotifyService: {
        search: mocks.search,
        getUserProfile: mocks.profile,
        getRecentlyPlayed: mocks.recent,
        getSavedAlbums: mocks.albums,
        getSavedPlaylists: mocks.playlists,
    },
}));
vi.mock('../services/api', () => ({ api: { getSpotifyAuthStatus: mocks.status } }));
vi.mock('../services/libraryService', () => ({ libraryService: { getAllSongs: mocks.songs } }));

function deferred<T>() {
    let resolve!: (value: T) => void;
    let reject!: (reason: unknown) => void;
    const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
    return { promise, resolve, reject };
}

function results(name: string) {
    return {
        tracks: {
            items: [{
                id: name, name, duration_ms: 180000,
                artists: [{ id: 'fixture-artist', name: 'Fixture Artist' }],
                album: { id: 'fixture-album', name: 'Fixture Album', images: [] },
            }],
            next: null,
        },
        albums: { items: [], next: null },
        artists: { items: [], next: null },
        playlists: { items: [], next: null },
    };
}

type Results = ReturnType<typeof results>;
const initialState = useStore.getState();
let root: Root | undefined;
let host: HTMLDivElement;
let profile: ReturnType<typeof deferred<null>>;
let requests: Array<{
    query: string;
    options: SpotifySearchOptions;
    final: ReturnType<typeof deferred<Results>>;
}>;

beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    useStore.setState({
        spotifyConnected: false, spotifySessionGeneration: 0, spotifyAuthRequired: false,
        spotifyUser: null, spotifySearchQuery: '', spotifySearchResults: null,
        spotifyActiveTab: 'search',
    });
    profile = deferred<null>();
    mocks.profile.mockReturnValue(profile.promise);
    mocks.status.mockResolvedValue({ connected: true, authRequired: false });
    mocks.songs.mockResolvedValue([]);
    mocks.recent.mockResolvedValue({ items: [] });
    mocks.albums.mockResolvedValue({ items: [] });
    mocks.playlists.mockResolvedValue({ items: [] });
    requests = [];
    mocks.search.mockImplementation((query: string, _types: string[], _limit: number, _offset: number, options: SpotifySearchOptions) => {
        const final = deferred<Results>();
        requests.push({ query, options, final });
        return final.promise;
    });
    host = document.createElement('div');
    document.body.appendChild(host);
    root = createRoot(host);
});

afterEach(async () => {
    if (root) await act(async () => root!.unmount());
    root = undefined;
    // Finish the pending profile after unmount so its timeout is also cleaned up.
    await act(async () => profile.resolve(null));
    host.remove();
    useStore.setState(initialState, true);
    vi.clearAllTimers();
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
});

async function mount() {
    await act(async () => root!.render(<MemoryRouter initialEntries={['/spotify']}><Routes><Route path="/spotify" element={<Spotify />} /><Route path="/spotify/artist/:id" element={<p>Artist destination</p>} /></Routes></MemoryRouter>));
}

function searchInput() {
    const input = host.querySelector<HTMLInputElement>('input[placeholder^="Search Spotify"]');
    expect(input).not.toBeNull();
    return input!;
}

async function typeQuery(query: string) {
    const input = searchInput();
    await act(async () => {
        // Bypass React's value tracker to simulate a real controlled-input change.
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, query);
        input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    expect(input.value).toBe(query);
}

async function enter() {
    await act(async () => searchInput().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })));
}

async function advance(ms: number) {
    await act(async () => { vi.advanceTimersByTime(ms); });
}

async function tab(label: string) {
    const button = [...host.querySelectorAll('button')].find(item => item.textContent?.trim() === label);
    expect(button).toBeDefined();
    await act(async () => button!.click());
}

describe('Spotify interactive search regressions', () => {
    it('restores a logged-in session and dispatches Enter immediately while the profile is pending', async () => {
        await mount();
        expect(mocks.status).toHaveBeenCalledTimes(1);
        expect(useStore.getState().spotifyConnected).toBe(true);
        expect(mocks.profile).toHaveBeenCalledTimes(1);
        expect(useStore.getState().spotifyUser).toBeNull();
        expect(host.textContent).toContain('Spotify account');

        await typeQuery('instant query');
        expect(mocks.search).not.toHaveBeenCalled();
        await enter();
        expect(mocks.search).toHaveBeenCalledTimes(1);
        expect(mocks.search).toHaveBeenCalledWith('instant query', ['album', 'playlist', 'track', 'artist'], 20, 0, {
            signal: expect.any(AbortSignal), onCatalogResults: expect.any(Function),
        });
        expect(requests[0].options.signal!.aborted).toBe(false);
        expect(useStore.getState().spotifySearchQuery).toBe('instant query');
        expect(useStore.getState().spotifyUser).toBeNull();
        await advance(150);
        expect(mocks.search).toHaveBeenCalledTimes(1);
    });

    it('debounces typing for 150ms and resets the delay when the input changes', async () => {
        await mount();
        await typeQuery('first draft');
        await advance(149);
        expect(mocks.search).not.toHaveBeenCalled();
        await typeQuery('final query');
        await advance(149);
        expect(mocks.search).not.toHaveBeenCalled();
        await advance(1);
        expect(mocks.search).toHaveBeenCalledTimes(1);
        expect(requests[0].query).toBe('final query');
        expect(useStore.getState().spotifySearchQuery).toBe('final query');
    });

    it('renders catalog results before fallback resolves, then displays the final results', async () => {
        await mount();
        await typeQuery('progressive query');
        await enter();
        const catalog = results('Early catalog track');
        // The final service promise deliberately remains pending during publication.
        await act(async () => requests[0].options.onCatalogResults!(catalog));
        expect(host.textContent).toContain('Early catalog track');
        expect(useStore.getState().spotifySearchResults).toEqual(catalog);
        expect(host.querySelector('svg.animate-spin')).toBeNull();

        const final = results('Final fallback track');
        await act(async () => requests[0].final.resolve(final));
        expect(host.textContent).toContain('Final fallback track');
        expect(host.textContent).not.toContain('Early catalog track');
        expect(useStore.getState().spotifySearchResults).toEqual(final);
    });

    it('aborts on input changes before debounce and suppresses stale callbacks and completions', async () => {
        await mount();
        await typeQuery('old query');
        await enter();
        const old = requests[0];
        await typeQuery('new query');
        expect(old.options.signal!.aborted).toBe(true);
        expect(mocks.search).toHaveBeenCalledTimes(1);
        await act(async () => old.options.onCatalogResults!(results('Stale early track')));
        expect(host.textContent).not.toContain('Stale early track');
        expect(useStore.getState().spotifySearchResults).toBeNull();

        await enter();
        const fresh = results('Current track');
        await act(async () => requests[1].options.onCatalogResults!(fresh));
        await act(async () => {
            old.options.onCatalogResults!(results('Stale callback track'));
            old.final.resolve(results('Stale final track'));
        });
        expect(host.textContent).toContain('Current track');
        expect(host.textContent).not.toContain('Stale');
        expect(useStore.getState().spotifySearchResults).toEqual(fresh);
        expect(requests[1].options.signal!.aborted).toBe(false);
    });

    it('aborts when leaving Search and searches again only when returning to Search', async () => {
        await mount();
        await typeQuery('tab query');
        await enter();
        const old = requests[0];
        await tab('Recently Played');
        expect(old.options.signal!.aborted).toBe(true);
        expect(mocks.recent).toHaveBeenCalledWith(50);
        await act(async () => {
            old.options.onCatalogResults!(results('Off-tab catalog track'));
            old.final.resolve(results('Off-tab final track'));
        });
        await advance(150);
        expect(mocks.search).toHaveBeenCalledTimes(1);
        expect(useStore.getState().spotifySearchResults).toBeNull();
        await tab('Search');
        expect(mocks.search).toHaveBeenCalledTimes(2);
        expect(requests[1].query).toBe('tab query');
        expect(requests[1].options.signal!.aborted).toBe(false);
    });

    it('does not search a persisted query while a library tab is active', async () => {
        useStore.setState({ spotifySearchQuery: 'persisted query', spotifyActiveTab: 'albums' });
        await mount();
        await advance(150);
        expect(mocks.albums).toHaveBeenCalledWith(20, 0);
        expect(mocks.search).not.toHaveBeenCalled();
        await tab('Search');
        expect(mocks.search).toHaveBeenCalledTimes(1);
        expect(requests[0].query).toBe('persisted query');
    });

    it('aborts on unmount and ignores late catalog callbacks and final completion', async () => {
        await mount();
        await typeQuery('unmount query');
        await enter();
        const request = requests[0];
        await act(async () => root!.unmount());
        root = undefined;
        expect(request.options.signal!.aborted).toBe(true);
        await act(async () => {
            request.options.onCatalogResults!(results('Unmounted catalog track'));
            request.final.resolve(results('Unmounted final track'));
        });
        expect(host.textContent).toBe('');
        expect(useStore.getState().spotifySearchResults).toBeNull();
    });
});

describe('Spotify saved library pagination', () => {
    const playlist = (id: string) => ({ id, name: id, images: [], tracks: { total: 1 } });
    it('uses provider positions for offsets and preserves duplicates and unavailable entries', async () => {
        mocks.playlists.mockResolvedValueOnce({ offset: 0, total: 4, next: 'more', items: [playlist('same'), null] })
            .mockResolvedValueOnce({ offset: 2, total: 4, next: null, items: [playlist('same'), playlist('last')] });
        await mount(); await tab('Saved Playlists'); await tab('Load more');
        expect(mocks.playlists).toHaveBeenLastCalledWith(20, 2);
        expect(host.querySelectorAll('[aria-label="Open Spotify playlist same"]')).toHaveLength(2);
        expect(host.querySelector('[aria-label="Open Spotify playlist last"]')).not.toBeNull();
        expect(host.textContent).not.toContain('Load more');
    });
    it('keeps existing cards when a changed total invalidates a page and allows refresh', async () => {
        mocks.albums.mockResolvedValueOnce({ offset: 0, total: 2, next: 'more', items: [{ album: { id: 'old', name: 'Old album', images: [] } }] })
            .mockResolvedValueOnce({ offset: 1, total: 3, next: 'more', items: [{ album: { id: 'new', name: 'New album', images: [] } }] })
            .mockResolvedValueOnce({ offset: 0, total: 0, next: null, items: [] });
        await mount(); await tab('Saved Albums'); await tab('Load more');
        expect(host.textContent).toContain('Old album');
        expect(host.textContent).not.toContain('New album');
        expect(host.querySelector('[role="alert"]')?.textContent).toContain('Could not load more');
        await tab('Refresh library');
        expect(mocks.albums).toHaveBeenLastCalledWith(20, 0);
        expect(host.textContent).not.toContain('Old album');
    });
    it('discards a late page when the account generation changes', async () => {
        const late = deferred<any>();
        mocks.playlists.mockResolvedValueOnce({ offset: 0, total: 2, next: 'more', items: [playlist('old')] }).mockReturnValueOnce(late.promise);
        await mount(); await tab('Saved Playlists'); await tab('Load more');
        await act(async () => useStore.setState({ spotifySessionGeneration: 1, spotifyConnected: false }));
        await act(async () => late.resolve({ offset: 1, total: 2, next: null, items: [playlist('retired')] }));
        expect(host.textContent).not.toContain('retired');
    });
});


it('shows a provider failure instead of claiming an empty recent history', async () => {
    mocks.recent.mockRejectedValueOnce(new Error('fixture failure')).mockResolvedValueOnce({ items: [] });
    await mount(); await tab('Recently Played');
    expect(host.querySelector('[role="alert"]')?.textContent).toContain('Spotify could not load this view');
    expect(host.textContent).not.toContain('No recently played tracks');
    await tab('Retry library request');
    expect(mocks.recent).toHaveBeenCalledTimes(2);
    expect(host.textContent).toContain('No recently played tracks');
});


it('shows a current search failure, clears old results, and retries the same query', async () => {
    await mount(); await typeQuery('previous'); await enter();
    await act(async () => requests[0].final.resolve(results('Previous track')));
    await typeQuery('current'); await enter();
    await act(async () => requests[1].final.reject(new Error('fixture search failure')));
    expect(host.querySelector('[role="alert"]')?.textContent).toContain('Spotify search failed');
    expect(host.textContent).not.toContain('Previous track');
    await tab('Retry search');
    expect(requests[2].query).toBe('current');
    await act(async () => requests[2].final.resolve(results('Retried track')));
    expect(host.textContent).toContain('Retried track');
    expect(host.querySelector('[role="alert"]')).toBeNull();
});

 describe('Spotify artist navigation', () => {
    it('opens an artist result from the Artists tab', async () => {
        await mount(); await typeQuery('artist query'); await enter();
        const catalog = results('track');
        catalog.artists.items = [{id: 'AAAAAAAAAAAAAAAAAAAAAA', name: 'Result Artist', images: []}] as any;
        await act(async () => requests[0].final.resolve(catalog));
        await tab('Artists (1)');
        const artistLink = host.querySelector<HTMLAnchorElement>('a[aria-label="View artist Result Artist"]');
        expect(artistLink?.getAttribute('href')).toBe('/spotify/artist/AAAAAAAAAAAAAAAAAAAAAA');
        await act(async () => artistLink!.click());
        expect(host.textContent).toContain('Artist destination');
    });
    it('opens the same artist destination from a track artist name without playing the track', async () => {
        const play = vi.fn(); useStore.setState({playSong: play});
        await mount(); await typeQuery('track query'); await enter();
        await act(async () => requests[0].final.resolve(results('track')));
        const artistLink = host.querySelector<HTMLAnchorElement>('a[href="/spotify/artist/fixture-artist"]');
        expect(artistLink).not.toBeNull();
        await act(async () => artistLink!.click());
        expect(host.textContent).toContain('Artist destination');
        expect(play).not.toHaveBeenCalled();
    });
 });
