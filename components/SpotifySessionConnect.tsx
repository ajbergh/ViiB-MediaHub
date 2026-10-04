import React, { useEffect, useRef, useState } from 'react';
import { api } from '../services/api';
import { useStore } from '../store';
import { Button } from './ui/Button';

export const SpotifySessionConnect: React.FC<{ onConnected?: () => void }> = ({ onConnected }) => {
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const pending = useRef<string | null>(null);
    const generation = useRef(0);
    const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
    const authRequired = useStore(state => state.spotifyAuthRequired);
    const connected = useStore(state => state.spotifyConnected);
    const setConnected = useStore(state => state.setSpotifyConnected);
    const logout = useStore(state => state.logoutSpotify);
    useEffect(() => () => {
        generation.current++;
        clearTimeout(timer.current);
        if (pending.current) void api.cancelSpotifyBrowserLogin(pending.current).catch(() => {});
    }, []);
    const connect = async () => {
        const attempt = ++generation.current;
        const sessionGeneration = useStore.getState().spotifySessionGeneration;
        setBusy(true);
        setError('');
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
                    onConnected?.();
                } else if (status.state === 'failed') {
                    setError(status.message || 'Could not connect to Spotify. Try again.');
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
            const status = await api.startSpotifyBrowserLogin();
            if (generation.current !== attempt) {
                void api.cancelSpotifyBrowserLogin(status.id).catch(() => {});
                return;
            }
            pending.current = status.id;
            void poll(status.id);
        } catch {
            if (generation.current !== attempt) return;
            setError('Could not open Spotify sign-in. Try again.');
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
        {connected && <p className="text-success text-sm">Spotify session connected</p>}
        <p className="text-sm text-text-secondary">Sign in on Spotify in the window that opens. ViiB connects automatically when you finish.</p>
        <p className="text-xs text-text-subtle">Your session is stored encrypted on this device.</p>
        {busy && <p role="status" className="text-sm text-text-secondary">Waiting for Spotify sign-in…</p>}
        <div className="flex gap-3">
            <Button type="button" disabled={busy} onClick={() => void connect()}>{connected ? 'Switch Spotify account' : 'Sign in with Spotify'}</Button>
            {busy && <Button type="button" variant="secondary" onClick={() => void cancel()}>Cancel sign-in</Button>}
            {connected && <Button type="button" variant="secondary" disabled={busy} onClick={() => void disconnect()}>Disconnect</Button>}
        </div>
        {error && <p role="alert" className="text-danger text-sm">{error}</p>}
    </div>;
};
