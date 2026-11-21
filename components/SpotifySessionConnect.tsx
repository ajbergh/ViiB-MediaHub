/** Offers desktop PKCE and Web Player sign-in, polls attempt status, and disconnects backend-owned sessions. */

import React, { useEffect, useId, useRef, useState } from 'react';
import { api } from '../services/api';
import { openExternalURL } from '../services/externalNavigation';
import { useStore } from '../store';
import { getOAuthCallbackUrl, isWailsEnvironment, SPOTIFY_DESKTOP_CALLBACK_URL } from '../utils';
import { Button } from './ui/Button';

const isSpotifyClientId = (value: string) => /^[a-f0-9]{32}$/i.test(value);
const releaseClientId = (import.meta.env.VITE_SPOTIFY_CLIENT_ID ?? '').trim();
const hasReleaseClientId = isSpotifyClientId(releaseClientId);

export const SpotifySessionConnect: React.FC<{ onConnected?: () => void }> = ({ onConnected }) => {
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [browserRequired, setBrowserRequired] = useState(false);
    const [showWebAPIOption, setShowWebAPIOption] = useState(false);
    const [showOAuthSetup, setShowOAuthSetup] = useState(false);
    const [callbackUrl, setCallbackUrl] = useState(SPOTIFY_DESKTOP_CALLBACK_URL);
    const clientIdInput = useId();
    const savedClientId = useStore(state => state.spotifyClientId);
    const [clientId, setClientId] = useState(savedClientId);
    const desktop = isWailsEnvironment();
    const pending = useRef<string | null>(null);
    const generation = useRef(0);
    const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
    const authRequired = useStore(state => state.spotifyAuthRequired);
    const connected = useStore(state => state.spotifyConnected);
    const webAPIOnly = useStore(state => state.spotifyWebAPIOnly);
    const setConnected = useStore(state => state.setSpotifyConnected);
    const logout = useStore(state => state.logoutSpotify);
    useEffect(() => () => {
        generation.current++;
        clearTimeout(timer.current);
        if (pending.current) void api.cancelSpotifyBrowserLogin(pending.current).catch(() => {});
    }, []);
    const connect = async (oauthClientId?: string) => {
        const attempt = ++generation.current;
        const sessionGeneration = useStore.getState().spotifySessionGeneration;
        setBusy(true);
        setError('');
        setBrowserRequired(false);
        const poll = async (id: string) => {
            try {
                const status = await api.getSpotifyBrowserLogin(id);
                if (generation.current !== attempt) return;
                if (status.state === 'pending') {
                    timer.current = setTimeout(() => void poll(id), 1000);
                    return;
                }
                pending.current = null;
                if (status.state === 'connected') {
                    const session = await api.getSpotifyAuthStatus();
                    if (generation.current !== attempt) return;
                    if (useStore.getState().spotifySessionGeneration !== sessionGeneration) { setBusy(false); return; }
                    logout();
                    setConnected(session.connected && !session.authRequired);
                    const ready = session.connected && !session.authRequired;
                    useStore.getState().setSpotifyAccountProduct(ready ? session.product : undefined, ready && session.webApiOnly);
                    onConnected?.();
                } else if (status.state === 'failed') {
                    setError(status.message || 'Could not connect to Spotify. Try again.');
                    setBrowserRequired(status.browserRequired === true);
                }
                setBusy(false);
            } catch {
                if (generation.current !== attempt) return;
                setError('Could not check Spotify sign-in. Try again.');
                if (pending.current) void api.cancelSpotifyBrowserLogin(pending.current).catch(() => {});
                pending.current = null;
                setBusy(false);
            }
        };
        try {
            const registerAttempt = (id: string) => {
                if (generation.current !== attempt) {
                    void api.cancelSpotifyBrowserLogin(id).catch(() => {});
                    return false;
                }
                pending.current = id;
                return true;
            };
            if (oauthClientId) {
                const status = await api.startSpotifyOAuthLogin(oauthClientId);
                if (!registerAttempt(status.id)) return;
                await openExternalURL(status.authUrl);
                if (generation.current !== attempt) return;
                if (oauthClientId !== releaseClientId) useStore.getState().setSpotifyCredentials(oauthClientId, '');
                void poll(status.id);
                return;
            }
            const status = await api.startSpotifyBrowserLogin();
            if (registerAttempt(status.id)) void poll(status.id);
        } catch (cause) {
            if (generation.current !== attempt) return;
            if (pending.current) void api.cancelSpotifyBrowserLogin(pending.current).catch(() => {});
            pending.current = null;
            setError(oauthClientId && cause instanceof Error ? cause.message : 'Could not open Spotify sign-in. Try again.');
            setBusy(false);
        }
    };
    const cancel = async () => {
        generation.current++;
        clearTimeout(timer.current);
        const id = pending.current;
        pending.current = null;
        try {
            if (id) await api.cancelSpotifyBrowserLogin(id);
            setBusy(false);
            setError('');
        } catch {
            pending.current = id;
            setError('Could not cancel sign-in. Try again.');
        }
    };
    const disconnect = async () => {
        setBusy(true);
        setError('');
        try { await api.disconnectSpotifySession(); logout(); }
        catch { setError('Could not disconnect. Try again.'); }
        finally { setBusy(false); }
    };
    return <div className="w-full max-w-lg space-y-4 text-left">
        {authRequired && <p className="text-warning text-sm">Your Spotify session expired. Connect again to continue.</p>}
        {connected && <p className="text-success text-sm">{webAPIOnly ? 'Spotify Web API connected' : 'Spotify session connected'}</p>}
        <p className="text-sm text-text-secondary">Sign in once to search Spotify. With Spotify Premium, the same connection enables streaming and downloads in ViiB. Sign-in uses Chrome, Microsoft Edge, or Chromium.</p>
        <p className="text-xs text-text-subtle">ViiB saves your session encrypted on this device and restores it when you reopen the app.</p>
        {busy && <p role="status" className="text-sm text-text-secondary">Waiting for Spotify sign-in…</p>}
        <div className="flex flex-wrap gap-3">
            <Button type="button" disabled={busy} onClick={() => void connect()}>{webAPIOnly ? 'Connect for streaming and downloads' : connected ? 'Switch Spotify account' : 'Sign in with Spotify'}</Button>
            {busy && <Button type="button" variant="secondary" onClick={() => void cancel()}>Cancel sign-in</Button>}
            {connected && <Button type="button" variant="secondary" disabled={busy} onClick={() => void disconnect()}>Disconnect</Button>}
        </div>
        {error && <p role="alert" className="text-danger text-sm">{error}</p>}
        {browserRequired && <a className="text-sm underline" href="https://www.google.com/chrome/" target="_blank" rel="noreferrer" onClick={desktop ? event => {
            event.preventDefault();
            void openExternalURL('https://www.google.com/chrome/').catch(() => setError('Could not open the Chrome download page.'));
        } : undefined}>Get Chrome for Spotify sign-in</a>}
        {desktop && <Button type="button" variant="secondary" disabled={busy} aria-expanded={showWebAPIOption} onClick={() => setShowWebAPIOption(!showWebAPIOption)}>Browsing-only sign-in</Button>}
        {desktop && showWebAPIOption && <div className="space-y-2">
            <p className="text-sm text-text-secondary">Use your default browser, including Safari, for Spotify Web API features like top tracks and Spotify Connect.{!hasReleaseClientId && ' This option requires a Spotify Developer Client ID.'}</p>
            {hasReleaseClientId && <Button type="button" disabled={busy} onClick={() => void connect(releaseClientId)}>Sign in with your default browser</Button>}
            <Button type="button" variant="secondary" disabled={busy} aria-expanded={showOAuthSetup} onClick={() => {
                setShowOAuthSetup(!showOAuthSetup);
                if (!showOAuthSetup) void getOAuthCallbackUrl().then(setCallbackUrl);
            }}>{hasReleaseClientId ? 'Use your own Spotify Client ID' : 'Sign in with your default browser'}</Button>
            {showOAuthSetup && <div className="space-y-2">
                <p className="text-sm text-text-secondary">Add <code className="break-all">{callbackUrl}</code> as a Redirect URI in your <a className="underline" href="https://developer.spotify.com/dashboard" target="_blank" rel="noreferrer" onClick={event => {
                    event.preventDefault();
                    void openExternalURL('https://developer.spotify.com/dashboard').catch(() => setError('Could not open the Spotify Developer Dashboard.'));
                }}>Spotify Developer Dashboard</a> app. If the app uses Development Mode, authorize your Spotify account there too.</p>
                <label className="block text-sm" htmlFor={clientIdInput}>Spotify Client ID</label>
                <input id={clientIdInput} className="w-full rounded border border-border bg-surface px-3 py-2 text-sm" value={clientId} autoComplete="off" spellCheck={false} maxLength={32} disabled={busy} onChange={event => setClientId(event.target.value)} />
                <Button type="button" disabled={busy || !isSpotifyClientId(clientId.trim())} onClick={() => void connect(clientId.trim())}>Open Spotify authorization</Button>
            </div>}
        </div>}
    </div>;
};
