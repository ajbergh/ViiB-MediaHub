// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot, Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { SpotifySessionConnect } from './SpotifySessionConnect';
import { api } from '../services/api';
import { useStore } from '../store';

let root: Root | undefined;
(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
afterEach(async () => {
    if (root) await act(async () => root!.unmount());
    root = undefined;
    vi.useRealTimers();
    vi.restoreAllMocks();
    useStore.getState().logoutSpotify();
});
async function render() {
    const host = document.createElement('div');
    root = createRoot(host);
    await act(async () => root!.render(<SpotifySessionConnect />));
    return host;
}
function button(host: HTMLElement, text: string) {
    return Array.from(host.querySelectorAll('button')).find(button => button.textContent === text)!;
}
describe('Spotify browser sign-in', () => {
    it('shows reconnect state and never asks for a password or cookie', async () => {
        useStore.getState().markSpotifyAuthRequired();
        const host = await render();
        expect(host.textContent).toContain('Your Spotify session expired');
        expect(host.querySelector('input')).toBeNull();
        expect(host.textContent).not.toContain('sp_dc');
        expect(button(host, 'Sign in with Spotify')).toBeTruthy();
    });
    it('connects through browser status, retires renderer state and disconnects', async () => {
        useStore.getState().logoutSpotify();
        const start = vi.spyOn(api, 'startSpotifyBrowserLogin').mockResolvedValue({ id: 'operation', state: 'pending' });
        vi.spyOn(api, 'getSpotifyBrowserLogin').mockResolvedValue({ id: 'operation', state: 'connected' });
        vi.spyOn(api, 'getSpotifyAuthStatus').mockResolvedValue({ provider: 'webplayer', connected: true, authRequired: false, message: '' });
        const manual = vi.spyOn(api, 'connectSpotifySession');
        const disconnect = vi.spyOn(api, 'disconnectSpotifySession').mockResolvedValue({ provider: 'webplayer', connected: false, authRequired: true, message: '' });
        const host = await render();
        await act(async () => button(host, 'Sign in with Spotify').click());
        expect(start).toHaveBeenCalledWith();
        expect(manual).not.toHaveBeenCalled();
        expect(useStore.getState().spotifyConnected).toBe(true);
        expect(useStore.getState().spotifyAccessToken).toBeNull();
        await act(async () => button(host, 'Disconnect').click());
        expect(disconnect).toHaveBeenCalledOnce();
        expect(useStore.getState().spotifyConnected).toBe(false);
    });
    it('cancels a pending login and stops polling', async () => {
        vi.useFakeTimers();
        vi.spyOn(api, 'startSpotifyBrowserLogin').mockResolvedValue({ id: 'operation', state: 'pending' });
        const poll = vi.spyOn(api, 'getSpotifyBrowserLogin').mockResolvedValue({ id: 'operation', state: 'pending' });
        const cancel = vi.spyOn(api, 'cancelSpotifyBrowserLogin').mockResolvedValue({ id: 'operation', state: 'canceled' });
        const host = await render();
        await act(async () => button(host, 'Sign in with Spotify').click());
        expect(host.textContent).toContain('Waiting for Spotify sign-in');
        await act(async () => button(host, 'Cancel sign-in').click());
        await act(async () => vi.advanceTimersByTimeAsync(5000));
        expect(cancel).toHaveBeenCalledWith('operation');
        expect(poll).toHaveBeenCalledOnce();
        expect(useStore.getState().spotifyConnected).toBe(false);
    });
    it('cancels on unmount and ignores a late connected response', async () => {
        let resolve!: (value: { id: string; state: 'connected' }) => void;
        vi.spyOn(api, 'startSpotifyBrowserLogin').mockResolvedValue({ id: 'operation', state: 'pending' });
        vi.spyOn(api, 'getSpotifyBrowserLogin').mockImplementation(() => new Promise(done => { resolve = done; }));
        const cancel = vi.spyOn(api, 'cancelSpotifyBrowserLogin').mockResolvedValue({ id: 'operation', state: 'canceled' });
        const host = await render();
        await act(async () => button(host, 'Sign in with Spotify').click());
        await act(async () => root!.unmount());
        root = undefined;
        await act(async () => resolve({ id: 'operation', state: 'connected' }));
        expect(cancel).toHaveBeenCalledWith('operation');
        expect(useStore.getState().spotifyConnected).toBe(false);
    });
    it('renders browser failure and allows another attempt', async () => {
        vi.spyOn(api, 'startSpotifyBrowserLogin').mockResolvedValue({ id: 'operation', state: 'pending' });
        vi.spyOn(api, 'getSpotifyBrowserLogin').mockResolvedValue({ id: 'operation', state: 'failed', message: 'Install Chrome, Chromium or Microsoft Edge to sign in.' });
        const host = await render();
        await act(async () => button(host, 'Sign in with Spotify').click());
        expect(host.querySelector('[role="alert"]')?.textContent).toContain('Install Chrome');
        expect(button(host, 'Sign in with Spotify').disabled).toBe(false);
    });
});

it('recovers when final session status fails after browser connection', async () => {
    vi.spyOn(api, 'startSpotifyBrowserLogin').mockResolvedValue({ id: 'operation', state: 'pending' });
    vi.spyOn(api, 'getSpotifyBrowserLogin').mockResolvedValue({ id: 'operation', state: 'connected' });
    vi.spyOn(api, 'getSpotifyAuthStatus').mockRejectedValue(new Error('offline'));
    const host = await render();
    await act(async () => button(host, 'Sign in with Spotify').click());
    expect(host.querySelector('[role="alert"]')?.textContent).toContain('Could not check');
    expect(button(host, 'Sign in with Spotify').disabled).toBe(false);
});

it('does not restore a renderer session retired during browser sign-in', async () => {
    let resolve!: (value: { id: string; state: 'connected' }) => void;
    vi.spyOn(api, 'startSpotifyBrowserLogin').mockResolvedValue({ id: 'operation', state: 'pending' });
    vi.spyOn(api, 'getSpotifyBrowserLogin').mockImplementation(() => new Promise(done => { resolve = done; }));
    vi.spyOn(api, 'getSpotifyAuthStatus').mockResolvedValue({ provider: 'webplayer', connected: true, authRequired: false, message: '' });
    const host = await render();
    await act(async () => button(host, 'Sign in with Spotify').click());
    await act(async () => useStore.getState().markSpotifyAuthRequired());
    await act(async () => resolve({ id: 'operation', state: 'connected' }));
    expect(useStore.getState().spotifyConnected).toBe(false);
    expect(button(host, 'Sign in with Spotify').disabled).toBe(false);
});
