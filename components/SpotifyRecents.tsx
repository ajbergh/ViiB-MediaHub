/** Spotify Web App Recents: dated, ordered entries and expandable listening groups. */
import React, { useEffect, useRef, useState } from 'react';
import { CheckCircle, ChevronDown, Download, ExternalLink, ListPlus, Play } from 'lucide-react';
import { useNavigate } from 'react-router';
import { useStore } from '../store';
import { SpotifyService } from '../services/spotifyService';
import { SpotifyRateLimitError } from '../lib/spotifyErrors';
import { Button } from './ui/Button';
import { formatTime } from '../utils';
import { spotifyTrackToSong } from '../lib/spotifyHelpers';
import { ContextMenuType } from '../types';

export interface RecentEntry {
    uri: string; name: string; type: string; date: string; subtitle: string;
    images: Array<{url: string}>; group_id: string; children_group_id: string; group_count: number;
    track?: any;
}
export interface WebPlayerRecents {
    source: 'webplayer_recents'; items: RecentEntry[]; limit: number; offset: number;
    next_offset: number | null; total: number;
}

function dateLabel(value: string) {
    const date = new Date(`${value}T12:00:00`);
    const today = new Date();
    if (date.toDateString() === today.toDateString()) return 'Today';
    today.setDate(today.getDate() - 1);
    if (date.toDateString() === today.toDateString()) return 'Yesterday';
    return date.toLocaleDateString(undefined, {weekday:'short', month:'short', day:'numeric'});
}

