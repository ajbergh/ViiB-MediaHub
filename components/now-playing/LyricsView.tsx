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
}

export const LyricsView: React.FC<Props> = ({ song, currentTime, onSeek }) => {
    const [lyrics, setLyrics] = useState<string | null>(null);
    const [syncedLyrics, setSyncedLyrics] = useState<SyncedLine[] | null>(null);
    const [isLoading, setIsLoading] = useState(false);
    const [error, setError] = useState(false);
    const activeLineRef = useRef<HTMLDivElement>(null);
    const containerRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
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

                let response = await fetch(`https://lrclib.net/api/get?${params.toString()}`);
                
                if (!response.ok) {
                    // Retry without album
                    params = new URLSearchParams({
                        track_name: song.title,
                        artist_name: song.artist
                    });
                    response = await fetch(`https://lrclib.net/api/get?${params.toString()}`);
                }

                if (!response.ok) throw new Error('Lyrics not found');

                const data = await response.json();
                
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
                console.warn('Failed to fetch lyrics:', err);
                setError(true);
            } finally {
                setIsLoading(false);
            }
        };

        fetchLyrics();
    }, [song.id, song.title, song.artist]);

    // Auto-scroll synced lyrics
    useEffect(() => {
        if (syncedLyrics && activeLineRef.current && containerRef.current) {
            activeLineRef.current.scrollIntoView({
                behavior: 'smooth',
                block: 'center',
            });
        }
    }, [currentTime, syncedLyrics]);

    const activeLyricIndex = syncedLyrics 
        ? syncedLyrics.findIndex((line, index) => {
            const nextLine = syncedLyrics[index + 1];
            return currentTime >= line.time && (!nextLine || currentTime < nextLine.time);
        })
        : -1;

    if (isLoading) {
        return (
            <div className="flex-1 flex items-center justify-center text-white/50 animate-pulse">
                Loading lyrics...
            </div>
        );
    }

    if (error) {
        return (
            <div className="flex-1 flex flex-col items-center justify-center text-white/50 text-center">
                <Mic2 size={48} className="mb-4 opacity-50" />
                <p>Lyrics not available for this track.</p>
            </div>
        );
    }

    if (syncedLyrics) {
        return (
            <div ref={containerRef} className="space-y-6 py-[50%] text-center">
                {syncedLyrics.map((line, idx) => (
                    <div 
                        key={idx}
                        ref={idx === activeLyricIndex ? activeLineRef : null}
                        className={`transition-all duration-500 text-2xl md:text-3xl font-bold leading-relaxed px-4 ${
                            idx === activeLyricIndex 
                                ? 'text-white scale-105 origin-center' 
                                : 'text-white/30 blur-[1px] hover:text-white/60 hover:blur-0 cursor-pointer'
                        }`}
                        onClick={() => onSeek(line.time)}
                    >
                        {line.text || "♫"}
                    </div>
                ))}
            </div>
        );
    }

    return (
        <div className="whitespace-pre-wrap text-center text-lg leading-relaxed text-white/80 font-medium pb-10">
            {lyrics}
        </div>
    );
};
