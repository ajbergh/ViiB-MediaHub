/**
 * ViiB MediaHub - Listening Stats Page
 *
 * Displays catalog-derived play totals, estimated all-time listening duration,
 * unique played songs, top track/artists/albums/genres and library counts.
 * The weekly card sums lifetime play counts for songs last played within seven
 * days; it is not an event-level weekly listening-time calculation. Shows an
 * empty state when the library has no recorded plays.
 */

import { Page, PageHeader } from '../components/ui/Page';
import { EmptyState } from '../components/EmptyState';
import { EMPTY_STATE } from '../lib/emptyStateCopy';

import React, { useMemo } from 'react';
import { useStore, useAlbums, useArtists } from '../store';
import { useNavigate } from 'react-router';
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

const STAT_CARD_COLORS: Record<NonNullable<StatCardProps['color']>, { icon: string }> = {
    brand: { icon: 'text-brand' },
    'accent-blue': { icon: 'text-accent-blue' },
    'accent-green': { icon: 'text-accent-green' },
    'accent-orange': { icon: 'text-accent-orange' },
    'accent-crimson': { icon: 'text-accent-crimson' },
    success: { icon: 'text-success' },
    warning: { icon: 'text-warning' }
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
        <div className="bg-surface-2 p-5 rounded-lg hover:bg-surface-hover transition-colors group">
            <div className={`${c.icon} mb-3`}>{icon}</div>
            <h3 className="text-section font-bold text-text-main mb-0.5">{value}</h3>
            <p className="text-text-secondary text-sm font-medium">{label}</p>
            {subtext && <p className="text-text-subtle text-xs mt-1">{subtext}</p>}
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
        className="flex items-center gap-4 p-2.5 hover:bg-surface-hover rounded-md transition-colors cursor-pointer group"
        onClick={onClick}
    >
        <span className="w-6 text-center text-text-subtle text-sm font-semibold">{rank}</span>
        <div 
            className="w-11 h-11 rounded flex-shrink-0 bg-surface-3 flex items-center justify-center overflow-hidden shadow-sm"
            style={{ background: fallbackGradient }}
        >
            {imageUrl ? (
                <img src={imageUrl} alt="" className="w-full h-full object-cover" />
            ) : (
                <span className="text-lg font-bold text-text-main/30">{title.charAt(0)}</span>
            )}
        </div>
        <div className="flex-1 min-w-0">
            <p className="font-medium text-sm text-text-main truncate group-hover:text-brand transition-colors">{title}</p>
            <p className="text-xs text-text-secondary truncate">{subtitle}</p>
        </div>
        <div className="text-right">
            <p className="text-sm font-semibold text-text-main">{plays}</p>
            <p className="text-[11px] text-text-subtle">plays</p>
        </div>
    </div>
);

export const Stats: React.FC = () => {
    const songs = useStore(state => state.songs);
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
                    heading="Listening Stats"
                    subtitle="Your listening activity and top music"
                />
                <div className="flex items-center justify-center py-24">
                    <EmptyState
                        icon={<copy.icon size={48} />}
                        title={copy.title}
                        description={copy.description}
                        action={copy.primaryAction ? {
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
                            heading="Listening Stats"
                            subtitle="Your listening activity and top music"
                        />

            {/* Overview Stats Grid */}
            <section className="mb-10">
                <h2 className="text-section font-bold mb-4">
                    Overview
                </h2>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    <StatCard 
                        icon={<Headphones size={22} />}
                        label="Total Listening Time"
                        value={formatDuration(stats.totalDuration)}
                        subtext="All time"
                        color="brand"
                    />
                    <StatCard 
                        icon={<Play size={22} />}
                        label="Total Plays"
                        value={stats.totalPlays.toLocaleString()}
                        subtext={`${stats.avgPlaysPerSong} avg per song`}
                        color="brand"
                    />
                    <StatCard 
                        icon={<Music size={22} />}
                        label="Songs Played"
                        value={stats.uniqueSongsPlayed}
                        subtext={`of ${stats.totalSongs} total`}
                        color="accent-blue"
                    />
                    <StatCard 
                        icon={<Calendar size={22} />}
                        label="This Week"
                        value={stats.playsThisWeek.toLocaleString()}
                        subtext="plays"
                        color="accent-green"
                    />
                </div>
            </section>

            {/* Most Played Song Highlight */}
            {stats.mostPlayedSong && stats.mostPlayedSong.playCount && stats.mostPlayedSong.playCount > 0 && (
                <section className="mb-10">
                    <div className="bg-gradient-to-br from-surface-3 to-surface-2 p-6 rounded-lg">
                        <div className="flex items-center gap-2 mb-4">
                            <Award size={18} className="text-brand" />
                            <h3 className="text-xs font-bold uppercase tracking-widest text-text-secondary">Top Track</h3>
                        </div>
                        <div className="flex items-center gap-5">
                            <div 
                                className="w-24 h-24 rounded-md bg-surface-3 flex-shrink-0 overflow-hidden shadow-xl"
                                style={{ background: coverBackground(stats.mostPlayedSong.coverUrl, stats.mostPlayedSong.album) }}
                            >
                                {stats.mostPlayedSong.coverUrl && (
                                    <img src={stats.mostPlayedSong.coverUrl} alt="" className="w-full h-full object-cover" />
                                )}
                            </div>
                            <div className="flex-1 min-w-0">
                                <h4 className="text-section font-bold text-text-main truncate">{stats.mostPlayedSong.title}</h4>
                                <p className="text-text-secondary text-sm mt-0.5">{stats.mostPlayedSong.artist}</p>
                                <div className="flex items-center gap-4 mt-2.5 text-sm">
                                    <span className="flex items-center gap-1.5 text-brand font-bold">
                                        <Play size={14} className="fill-current" />
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
                                        onClick={() => navigate(`/album/${encodeURIComponent(album.name)}/${encodeURIComponent(album.artist)}`)}
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