export function SpotifyRecents({data, disabled, onPlay, onQueue, onDownload, downloadedIds, downloadingIds}: {
    data: WebPlayerRecents; disabled: boolean;
    onPlay: (track: any, tracks: any[], index: number) => void; onQueue: (track: any) => void;
    onDownload: (track: any) => void; downloadedIds: Set<string>; downloadingIds: Set<string>;
}) {
    const navigate = useNavigate();
    const [expanded, setExpanded] = useState<Set<string>>(new Set());
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [changed, setChanged] = useState(false);
    const request = useRef(0);
    useEffect(() => { setBusy(false); return () => {request.current++;}; }, [data]);
    const rows = data.items.map((item, index) => ({ item, index }));
    const tracks = data.items.flatMap(item => item.track ? [item.track] : []);
    let queueIndex = 0;
    const queuePositions = data.items.map(item => item.track ? queueIndex++ : -1);
    const parents = new Set(data.items.filter(item => item.children_group_id).map(item => `${item.date}:${item.children_group_id}`));
    const dates = [...new Set(data.items.map(item => item.date))];
    const open = (item: RecentEntry) => {
        const match = /^spotify:(album|playlist|artist):([a-zA-Z0-9]{22})$/.exec(item.uri);
        if (match) navigate(`/spotify/${match[1]}/${match[2]}`);
        else if (/^spotify:user:[^:]+:collection$/.test(item.uri)) window.open('https://open.spotify.com/collection/tracks', '_blank', 'noopener,noreferrer');
    };
    const renderRow = (item: RecentEntry, index: number, child = false) => {
        const groupKey = `${item.date}:${item.children_group_id}`;
        const isGroup = Boolean(item.children_group_id);
        const label = item.name || (item.type === 'unavailable' ? 'Unavailable item' : item.group_count ? `${item.group_count} ${item.group_count === 1 ? 'song' : 'songs'} played` : 'Songs played');
        const external = /^spotify:(episode|show|audiobook|chapter):([a-zA-Z0-9]{22})$/.exec(item.uri);
        return <React.Fragment key={`${item.date}:${item.uri}:${index}`}>
            <div className={`flex items-center gap-3 p-3 rounded-lg hover:bg-surface-hover ${child ? 'ml-8' : ''}`} onContextMenu={event => {
                if (!item.track) return;
                event.preventDefault();
                useStore.getState().openContextMenu(event, ContextMenuType.SONG, spotifyTrackToSong(item.track));
            }}>
                {isGroup && <button aria-label={`${expanded.has(groupKey) ? 'Collapse' : 'Expand'} ${label}`} aria-expanded={expanded.has(groupKey)} onClick={() => setExpanded(previous => {const next = new Set(previous); if (next.has(groupKey)) next.delete(groupKey); else next.add(groupKey); return next;})} className="p-2 text-text-secondary">
                    <ChevronDown size={18} className={expanded.has(groupKey) ? '' : '-rotate-90'}/>
                </button>}
                {item.images[0] && <img src={item.images[0].url} alt="" loading="lazy" className="w-12 h-12 rounded object-cover"/>}
                <div className="flex-1 min-w-0">
                    {item.track ? <button disabled={disabled} className="text-left text-text-main hover:text-brand disabled:opacity-50" onClick={() => onPlay(item.track, tracks, queuePositions[index])}>{label}</button>
                        : isGroup && item.uri ? <button className="text-left text-text-main hover:text-brand" onClick={() => open(item)}>{label}</button>
                        : <span className="text-text-main">{label || 'Unavailable item'}</span>}
                    <p className="text-sm text-text-secondary truncate">{item.subtitle || (item.type === 'group' ? 'Listening history' : item.type)}</p>
                </div>
                {item.track && <>
                    <span className="text-sm text-text-subtle">{formatTime(item.track.duration_ms / 1000)}</span>
                    <button disabled={disabled} aria-label={`Play ${item.name}`} onClick={() => onPlay(item.track, tracks, queuePositions[index])} className="p-2 text-brand disabled:opacity-50"><Play size={18}/></button>
                    <button disabled={disabled} aria-label={`Queue ${item.name}`} onClick={() => onQueue(item.track)} className="p-2 text-text-secondary disabled:opacity-50"><ListPlus size={18}/></button>
                    {downloadedIds.has(item.track.id) ? <CheckCircle size={18} className="text-brand" aria-label="Downloaded"/> : <button disabled={disabled || downloadingIds.has(item.track.id)} aria-label={`Download ${item.name}`} onClick={() => onDownload(item.track)} className="p-2 text-text-secondary disabled:opacity-50"><Download size={18}/></button>}
                </>}
                {external && <a href={`https://open.spotify.com/${external[1]}/${external[2]}`} target="_blank" rel="noopener noreferrer" aria-label={`Open ${item.name} in Spotify`} className="p-2 text-text-secondary"><ExternalLink size={18}/></a>}
            </div>
            {!child && isGroup && expanded.has(groupKey) && rows.filter(({item: entry}) => entry.date === item.date && entry.group_id === item.children_group_id).map(({item: entry, index: childIndex}) => renderRow(entry, childIndex, true))}
        </React.Fragment>;
    };
    const loadMore = async () => {
        if (busy || changed || data.next_offset === null) return;
        const id = ++request.current;
        const generation = useStore.getState().spotifySessionGeneration;
        const current = () => request.current === id && generation === useStore.getState().spotifySessionGeneration && useStore.getState().spotifyRecentlyPlayed === data;
        setBusy(true); setError('');
        try {
            const page = await SpotifyService.getRecentlyPlayed(data.limit, data.next_offset);
            if (!current()) return;
            if (page.source !== data.source || page.offset !== data.next_offset) throw new Error('Unexpected history page');
            if (page.total !== data.total) {
                setChanged(true);
                setError('Spotify history changed. Refresh history to continue.');
                return;
            }
            useStore.getState().setSpotifyRecentlyPlayed({...page, offset:data.offset, items:[...data.items, ...page.items]});
        } catch (error) {
            if (current()) setError(error instanceof SpotifyRateLimitError ? `Spotify is rate limiting requests. Retry in ${error.retryAfter} seconds.` : 'Could not load more history. Try again.');
        } finally { if (request.current === id) setBusy(false); }
    };
    return <div className="space-y-6">
        <Button disabled={busy} onClick={() => useStore.getState().setSpotifyRecentlyPlayed(null)}>Refresh history</Button>
        {dates.map(date => <section key={date} aria-label={`Recently played on ${date}`}>
            <h2 className="text-lg font-semibold text-text-main mb-2">{dateLabel(date)}</h2>
            <div className="bg-surface-1 rounded-xl">{rows.filter(({item}) => item.date === date && (!item.group_id || item.group_id === '0' || !parents.has(`${date}:${item.group_id}`))).map(({item, index}) => renderRow(item, index))}</div>
        </section>)}
        {data.items.length === 0 && <p className="text-text-secondary py-12 text-center">No recently played items</p>}
        {error && <p role="alert">{error}</p>}
        {!changed && data.next_offset !== null && <Button disabled={busy} onClick={loadMore}>{busy ? 'Loading more…' : 'Load more history'}</Button>}
    </div>;
}
