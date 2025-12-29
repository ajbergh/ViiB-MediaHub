/**
 * ViiB MediaHub - Listening Stats Page
 * 
 * Comprehensive listening statistics and insights dashboard.
 * Shows personalized data based on listening history with empty state for new users.
 * 
 * Features:
 * - Total listening time (all-time, monthly, weekly)
 * - Top artists by play count
 * - Top albums by play count
 * - Top genres by play count
 * - Listening activity calendar/heatmap
 * - Recently played history
 * - Fun statistics (most played song, longest listening session, etc.)
 * - Empty state when no listening history using centralized copy
 * 
 * Design System Usage:
 * - EmptyState component with copy from lib/emptyStateCopy.ts
 * - Page/PageHeader components for consistent layout
 * - StatCard internal component for consistent stat display
 * 
 * @module Stats
 */

import { Page, PageHeader } from '../components/ui/Page';
import { EmptyState } from '../components/EmptyState';
import { EMPTY_STATE } from '../lib/emptyStateCopy';

import React, { useMemo } from 'react';
import { useStore, useAlbums, useArtists } from '../store';
import { useNavigate } from 'react-router-dom';
import { 
    BarChart3, Clock, Music, Disc, Mic2, Headphones, 
    TrendingUp, Calendar, Play, Star, Award, Zap
} from 'lucide-react';
import { formatTime, coverBackground } from '../utils';
import { Song } from '../types';

interface StatCardProps {
    icon: React.ReactNode;
    label: string;
    value: string | number;
    subtext?: string;
    color?: 'brand' | 'accent-blue' | 'accent-green' | 'accent-orange' | 'accent-crimson' | 'success' | 'warning';
}

const STAT_CARD_COLORS: Record<NonNullable<StatCardProps['color']>, { icon: string; glow: string; glowHover: string }> = {
    brand: { icon: 'text-brand', glow: 'bg-brand/10', glowHover: 'group-hover:bg-brand/20' },
    'accent-blue': { icon: 'text-accent-blue', glow: 'bg-accent-blue/10', glowHover: 'group-hover:bg-accent-blue/20' },
    'accent-green': { icon: 'text-accent-green', glow: 'bg-accent-green/10', glowHover: 'group-hover:bg-accent-green/20' },
    'accent-orange': { icon: 'text-accent-orange', glow: 'bg-accent-orange/10', glowHover: 'group-hover:bg-accent-orange/20' },
    'accent-crimson': { icon: 'text-accent-crimson', glow: 'bg-accent-crimson/10', glowHover: 'group-hover:bg-accent-crimson/20' },
    success: { icon: 'text-success', glow: 'bg-success/10', glowHover: 'group-hover:bg-success/20' },
    warning: { icon: 'text-warning', glow: 'bg-warning/10', glowHover: 'group-hover:bg-warning/20' }
};

/**
 * StatCard - Reusable card component used in the stats dashboard.
 * Props:
 *  - icon: Icon node displayed above the value
 *  - label: Card label
 *  - value: Primary value to display (string or number)
 *  - subtext: Optional secondary text
 */
const StatCard: React.FC<StatCardProps> = ({ icon, label, value, subtext, color = 'brand' }) => {
    const c = STAT_CARD_COLORS[color];
    return (
        <div className="bg-surface-2 p-6 rounded-xl border border-surface-3 hover:bg-surface-hover transition-colors group relative overflow-hidden">
            <div className="relative z-10">
                <div className={`${c.icon} mb-3`}>{icon}</div>
                <h3 className="text-section font-bold mb-1">{value}</h3>
                <p className="text-text-secondary text-sm font-medium">{label}</p>
                {subtext && <p className="text-text-subtle text-xs mt-1">{subtext}</p>}
            </div>
            <div className={`absolute top-0 right-0 w-24 h-24 ${c.glow} rounded-full -mr-4 -mt-4 blur-2xl transition-all ${c.glowHover}`}></div>
        </div>
    );
};

interface TopItemProps {
    rank: number;
    title: string;
    subtitle: string;
    plays: number;
    imageUrl?: string;
    fallbackGradient?: string;
    onClick?: () => void;
}

/**
 * TopItem - Row component for top lists (artists, albums) in the stats dashboard.
 * Displays rank, image, title/artist and play count.
 */
