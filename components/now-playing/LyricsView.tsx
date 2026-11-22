/** Fetches and displays lyrics for the current song, including synchronized lines when available. */

import React, { useEffect, useState, useRef } from 'react';
import { Mic2 } from 'lucide-react';
import { Song } from '../../types';

interface SyncedLine {
    time: number;
    text: string;
}

interface Props {
    song: Song;
    currentTime: number;
    onSeek: (time: number) => void;
    isVisible: boolean;
    presentation?: 'drawer' | 'stage';
}

export const LyricsView: React.FC<Props> = ({ song, currentTime, onSeek, isVisible, presentation = 'drawer' }) => {
    const [lyrics, setLyrics] = useState<string | null>(null);
    const [syncedLyrics, setSyncedLyrics] = useState<SyncedLine[] | null>(null);
    const [isLoading, setIsLoading] = useState(false);
    const [error, setError] = useState(false);
    const activeLineRef = useRef<HTMLDivElement>(null);
    const containerRef = useRef<HTMLDivElement>(null);
    const onStage = presentation === 'stage';

    useEffect(() => {
        const controller = new AbortController();
        const { signal } = controller;
        const fetchLyrics = async () => {
            setIsLoading(true);
            setError(false);
            setLyrics(null);
            setSyncedLyrics(null);

            try {
                let params = new URLSearchParams({
                    track_name: song.title,
                    artist_name: song.artist,
                    album_name: song.album
                });

                let response = await fetch(`https://lrclib.net/api/get?${params.toString()}`, { signal });
                
                if (!response.ok) {
                    // Retry without album
                    params = new URLSearchParams({
                        track_name: song.title,
                        artist_name: song.artist
                    });
                    response = await fetch(`https://lrclib.net/api/get?${params.toString()}`, { signal });
                }

                if (!response.ok) throw new Error('Lyrics not found');

                const data = await response.json();
                if (signal.aborted) return;
                
                if (data.syncedLyrics) {
                    const lines = data.syncedLyrics.split('\n');
                    const parsed: SyncedLine[] = [];
                    const timeRegex = /^\[(\d{2}):(\d{2}\.\d{2,3})\](.*)$/;

                    lines.forEach((line: string) => {
                        const match = line.match(timeRegex);
                        if (match) {
                            const minutes = parseInt(match[1], 10);
                            const seconds = parseFloat(match[2]);
                            const text = match[3].trim();
                            parsed.push({ time: minutes * 60 + seconds, text });
                        }
                    });
                    setSyncedLyrics(parsed);
                }
                
                if (data.plainLyrics) {
                    setLyrics(data.plainLyrics);
                } else if (!data.syncedLyrics) {
                    setError(true);
                }

            } catch (err) {
                if (signal.aborted) return;
                console.warn('Failed to fetch lyrics:', err);
                setError(true);
            } finally {
                if (!signal.aborted) setIsLoading(false);
            }
        };

        fetchLyrics();
        return () => controller.abort();
    }, [song.id, song.title, song.artist, song.album]);

    const activeLyricIndex = syncedLyrics 
        ? syncedLyrics.findIndex((line, index) => {
            const nextLine = syncedLyrics[index + 1];
            return currentTime >= line.time && (!nextLine || currentTime < nextLine.time);
        })
        : -1;

    useEffect(() => {
        if (!isVisible || !activeLineRef.current) return;
        const viewport = containerRef.current?.closest<HTMLElement>('[data-lyrics-scroll]');
        if (!viewport) return;
        const line = activeLineRef.current.getBoundingClientRect();
        const relativeTop = line.top - viewport.getBoundingClientRect().top;
        viewport.scrollTo({
            top: viewport.scrollTop + relativeTop - (viewport.clientHeight - line.height) / 2,
            behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'smooth',
        });
    }, [activeLyricIndex, isVisible, presentation, syncedLyrics]);

    if (isLoading) {
        return (
            <div className="flex-1 flex items-center justify-center text-text-subtle opacity-70 animate-pulse">
                Loading lyrics...
            </div>
        );
    }

    if (error) {
        return (
            <div className="flex-1 flex flex-col items-center justify-center text-text-subtle opacity-70 text-center">
                <Mic2 size={48} className="mb-4 opacity-60" />
                <p>Lyrics not available for this track.</p>
            </div>
        );
    }

    if (syncedLyrics) {
        return (
            <div ref={containerRef} className={`space-y-6 py-[50%] text-center ${onStage ? 'mx-auto w-full max-w-4xl px-3 sm:px-8' : ''}`}>
                {syncedLyrics.map((line, idx) => (
                    <div 
                        key={idx}
                        ref={idx === activeLyricIndex ? activeLineRef : null}
                        role="button"
                        tabIndex={0}
                        aria-current={idx === activeLyricIndex}
                        className={`cursor-pointer transition-all duration-500 font-bold leading-snug px-4 motion-reduce:transition-none ${onStage ? 'text-[clamp(1.35rem,4vw,3.5rem)] tracking-tight' : 'text-card md:text-section'} ${
                            idx === activeLyricIndex 
                                ? `scale-105 origin-center motion-reduce:transform-none ${onStage ? 'bg-gradient-to-r from-text-main via-text-main to-brand bg-clip-text text-transparent' : 'text-text-main'}`
                                : 'text-text-main opacity-30 blur-[1px] hover:opacity-60 hover:blur-0 cursor-pointer'
                        }`}
                        onClick={() => onSeek(line.time)}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter' || e.key === ' ') {
                                e.preventDefault();
                                e.stopPropagation();
                                onSeek(line.time);
                            }
                        }}
                    >
                        {line.text || "♫"}
                    </div>
                ))}
            </div>
        );
    }

    return (
        <div className={`whitespace-pre-wrap text-center leading-relaxed text-text-main opacity-80 font-medium ${onStage ? 'mx-auto max-w-3xl px-5 py-14 text-xl sm:text-3xl' : 'pb-10 text-lg'}`}>
            {lyrics}
        </div>
    );
};
