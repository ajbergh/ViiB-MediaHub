// @vitest-environment jsdom
/** Tests artist browsing, album navigation, playback context, partial failures, and session retirement. */
import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useStore } from '../store';
import { SpotifyArtistDetail } from './SpotifyArtistDetail';

const mocks = vi.hoisted(() => ({
    profile: vi.fn(),
    top: vi.fn(),
    releases: vi.fn(),
    download: vi.fn(),
}));
vi.mock('../services/spotifyArtist', () => ({
    spotifyArtist: {
        profile: mocks.profile,
        topTracks: mocks.top,
        releases: mocks.releases,
    },
}));
vi.mock('../services/api', () => ({ api: { downloadTrack: mocks.download } }));
vi.mock('../components/SpotifySessionConnect', () => ({
    SpotifySessionConnect: () => <p>Connect Spotify</p>,
}));
const artistID = 'AAAAAAAAAAAAAAAAAAAAAA';
const albumID = 'BBBBBBBBBBBBBBBBBBBBBB';
const tracks = [0, 1].map((index) => ({
    id: String(index),
    name: 'Top track ' + index,
    duration_ms: 180000,
    artists: [{ id: artistID, name: 'Result Artist' }],
    album: { id: albumID, name: 'Artist Album', images: [] },
}));
const profile = {
    id: artistID,
    name: 'Result Artist',
    images: [],
    followers: { total: 42 },
};
const releases = {
    items: [
        {
            id: albumID,
            name: 'Artist Album',
            images: [],
            album_type: 'album',
            release_date: '2020-01-01',
        },
        {
            id: 'SSSSSSSSSSSSSSSSSSSSSS',
            name: 'Artist Single',
            images: [],
            album_type: 'single',
            release_date: null,
        },
    ],
    truncated: false,
};
const initialState = useStore.getState();
let root: Root;
let host: HTMLDivElement;
const play = vi.fn(),
    queue = vi.fn(),
    toast = vi.fn();
function deferred<T>() {
    let resolve!: (value: T) => void;
    const promise = new Promise<T>((done) => {
        resolve = done;
    });
    return { promise, resolve };
}
beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    useStore.setState({
        spotifyConnected: true,
        spotifySessionGeneration: 10,
        playSong: play,
        addToQueue: queue,
        showToast: toast,
    });
    mocks.profile.mockResolvedValue(profile);
    mocks.top.mockResolvedValue({ tracks });
    mocks.releases.mockResolvedValue(releases);
    mocks.download.mockResolvedValue({});
    host = document.createElement('div');
    document.body.appendChild(host);
    root = createRoot(host);
});
afterEach(async () => {
    await act(async () => root.unmount());
    host.remove();
    useStore.setState(initialState, true);
    vi.unstubAllGlobals();
});
async function mount() {
    await act(async () =>
        root.render(
            <MemoryRouter initialEntries={['/spotify/artist/' + artistID]}>
                <Routes>
                    <Route
                        path="/spotify/artist/:id"
                        element={<SpotifyArtistDetail />}
                    />
                    <Route
                        path="/spotify/album/:id"
                        element={<p>Album destination</p>}
                    />
                    <Route
                        path="/spotify"
                        element={<p>Search destination</p>}
                    />
                </Routes>
            </MemoryRouter>,
        ),
    );
}
async function click(label: string) {
    const button = host.querySelector<HTMLButtonElement>(
        'button[aria-label="' + label + '"]',
    );
    expect(button).not.toBeNull();
    await act(async () => button!.click());
}
describe('Spotify artist detail', () => {
    it('renders profile, top tracks, albums and singles, with complete top-track playback and queue context', async () => {
        await mount();
        expect(host.textContent).toContain('Result Artist');
        expect(host.textContent).toContain('42 followers');
        expect(host.textContent).toContain('Artist Album');
        expect(host.textContent).toContain('Artist Single');
        await click('Play Top track 1');
        expect(play).toHaveBeenCalledWith(
            expect.objectContaining({ spotifyId: '1' }),
            expect.arrayContaining([
                expect.objectContaining({ spotifyId: '0' }),
                expect.objectContaining({ spotifyId: '1' }),
            ]),
        );
        await click('Queue top tracks');
        expect(queue).toHaveBeenCalledWith(
            expect.arrayContaining([
                expect.objectContaining({ spotifyId: '0' }),
                expect.objectContaining({ spotifyId: '1' }),
            ]),
        );
        await click('Download Top track 0');
        expect(mocks.download).toHaveBeenCalledWith(
            '0',
            'Top track 0',
            'Result Artist',
            'Artist Album',
            180,
        );
    });
    it('opens the existing album page from discography and returns to search', async () => {
        await mount();
        const album = host.querySelector<HTMLAnchorElement>(
            'section[aria-label="Albums"] a',
        );
        expect(album?.getAttribute('href')).toBe('/spotify/album/' + albumID);
        await act(async () => album!.click());
        expect(host.textContent).toContain('Album destination');
    });
    it('shows top tracks when discography fails and retries without replacing them with an empty success', async () => {
        mocks.releases.mockRejectedValueOnce(
            new Error('Release request failed'),
        );
        await mount();
        expect(host.textContent).toContain('Top track 0');
        expect(host.querySelector('[role="alert"]')?.textContent).toContain(
            'Release request failed',
        );
        expect(host.textContent).not.toContain('No releases available.');
        const retry = [...host.querySelectorAll('button')].find(
            (button) => button.textContent === 'Retry',
        );
        await act(async () => retry!.click());
        expect(host.textContent).toContain('Artist Album');
        expect(host.querySelector('[role="alert"]')).toBeNull();
    });
    it('aborts retired requests and hides prior-account data on disconnect', async () => {
        const old = deferred<typeof releases>();
        mocks.releases.mockReturnValueOnce(old.promise);
        await mount();
        const signal = mocks.releases.mock.calls[0][1] as AbortSignal;
        await act(async () =>
            useStore.setState({
                spotifyConnected: false,
                spotifySessionGeneration: 11,
            }),
        );
        expect(signal.aborted).toBe(true);
        expect(host.textContent).toContain('Connect Spotify');
        await act(async () => old.resolve(releases));
        expect(host.textContent).not.toContain('Artist Album');
        expect(host.textContent).not.toContain('Top track 0');
    });
    it('ignores old account responses after reconnect while showing new account data', async () => {
        const old = deferred<typeof profile>();
        mocks.profile.mockReturnValueOnce(old.promise);
        await mount();
        mocks.profile.mockResolvedValue({
            ...profile,
            name: 'New account artist',
        });
        await act(async () =>
            useStore.setState({ spotifySessionGeneration: 11 }),
        );
        expect(host.textContent).toContain('New account artist');
        await act(async () =>
            old.resolve({ ...profile, name: 'Stale artist' }),
        );
        expect(host.textContent).not.toContain('Stale artist');
    });
    it('discloses bounded discography and handles empty top tracks', async () => {
        mocks.releases.mockResolvedValue({ ...releases, truncated: true });
        mocks.top.mockResolvedValue({ tracks: [] });
        await mount();
        expect(host.textContent).toContain(
            'Showing the first 100 release groups',
        );
        expect(host.textContent).toContain('No top tracks available.');
        expect(
            host.querySelector<HTMLButtonElement>(
                'button[aria-label="Queue top tracks"]',
            )?.disabled,
        ).toBe(true);
    });
});