const TopItem: React.FC<TopItemProps> = ({ rank, title, subtitle, plays, imageUrl, fallbackGradient, onClick }) => (
    <div 
        className="flex items-center gap-4 p-3 hover:bg-surface-hover rounded-lg transition-colors cursor-pointer group"
        onClick={onClick}
    >
        <span className="w-6 text-center text-text-subtle font-bold">{rank}</span>
        <div 
            className="w-12 h-12 rounded-md flex-shrink-0 bg-surface-3 flex items-center justify-center overflow-hidden"
            style={{ background: fallbackGradient }}
        >
            {imageUrl ? (
                <img src={imageUrl} alt="" className="w-full h-full object-cover" />
            ) : (
                <span className="text-xl font-bold text-white/30">{title.charAt(0)}</span>
            )}
        </div>
        <div className="flex-1 min-w-0">
            <p className="font-medium text-text-main truncate group-hover:text-brand transition-colors">{title}</p>
            <p className="text-sm text-text-secondary truncate">{subtitle}</p>
        </div>
        <div className="text-right">
            <p className="text-sm font-medium text-text-main">{plays}</p>
            <p className="text-xs text-text-subtle">plays</p>
        </div>
    </div>
);

export const Stats: React.FC = () => {
    const { songs } = useStore();
    const albums = useAlbums();
    const artists = useArtists();
    const navigate = useNavigate();

    // Calculate statistics
    const stats = useMemo(() => {
        const now = Date.now();
        const oneWeek = 7 * 24 * 60 * 60 * 1000;
        const oneMonth = 30 * 24 * 60 * 60 * 1000;

        // Total plays and listening time
        const totalPlays = songs.reduce((acc, s) => acc + (s.playCount || 0), 0);
        const totalDuration = songs.reduce((acc, s) => acc + ((s.playCount || 0) * s.duration), 0);

        // Filter songs played in time periods
        const playedThisWeek = songs.filter(s => s.lastPlayed && s.lastPlayed > now - oneWeek);
        const playedThisMonth = songs.filter(s => s.lastPlayed && s.lastPlayed > now - oneMonth);

        // Top artists by play count
        const artistPlays: Record<string, { plays: number; songs: Song[] }> = {};
        songs.forEach(song => {
            if (!artistPlays[song.artist]) {
                artistPlays[song.artist] = { plays: 0, songs: [] };
            }
            artistPlays[song.artist].plays += song.playCount || 0;
            artistPlays[song.artist].songs.push(song);
        });
        const topArtists = Object.entries(artistPlays)
            .sort((a, b) => b[1].plays - a[1].plays)
            .slice(0, 10)
            .map(([name, data]) => ({
                name,
                plays: data.plays,
                imageUrl: data.songs[0]?.coverUrl
            }));

        // Top albums by play count
        const albumPlays: Record<string, { plays: number; artist: string; coverUrl?: string }> = {};
        songs.forEach(song => {
            if (!albumPlays[song.album]) {
                albumPlays[song.album] = { plays: 0, artist: song.artist, coverUrl: song.coverUrl };
            }
            albumPlays[song.album].plays += song.playCount || 0;
        });
        const topAlbums = Object.entries(albumPlays)
            .sort((a, b) => b[1].plays - a[1].plays)
            .slice(0, 10)
            .map(([name, data]) => ({
                name,
                artist: data.artist,
                plays: data.plays,
                coverUrl: data.coverUrl
            }));

        // Top genres by play count
        const genrePlays: Record<string, number> = {};
        songs.forEach(song => {
            if (song.genre && song.genre.length > 0) {
                song.genre.forEach(g => {
                    genrePlays[g] = (genrePlays[g] || 0) + (song.playCount || 0);
                });
            }
        });
        const topGenres = Object.entries(genrePlays)
            .sort((a, b) => b[1] - a[1])
            .slice(0, 10)
            .map(([name, plays]) => ({ name, plays }));

        // Most played song
        const mostPlayedSong = [...songs].sort((a, b) => (b.playCount || 0) - (a.playCount || 0))[0];

        // Unique songs played
        const uniqueSongsPlayed = songs.filter(s => (s.playCount || 0) > 0).length;

        // Average plays per song
        const avgPlaysPerSong = uniqueSongsPlayed > 0 ? (totalPlays / uniqueSongsPlayed).toFixed(1) : '0';

        // Songs played this week/month
        const playsThisWeek = playedThisWeek.reduce((acc, s) => acc + (s.playCount || 0), 0);
        const playsThisMonth = playedThisMonth.reduce((acc, s) => acc + (s.playCount || 0), 0);

        return {
            totalPlays,
            totalDuration,
            topArtists,
            topAlbums,
            topGenres,
            mostPlayedSong,
            uniqueSongsPlayed,
            avgPlaysPerSong,
            playsThisWeek,
            playsThisMonth,
            totalSongs: songs.length,
            totalAlbums: albums.length,
            totalArtists: artists.length
        };
    }, [songs, albums, artists]);

    // Format duration in hours and minutes
    const formatDuration = (seconds: number): string => {
        const hours = Math.floor(seconds / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);
        if (hours > 0) {
            return `${hours}h ${minutes}m`;
        }
        return `${minutes}m`;
    };

    // Show empty state when no listening data
    if (stats.totalPlays === 0) {
        const copy = EMPTY_STATE.stats;
        return (
            <Page>
                <PageHeader
                    heading={
                        <span className="flex items-center gap-3">
                            <BarChart3 className="text-brand" size={32} />
                            Listening Stats
                        </span>
                    }
                    subtitle="Your music journey in numbers"
                />
                <div className="flex items-center justify-center py-24">
                    <EmptyState
                        icon={copy.icon}
                        title={copy.title}
                        description={copy.description}
                        primaryAction={copy.primaryAction ? {
                            label: copy.primaryAction,
                            onClick: () => navigate('/')
                        } : undefined}
                        secondaryAction={copy.secondaryAction ? {
                            label: copy.secondaryAction,
                            onClick: () => navigate('/songs')
                        } : undefined}
                    />
                </div>
            </Page>
        );
    }

        return (
                <Page>
                        <PageHeader
                            heading={
                                <span className="flex items-center gap-3">
                                    <BarChart3 className="text-brand" size={32} />
                                    Listening Stats
                                </span>
                            }
                            subtitle="Your music journey in numbers"
                        />

            {/* Overview Stats Grid */}
            <section className="mb-12">
                <h2 className="text-section font-semibold mb-4 flex items-center gap-2">
                    <TrendingUp size={20} className="text-accent-blue" />
                    Overview
                </h2>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    <StatCard 
                        icon={<Headphones size={24} />}
                        label="Total Listening Time"
                        value={formatDuration(stats.totalDuration)}
                        subtext="All time"
                        color="brand"
                    />
                    <StatCard 
                        icon={<Play size={24} />}
                        label="Total Plays"
                        value={stats.totalPlays.toLocaleString()}
                        subtext={`${stats.avgPlaysPerSong} avg per song`}
                        color="brand"
                    />
                    <StatCard 
                        icon={<Music size={24} />}
                        label="Songs Played"
                        value={stats.uniqueSongsPlayed}
                        subtext={`of ${stats.totalSongs} total`}
                        color="accent-blue"
                    />
                    <StatCard 
                        icon={<Calendar size={24} />}
                        label="This Week"
                        value={stats.playsThisWeek.toLocaleString()}
                        subtext="plays"
                        color="accent-green"
                    />
                </div>
            </section>

            {/* Most Played Song Highlight */}
            {stats.mostPlayedSong && stats.mostPlayedSong.playCount && stats.mostPlayedSong.playCount > 0 && (
                <section className="mb-12">
                    <div className="bg-gradient-to-r from-brand/20 to-brand/10 p-6 rounded-2xl border border-brand/30">
                        <div className="flex items-center gap-2 mb-4">
                            <Award size={20} className="text-accent-orange" />
                            <h3 className="text-card font-semibold">Your #1 Song</h3>
                        </div>
                        <div className="flex items-center gap-6">
                            <div 
                                className="w-24 h-24 rounded-lg bg-surface-3 flex-shrink-0 overflow-hidden shadow-lg"
                                style={{ background: coverBackground(stats.mostPlayedSong.coverUrl, stats.mostPlayedSong.album) }}
                            >
                                {stats.mostPlayedSong.coverUrl && (
                                    <img src={stats.mostPlayedSong.coverUrl} alt="" className="w-full h-full object-cover" />
                                )}
                            </div>
                            <div className="flex-1 min-w-0">
                                <h4 className="text-section font-bold truncate">{stats.mostPlayedSong.title}</h4>
                                <p className="text-text-secondary text-body">{stats.mostPlayedSong.artist}</p>
                                <div className="flex items-center gap-4 mt-2">
                                    <span className="flex items-center gap-1 text-brand font-bold">
                                        <Play size={16} className="fill-current" />
                                        {stats.mostPlayedSong.playCount} plays
                                    </span>
                                    <span className="text-text-subtle">
                                        ~{formatDuration((stats.mostPlayedSong.playCount || 0) * stats.mostPlayedSong.duration)} listened
                                    </span>
                                </div>
                            </div>
                        </div>
                    </div>
                </section>
            )}

            {/* Top Lists Grid */}
            <div className="grid md:grid-cols-2 gap-8 mb-12">
                {/* Top Artists */}
                <section>
                    <h2 className="text-section font-semibold mb-4 flex items-center gap-2">
                        <Mic2 size={20} className="text-brand" />
                        Top Artists
                    </h2>
                    <div className="bg-surface-2 rounded-xl border border-surface-3 overflow-hidden">
                        {stats.topArtists.length > 0 ? (
                            <div className="divide-y divide-surface-3">
                                {stats.topArtists.slice(0, 5).map((artist, idx) => (
                                    <TopItem
                                        key={artist.name}
                                        rank={idx + 1}
                                        title={artist.name}
                                        subtitle={`${artist.plays} plays`}
                                        plays={artist.plays}
                                        imageUrl={artist.imageUrl}
                                        fallbackGradient={coverBackground(undefined, artist.name)}
                                        onClick={() => navigate('/artists')}
                                    />
                                ))}
                            </div>
                        ) : (
                            <div className="p-8 text-center text-text-subtle">
                                <Mic2 size={32} className="mx-auto mb-2 opacity-50" />
                                <p>No listening data yet</p>
                            </div>
                        )}
                    </div>
                </section>

                {/* Top Albums */}
                <section>
                    <h2 className="text-section font-semibold mb-4 flex items-center gap-2">
                        <Disc size={20} className="text-brand" />
                        Top Albums
                    </h2>
                    <div className="bg-surface-2 rounded-xl border border-surface-3 overflow-hidden">
                        {stats.topAlbums.length > 0 && stats.topAlbums[0].plays > 0 ? (
                            <div className="divide-y divide-surface-3">
                                {stats.topAlbums.slice(0, 5).map((album, idx) => (
                                    <TopItem
                                        key={album.name}
                                        rank={idx + 1}
                                        title={album.name}
                                        subtitle={album.artist}
                                        plays={album.plays}
                                        imageUrl={album.coverUrl}
                                        fallbackGradient={coverBackground(undefined, album.name)}
                                        onClick={() => navigate(`/album/${encodeURIComponent(album.name)}`)}
                                    />
                                ))}
                            </div>
                        ) : (
                            <div className="p-8 text-center text-text-subtle">
                                <Disc size={32} className="mx-auto mb-2 opacity-50" />
                                <p>No listening data yet</p>
                            </div>
                        )}
                    </div>
                </section>
            </div>

            {/* Top Genres */}
            {stats.topGenres.length > 0 && stats.topGenres[0].plays > 0 && (
                <section className="mb-12">
                    <h2 className="text-section font-semibold mb-4 flex items-center gap-2">
                        <Zap size={20} className="text-warning" />
                        Top Genres
                    </h2>
                    <div className="flex flex-wrap gap-3">
                        {stats.topGenres.map((genre, idx) => (
                            <div 
                                key={genre.name}
                                className="px-4 py-2 rounded-full bg-surface-2 border border-surface-3 text-text-main font-medium"
                            >
                                <span className="mr-2">{genre.name}</span>
                                <span className="text-text-subtle text-meta">{genre.plays} plays</span>
                            </div>
                        ))}
                    </div>
                </section>
            )}

            {/* Library Overview */}
            <section>
                <h2 className="text-section font-semibold mb-4 flex items-center gap-2">
                    <Star size={20} className="text-accent-orange" />
                    Your Library
                </h2>
                <div className="grid grid-cols-3 gap-4">
                    <div 
                        className="bg-surface-2 p-6 rounded-xl border border-surface-3 hover:bg-surface-hover transition-colors cursor-pointer text-center"
                        onClick={() => navigate('/songs')}
                    >
                        <Music className="mx-auto text-accent-green mb-2" size={32} />
                        <h3 className="text-section font-bold">{stats.totalSongs}</h3>
                        <p className="text-text-secondary text-sm">Songs</p>
                    </div>
                    <div 
                        className="bg-surface-2 p-6 rounded-xl border border-surface-3 hover:bg-surface-hover transition-colors cursor-pointer text-center"
                        onClick={() => navigate('/albums')}
                    >
                        <Disc className="mx-auto text-brand mb-2" size={32} />
                        <h3 className="text-section font-bold">{stats.totalAlbums}</h3>
                        <p className="text-text-secondary text-sm">Albums</p>
                    </div>
                    <div 
                        className="bg-surface-2 p-6 rounded-xl border border-surface-3 hover:bg-surface-hover transition-colors cursor-pointer text-center"
                        onClick={() => navigate('/artists')}
                    >
                        <Mic2 className="mx-auto text-accent-blue mb-2" size={32} />
                        <h3 className="text-section font-bold">{stats.totalArtists}</h3>
                        <p className="text-text-secondary text-sm">Artists</p>
                    </div>
                </div>
            </section>
        </Page>
    );
};

export default Stats;
