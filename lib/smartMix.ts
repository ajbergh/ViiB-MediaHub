/**
 * ViiB MediaHub - Smart Mix Generator
 * 
 * Automatically generates intelligent playlists based on listening habits,
 * metadata, and temporal patterns. Provides personalized music discovery
 * without requiring manual curation.
 * 
 * Generated Mixes:
 * - Heavy Rotation: Most played tracks from last 90 days
 * - Rediscover Favorites: High play count but not heard in 30+ days
 * - Fresh Finds: Recently added to library
 * - Chill Acoustic Evening: Genre-based (acoustic, folk, jazz, ambient)
 * - 90s Alternative Mix: 1990-1999 tracks matching genre keywords, or with no genre tags
 * - High Energy Workout: Upbeat genres (pop, dance, electronic, rock)
 * - Focus Mode: Genre-keyword matches such as instrumental, classical, ambient, or lo-fi
 * 
 * Each generated mix contains up to 50 songs; callers decide when to regenerate it.
 * 
 * @module smartMix
 */

import { Song, SmartMix } from '../types';

const MIX_SIZE = 50;

function topSongIds(songs: Song[], compare: (a: Song, b: Song) => number, include: (song: Song) => boolean = () => true): string[] {
    const top: Song[] = [];
    for (const song of songs) {
        if (!include(song) || (top.length === MIX_SIZE && compare(song, top[MIX_SIZE - 1]) >= 0)) continue;
        let low = 0;
        let high = top.length;
        while (low < high) {
            const middle = Math.floor((low + high) / 2);
            if (compare(song, top[middle]) < 0) high = middle;
            else low = middle + 1;
        }
        if (low < MIX_SIZE) top.splice(low, 0, song);
        if (top.length > MIX_SIZE) top.pop();
    }
    return top.map(song => song.id);
}

function sampleSongIds(songs: Song[], include: (song: Song) => boolean): string[] {
    const sample: string[] = [];
    let seen = 0;
    for (const song of songs) {
        if (!include(song)) continue;
        seen++;
        if (sample.length < MIX_SIZE) sample.push(song.id);
        else {
            const slot = Math.floor(Math.random() * seen);
            if (slot < MIX_SIZE) sample[slot] = song.id;
        }
    }
    for (let i = sample.length - 1; i > 0; i--) {
        const j = Math.floor(Math.random() * (i + 1));
        [sample[i], sample[j]] = [sample[j], sample[i]];
    }
    return sample;
}

export const generateSmartMixes = (songs: Song[]): SmartMix[] => {
    const now = Date.now();
    const daysAgo = (days: number) => now - (days * 24 * 60 * 60 * 1000);
    const mixes: SmartMix[] = [];

    // 1. Heavy Rotation: Most played recently
    const heavyRotationIds = topSongIds(songs, (a, b) => (b.playCount || 0) - (a.playCount || 0),
        song => (song.playCount || 0) > 0 && (song.lastPlayed || 0) > daysAgo(90));

    // Only add if we have enough data or just show empty state later
    mixes.push({
        id: 'heavy-rotation',
        name: 'Heavy Rotation',
        description: 'Your most played tracks recently.',
        coverColors: ['#FF512F', '#DD2476'],
        songIds: heavyRotationIds,
        rules: 'heavy-rotation',
        updatedAt: now
    });

    // 2. Rediscover Favorites: High play count but old lastPlayed
    const rediscoverIds = topSongIds(songs, (a, b) => (b.playCount || 0) - (a.playCount || 0),
        song => (song.playCount || 0) > 2 && (song.lastPlayed || 0) < daysAgo(30));

    mixes.push({
        id: 'rediscover',
        name: 'Rediscover Favorites',
        description: 'Great songs you haven\'t heard in a while.',
        coverColors: ['#8E2DE2', '#4A00E0'],
        songIds: rediscoverIds,
        rules: 'rediscover',
        updatedAt: now
    });

    // 3. Fresh Finds: Recently added
    const freshFindsIds = topSongIds(songs, (a, b) => b.addedAt - a.addedAt);

    mixes.push({
        id: 'fresh-finds',
        name: 'Fresh Finds',
        description: 'Recently added to your library.',
        coverColors: ['#11998e', '#38ef7d'],
        songIds: freshFindsIds,
        rules: 'fresh-finds',
        updatedAt: now
    });

    // 4. Chill Acoustic Evening: Genre based
    const chillKeywords = ['acoustic', 'folk', 'jazz', 'lo-fi', 'singer-songwriter', 'chill', 'ambient', 'piano', 'classical'];
    const chillIds = sampleSongIds(songs, s => {
        if (!s.genre || s.genre.length === 0) return false;
        const genres = s.genre.join(' ').toLowerCase();
        return chillKeywords.some(k => genres.includes(k));
    });
    
    mixes.push({
        id: 'chill-acoustic',
        name: 'Chill Acoustic Evening',
        description: 'Mellow, acoustic, and low-energy tracks.',
        coverColors: ['#FDC830', '#F37335'],
        songIds: chillIds,
        rules: 'chill-acoustic',
        updatedAt: now
    });

    // 5. 90s Alternative Mix: Year + Genre
    const altKeywords = ['rock', 'alternative', 'grunge', 'indie', 'punk'];
    const ninetiesIds = sampleSongIds(songs, s => {
        const is90s = s.year && s.year >= 1990 && s.year <= 1999;
        if (!is90s) return false;
        // If year matches, check genre if available
        if (!s.genre || s.genre.length === 0) return true;
        const genres = s.genre.join(' ').toLowerCase();
        return altKeywords.some(k => genres.includes(k));
    });

    mixes.push({
        id: '90s-alt',
        name: '90s Alternative Mix',
        description: 'Nostalgia trip: Alternative & Rock from the 90s.',
        coverColors: ['#232526', '#414345'],
        songIds: ninetiesIds,
        rules: '90s-alt',
        updatedAt: now
    });

    // 6. High Energy Workout: Upbeat genres
    const workoutKeywords = ['pop', 'dance', 'electronic', 'hip hop', 'rock', 'metal', 'techno', 'house', 'drum and bass', 'edm', 'workout', 'energy', 'upbeat'];
    const workoutIds = sampleSongIds(songs, s => {
        if (!s.genre || s.genre.length === 0) return false;
        const genres = s.genre.join(' ').toLowerCase();
        return workoutKeywords.some(k => genres.includes(k));
    });

    if (workoutIds.length > 5) {
        mixes.push({
            id: 'workout-energy',
            name: 'High Energy Workout',
            description: 'Pump up the volume with these energetic tracks.',
            coverColors: ['#FF416C', '#FF4B2B'],
            songIds: workoutIds,
            rules: 'workout-energy',
            updatedAt: now
        });
    }

    // 7. Focus Mode: Instrumental and calm
    const focusKeywords = ['instrumental', 'soundtrack', 'ambient', 'classical', 'jazz', 'lo-fi', 'study', 'focus', 'piano', 'score', 'cinematic'];
    const focusIds = sampleSongIds(songs, s => {
        if (!s.genre || s.genre.length === 0) return false;
        const genres = s.genre.join(' ').toLowerCase();
        return focusKeywords.some(k => genres.includes(k));
    });

    if (focusIds.length > 5) {
        mixes.push({
            id: 'focus-mode',
            name: 'Focus Mode',
            description: 'Instrumental and ambient tracks for deep work.',
            coverColors: ['#00c6ff', '#0072ff'],
            songIds: focusIds,
            rules: 'focus-mode',
            updatedAt: now
        });
    }

    return mixes;
};
