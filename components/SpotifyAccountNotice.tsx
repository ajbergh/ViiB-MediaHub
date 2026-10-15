import React, { useState } from 'react';
import { useStore } from '../store';
import { api } from '../services/api';
import { Button } from './ui/Button';

export const SpotifyAccountNotice: React.FC = () => {
    const connected = useStore(s => s.spotifyConnected);
    const product = useStore(s => s.spotifyAccountProduct);
    const [busy, setBusy] = useState(false);
    if (!connected || product === 'premium') return null;
    const refresh = async () => {
        const generation = useStore.getState().spotifySessionGeneration;
        setBusy(true);
        try {
            const status = await api.getSpotifyAuthStatus(true);
            if (generation === useStore.getState().spotifySessionGeneration) {
                useStore.getState().setSpotifyAccountProduct(status.connected && !status.authRequired ? status.product : undefined);
            }
        } finally { setBusy(false); }
    };
    return <div role="status" className="rounded-lg bg-surface-1 p-4 text-sm text-text-secondary mb-4">
        <p>{product === 'free' ? 'Spotify Free connected. Search, library browsing, and metadata are available. Streaming and downloads require Spotify Premium.' : 'Spotify account plan is unverified. Metadata is available. Streaming and downloads are disabled until Premium is verified.'} Local files remain playable.</p>
        <Button size="sm" disabled={busy} onClick={() => void refresh().catch(() => {})}>{busy ? 'Checking account…' : 'Recheck account plan'}</Button>
    </div>;
};
