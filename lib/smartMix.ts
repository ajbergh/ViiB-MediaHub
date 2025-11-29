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
 * - 90s Alternative Mix: Year + genre filtering (1990-1999 rock/alternative)
 * 
 * Each mix contains up to 50 songs and updates when library changes.
 * 
 * @module smartMix
 */

import { Song, SmartMix } from '../types';

export const generateSmartMixes = (songs: Song[]): SmartMix[] => {
    const now = Date.now();
    const daysAgo = (days: number) => now - (days * 24 * 60 * 60 * 1000);
    const mixes: SmartMix[] = [];

    // 1. Heavy Rotation: Most played recently
    const heavyRotationIds = [...songs]
        .filter(s => (s.playCount || 0) > 0 && (s.lastPlayed || 0) > daysAgo(90))
        .sort((a, b) => (b.playCount || 0) - (a.playCount || 0))
        .slice(0, 50)
        .map(s => s.id);

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
    const rediscoverIds = [...songs]
        .filter(s => (s.playCount || 0) > 2 && (s.lastPlayed || 0) < daysAgo(30))
        .sort((a, b) => (b.playCount || 0) - (a.playCount || 0))
        .slice(0, 50)
        .map(s => s.id);
    
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
    const freshFindsIds = [...songs]
        .sort((a, b) => b.addedAt - a.addedAt)
        .slice(0, 50)
        .map(s => s.id);

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
    const chillIds = [...songs]
        .filter(s => {
            if (!s.genre || s.genre.length === 0) return false;
            const genres = s.genre.join(' ').toLowerCase();
            return chillKeywords.some(k => genres.includes(k));
        })
        .sort(() => 0.5 - Math.random()) // Shuffle
        .slice(0, 50)
        .map(s => s.id);
    
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
    const ninetiesIds = [...songs]
        .filter(s => {
            const is90s = s.year && s.year >= 1990 && s.year <= 1999;
            if (!is90s) return false;
            // If year matches, check genre if available
            if (!s.genre || s.genre.length === 0) return true; 
            const genres = s.genre.join(' ').toLowerCase();
            return altKeywords.some(k => genres.includes(k));
        })
        .sort(() => 0.5 - Math.random())
        .slice(0, 50)
        .map(s => s.id);

    mixes.push({
        id: '90s-alt',
        name: '90s Alternative Mix',
        description: 'Nostalgia trip: Alternative & Rock from the 90s.',
        coverColors: ['#232526', '#414345'],
        songIds: ninetiesIds,
        rules: '90s-alt',
        updatedAt: now
    });

    return mixes;
};