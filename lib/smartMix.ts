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
const DAY_MS = 24 * 60 * 60 * 1000;

const CHILL_GENRE_RE = /acoustic|folk|jazz|lo-fi|singer-songwriter|chill|ambient|piano|classical/;
const ALT_GENRE_RE = /rock|alternative|grunge|indie|punk/;
const WORKOUT_GENRE_RE = /pop|dance|electronic|hip hop|rock|metal|techno|house|drum and bass|edm|workout|energy|upbeat/;
const FOCUS_GENRE_RE = /instrumental|soundtrack|ambient|classical|jazz|lo-fi|study|focus|piano|score|cinematic/;

interface TopScoreEntry {
    id: string;
    score: number;
    index: number;
}

class TopMixHeap {
    private heap: TopScoreEntry[] = [];

    // Root is the weakest entry in the current top MIX_SIZE (lowest score, or highest index on tie).
    private isWorse(a: TopScoreEntry, b: TopScoreEntry): boolean {
        return a.score < b.score || (a.score === b.score && a.index > b.index);
    }

    push(id: string, score: number, index: number): void {
        if (this.heap.length < MIX_SIZE) {
            this.heap.push({ id, score, index });
            let idx = this.heap.length - 1;
            while (idx > 0) {
                const parent = (idx - 1) >> 1;
                if (this.isWorse(this.heap[idx], this.heap[parent])) {
                    const tmp = this.heap[idx];
                    this.heap[idx] = this.heap[parent];
                    this.heap[parent] = tmp;
                    idx = parent;
                } else {
                    break;
                }
            }
            return;
        }

        if (score <= this.heap[0].score) return;

        this.heap[0] = { id, score, index };
        let idx = 0;
        const len = this.heap.length;
        while (true) {
            const left = (idx << 1) + 1;
            const right = left + 1;
            let worst = idx;
            if (left < len && this.isWorse(this.heap[left], this.heap[worst])) worst = left;
            if (right < len && this.isWorse(this.heap[right], this.heap[worst])) worst = right;
            if (worst === idx) break;
            const tmp = this.heap[idx];
            this.heap[idx] = this.heap[worst];
            this.heap[worst] = tmp;
            idx = worst;
        }
    }

    toIds(): string[] {
        this.heap.sort((a, b) => (b.score - a.score) || (a.index - b.index));
        return this.heap.map(entry => entry.id);
    }
}

function pushReservoir(sample: string[], seen: number, id: string): number {
    const nextSeen = seen + 1;
    if (sample.length < MIX_SIZE) {
        sample.push(id);
    } else {
        const slot = Math.floor(Math.random() * nextSeen);
        if (slot < MIX_SIZE) sample[slot] = id;
    }
    return nextSeen;
}

function shuffleInPlace(ids: string[]): string[] {
    for (let i = ids.length - 1; i > 0; i--) {
        const j = Math.floor(Math.random() * (i + 1));
        [ids[i], ids[j]] = [ids[j], ids[i]];
    }
    return ids;
}

export const generateSmartMixes = (songs: Song[]): SmartMix[] => {
    const now = Date.now();
    const cutoff90 = now - 90 * DAY_MS;
    const cutoff30 = now - 30 * DAY_MS;

    const heavyHeap = new TopMixHeap();
    const rediscoverHeap = new TopMixHeap();
    const freshHeap = new TopMixHeap();

    const chillIds: string[] = [];
    const ninetiesIds: string[] = [];
    const workoutIds: string[] = [];
    const focusIds: string[] = [];
    let chillSeen = 0;
    let ninetiesSeen = 0;
    let workoutSeen = 0;
    let focusSeen = 0;

    for (let i = 0; i < songs.length; i++) {
        const song = songs[i];
        const playCount = song.playCount || 0;
        const lastPlayed = song.lastPlayed || 0;

        if (playCount > 0 && lastPlayed > cutoff90) {
            heavyHeap.push(song.id, playCount, i);
        }
        if (playCount > 2 && lastPlayed < cutoff30) {
            rediscoverHeap.push(song.id, playCount, i);
        }
        freshHeap.push(song.id, song.addedAt, i);

        const rawGenres = song.genre;
        const hasGenre = Boolean(rawGenres && rawGenres.length > 0);
        const genresLower = hasGenre
            ? (rawGenres!.length === 1 ? rawGenres![0] : rawGenres!.join(' ')).toLowerCase()
            : '';

        if (hasGenre) {
            if (CHILL_GENRE_RE.test(genresLower)) {
                chillSeen = pushReservoir(chillIds, chillSeen, song.id);
            }
            if (WORKOUT_GENRE_RE.test(genresLower)) {
                workoutSeen = pushReservoir(workoutIds, workoutSeen, song.id);
            }
            if (FOCUS_GENRE_RE.test(genresLower)) {
                focusSeen = pushReservoir(focusIds, focusSeen, song.id);
            }
        }

        if (song.year && song.year >= 1990 && song.year <= 1999) {
            if (!hasGenre || ALT_GENRE_RE.test(genresLower)) {
                ninetiesSeen = pushReservoir(ninetiesIds, ninetiesSeen, song.id);
            }
        }
    }

    const mixes: SmartMix[] = [
        {
            id: 'heavy-rotation',
            name: 'Heavy Rotation',
            description: 'Your most played tracks recently.',
            coverColors: ['#FF512F', '#DD2476'],
            songIds: heavyHeap.toIds(),
            rules: 'heavy-rotation',
            updatedAt: now
        },
        {
            id: 'rediscover',
            name: 'Rediscover Favorites',
            description: 'Great songs you haven\'t heard in a while.',
            coverColors: ['#8E2DE2', '#4A00E0'],
            songIds: rediscoverHeap.toIds(),
            rules: 'rediscover',
            updatedAt: now
        },
        {
            id: 'fresh-finds',
            name: 'Fresh Finds',
            description: 'Recently added to your library.',
            coverColors: ['#11998e', '#38ef7d'],
            songIds: freshHeap.toIds(),
            rules: 'fresh-finds',
            updatedAt: now
        },
        {
            id: 'chill-acoustic',
            name: 'Chill Acoustic Evening',
            description: 'Mellow, acoustic, and low-energy tracks.',
            coverColors: ['#FDC830', '#F37335'],
            songIds: shuffleInPlace(chillIds),
            rules: 'chill-acoustic',
            updatedAt: now
        },
        {
            id: '90s-alt',
            name: '90s Alternative Mix',
            description: 'Nostalgia trip: Alternative & Rock from the 90s.',
            coverColors: ['#232526', '#414345'],
            songIds: shuffleInPlace(ninetiesIds),
            rules: '90s-alt',
            updatedAt: now
        }
    ];

    if (workoutIds.length > 5) {
        mixes.push({
            id: 'workout-energy',
            name: 'High Energy Workout',
            description: 'Pump up the volume with these energetic tracks.',
            coverColors: ['#FF416C', '#FF4B2B'],
            songIds: shuffleInPlace(workoutIds),
            rules: 'workout-energy',
            updatedAt: now
        });
    }

    if (focusIds.length > 5) {
        mixes.push({
            id: 'focus-mode',
            name: 'Focus Mode',
            description: 'Instrumental and ambient tracks for deep work.',
            coverColors: ['#00c6ff', '#0072ff'],
            songIds: shuffleInPlace(focusIds),
            rules: 'focus-mode',
            updatedAt: now
        });
    }

    return mixes;
};
