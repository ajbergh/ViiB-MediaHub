/**
 * Spotify Web API Integration Service
 * 
 * Normal catalog requests use the backend cookie session.
 * Used for searching Spotify catalog, fetching metadata, and enhancing
 * local library information.
 * 
 * Authentication Flow:
 * 1. User initiates login -> startAuth() generates code verifier/challenge
 * 2. User redirects to Spotify authorization page
 * 3. Spotify redirects back with authorization code
 * 4. handleCallback() exchanges code for access/refresh tokens
 * 5. Tokens stored in backend via api.saveSpotifyCredentials()
 * 
 * Features:
 * - OAuth 2.0 with PKCE (no client secret exposed to frontend)
 * - Backend-owned automatic token renewal
 * - Request queuing to respect rate limits (200ms between requests)
 * - Typed error handling (SpotifyAuthError, SpotifyRateLimitError, etc.)
 * - Fuzzy string matching for artist/album metadata
 * - Levenshtein distance algorithm for improved matching accuracy
 * 
 * The access tokens are also used by the backend for librespot downloads,
 * providing seamless integration between Web API and download functionality.
 */

import { backendSpotifyFetch, assertSpotifySession, SpotifySessionChangedError } from './spotifyBackend';
import { ArtistMetadata, AlbumMetadata, SpotifyProfile } from '../types';
import { useStore } from '../store';
import { isWailsEnvironment } from '../utils';
import { SpotifyAuthError, SpotifyRateLimitError, SpotifyApiError, SpotifyNetworkError } from '../lib/spotifyErrors';

const AUTH_URL = 'https://accounts.spotify.com/authorize';
const TOKEN_URL = 'https://accounts.spotify.com/api/token';
const API_BASE = '';

// Request queue to prevent flooding the API and respect rate limits
// The backend enforces provider cooldowns; this queue spaces renderer work
let requestQueue: (() => Promise<void>)[] = [];
let isProcessingQueue = false;

const processQueue = async () => {
    if (isProcessingQueue || requestQueue.length === 0) return;
    isProcessingQueue = true;

    const task = requestQueue.shift();
    if (task) {
        try {
            await task();
        } catch (e) {
            console.warn('Queue task failed', e);
        }
        // Respect rate limits roughly
        setTimeout(() => {
            isProcessingQueue = false;
            processQueue();
        }, 200);
    } else {
        isProcessingQueue = false;
    }
};

const enqueue = <T>(task: () => Promise<T>): Promise<T> => {
    const generation = useStore.getState().spotifySessionGeneration;
    return new Promise((resolve, reject) => {
        requestQueue.push(async () => {
            try {
                assertSpotifySession(generation);
                const result = await task();
                assertSpotifySession(generation);
                resolve(result);
            } catch (error) {
                reject(error);
            }
        });
        processQueue();
    });
};

// User searches must not wait for profile requests or background enrichment.
// The backend bounds catalog concurrency and enforces the shared cooldown.
const runInteractive = async <T>(task: () => Promise<T>, signal?: AbortSignal): Promise<T> => {
    const generation = useStore.getState().spotifySessionGeneration;
    signal?.throwIfAborted();
    assertSpotifySession(generation);
    const result = await task();
    signal?.throwIfAborted();
    assertSpotifySession(generation);
    return result;
};

export interface SpotifySearchOptions {
    signal?: AbortSignal;
    /** Publish catalog results before optional playlist scraping finishes. */
    onCatalogResults?: (results: any) => void;
}

// --- Helper Functions ---

/**
 * Generates a cryptographically secure random string for PKCE.
 * Used to generate code_verifier for OAuth PKCE flow.
 * 
 * @param length - Length of random string to generate
 * @returns Random alphanumeric string
 */
const generateRandomString = (length: number) => {
    const possible = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    const values = crypto.getRandomValues(new Uint8Array(length));
    return values.reduce((acc, x) => acc + possible[x % possible.length], "");
}

/**
 * Computes SHA-256 hash of input string.
 * Used to generate code_challenge from code_verifier for PKCE.
 * 
 * @param plain - Plain text to hash
 * @returns Browser digest bytes or a base64url challenge from the native runtime
 */
const sha256 = async (plain: string) => {
    if (isWailsEnvironment()) {
        const nativeHash = (window as Window & {
            go?: { main?: { App?: { GenerateSpotifyCodeChallenge?: (verifier: string) => Promise<string> } } };
        }).go?.main?.App?.GenerateSpotifyCodeChallenge;
        if (nativeHash) {
            // Native Wails pages, including macOS wails://wails, do not need
            // WebCrypto's secure-context support to create a PKCE challenge.
            return nativeHash(plain);
        }
    }
    if (!window.crypto?.subtle) {
        throw new Error(isWailsEnvironment()
            ? 'The desktop Spotify sign-in bridge is unavailable. Restart ViiB and try again.'
            : 'Secure hashing is unavailable in this browser. Open ViiB on localhost or HTTPS.');
    }
    const encoder = new TextEncoder();
    const data = encoder.encode(plain);
    return window.crypto.subtle.digest('SHA-256', data);
}

/**
 * Base64-URL encodes an ArrayBuffer.
 * Removes padding and replaces characters for URL safety (RFC 7636).
 * 
 * @param input - ArrayBuffer to encode
 * @returns Base64-URL encoded string
 */
const base64encode = (input: ArrayBuffer) => {
    return btoa(String.fromCharCode(...new Uint8Array(input)))
        .replace(/=/g, '')
        .replace(/\+/g, '-')
        .replace(/\//g, '_');
}

/**
 * Normalizes track/album names for comparison.
 * Removes parenthetical content, remaster/deluxe labels, special characters.
 * Used for fuzzy matching between Spotify results and local library.
 * 
 * @param str - String to clean
 * @returns Normalized lowercase string
 */
const cleanName = (str: string): string => {
    return str
        .replace(/[\(\[].*?[\)\]]/g, '') 
        .replace(/\b(remaster|remastered|deluxe|edition|version|feat|ft\.|vol\.|volume)\b.*$/i, '')
        .replace(/[^\w\s]/gi, '')
        .replace(/\s+/g, ' ')
        .trim()
        .toLowerCase();
};

/**
 * Calculates Levenshtein distance between two strings.
 * Used for fuzzy matching to find best Spotify match for local artists/albums.
 * 
 * The Levenshtein distance is the minimum number of single-character edits
 * (insertions, deletions, or substitutions) required to change one string
 * into another. Lower distance = more similar strings.
 * 
 * @param s1 - First string to compare
 * @param s2 - Second string to compare
 * @returns Edit distance between strings (0 = identical)
 */
const levenshteinDistance = (s1: string, s2: string): number => {
    const len1 = s1.length;
    const len2 = s2.length;
    
    // Create a 2D array for dynamic programming
    const dp: number[][] = Array(len1 + 1).fill(null).map(() => Array(len2 + 1).fill(0));
    
    // Initialize first row and column
    for (let i = 0; i <= len1; i++) dp[i][0] = i;
    for (let j = 0; j <= len2; j++) dp[0][j] = j;
    
    // Fill the matrix
    for (let i = 1; i <= len1; i++) {
        for (let j = 1; j <= len2; j++) {
            const cost = s1[i - 1] === s2[j - 1] ? 0 : 1;
            dp[i][j] = Math.min(
                dp[i - 1][j] + 1,      // deletion
                dp[i][j - 1] + 1,      // insertion
                dp[i - 1][j - 1] + cost // substitution
            );
        }
    }
    
    return dp[len1][len2];
};

const getSimilarity = (s1: string, s2: string): number => {
    const n1 = cleanName(s1);
    const n2 = cleanName(s2);
    
    // Exact match
    if (n1 === n2) return 1.0;
    
    // Empty strings
    if (!n1 || !n2) return 0.0;
    
    // Substring match (high score but not perfect)
    if (n1.includes(n2) || n2.includes(n1)) {
        // Calculate how much of the longer string is matched
        const shorter = n1.length < n2.length ? n1 : n2;
        const longer = n1.length >= n2.length ? n1 : n2;
        return 0.85 + (0.1 * (shorter.length / longer.length));
    }
    
    // Levenshtein distance-based similarity
    const distance = levenshteinDistance(n1, n2);
    const maxLen = Math.max(n1.length, n2.length);
    
    // Convert distance to similarity score (0 to 1)
    const similarity = 1 - (distance / maxLen);
    
    return similarity;
};

// --- Service Implementation ---

export const SpotifyService = {
    async generateAuthUrl(clientId: string, redirectUri: string) {
        const codeVerifier = generateRandomString(64);
        const hashed = await sha256(codeVerifier);
        const codeChallenge = typeof hashed === 'string' ? hashed : base64encode(hashed);
        const state = generateRandomString(32);

        const scopes = [
            'streaming',
            'user-read-email',
            'user-read-private',
            'user-library-read',
            'user-read-playback-state',
            'user-modify-playback-state',
            'playlist-read-private'
        ].join(' ');
        
        const params = new URLSearchParams({
            response_type: 'code',
            client_id: clientId,
            scope: scopes,
            redirect_uri: redirectUri,
            code_challenge_method: 'S256',
            code_challenge: codeChallenge,
            state
        });

        const url = `${AUTH_URL}?${params.toString()}`;
        
        // Log for debugging
        console.log('[SpotifyService] Generated Auth URL:', url);
        console.log('[SpotifyService] Redirect URI:', redirectUri);
        console.log('[SpotifyService] Client ID:', clientId);
        
        return {
            url,
            codeVerifier,
            state
        };
    },

    async exchangeCode(clientId: string, _clientSecret: string, code: string, redirectUri: string, codeVerifier: string) {
        const bodyParams: any = {
            grant_type: 'authorization_code',
            code,
            redirect_uri: redirectUri,
            client_id: clientId,
            code_verifier: codeVerifier
        };

        const headers: any = {
            'Content-Type': 'application/x-www-form-urlencoded'
        };


        try {
            const response = await fetch(TOKEN_URL, {
                method: 'POST',
                headers,
                body: new URLSearchParams(bodyParams)
            });

            if (!response.ok) {
                const err = await response.json().catch(() => ({ error_description: 'Unknown error' }));
                throw new SpotifyAuthError(
                    err.error_description || 'Failed to exchange authorization code',
                    response.status
                );
            }

            return response.json(); // { access_token, refresh_token, expires_in, ... }
        } catch (error) {
            if (error instanceof SpotifyAuthError) {
                throw error;
            }
            throw new SpotifyNetworkError('Network error during code exchange', error);
        }
    },

    async getUserProfile(signal?: AbortSignal): Promise<SpotifyProfile | null> {
        return enqueue(async () => {
            
            try {
                const res = await backendSpotifyFetch(`${API_BASE}/me`, signal);
                
                if (res.status === 401) {
                    throw new SpotifyAuthError('Unauthorized - token may be invalid', res.status);
                }
                
                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while fetching profile',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }
                
                if (res.ok) {
                    return await res.json();
                }
                
                throw new SpotifyApiError(
                    'Failed to fetch user profile',
                    res.status,
                    await res.json().catch(() => ({}))
                );
            } catch (error) {
                if (error instanceof SpotifyAuthError || error instanceof SpotifyRateLimitError || error instanceof SpotifyApiError) {
                    throw error;
                }
                throw new SpotifyNetworkError('Network error fetching user profile', error);
            }
        });
    },

    async searchArtist(artistName: string): Promise<ArtistMetadata | null> {
        return enqueue(async () => {
            const store = useStore.getState();

            try {
                const query = encodeURIComponent(artistName);
                const res = await backendSpotifyFetch(`${API_BASE}/search?q=${query}&type=artist&limit=3`);

                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while searching artist',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (!res.ok) {
                    throw new SpotifyApiError(
                        `Failed to search for artist: ${artistName}`,
                        res.status
                    );
                }

                const data = await res.json();
                if (!data.artists || data.artists.items.length === 0) {
                    return null;
                }

                const match = data.artists.items.find((a: any) => a && getSimilarity(a.name, artistName) > 0.8);
                if (!match) return null;

                const imageUrl = match.images && match.images.length > 0 ? match.images[0].url : '';

                return {
                    spotifyId: match.id,
                    name: match.name,
                    imageUrl,
                    url: match.external_urls.spotify,
                    fetchedAt: Date.now()
                };

            } catch (error) {
                if (error instanceof SpotifyAuthError || error instanceof SpotifyRateLimitError || error instanceof SpotifyApiError) {
                    store.addLog('error', `Spotify Artist Search Error: ${artistName}`, error);
                    throw error;
                }
                store.addLog('error', `Spotify Artist Search Error: ${artistName}`, error);
                throw new SpotifyNetworkError(`Network error searching for artist: ${artistName}`, error);
            }
        });
    },

    async searchAlbum(albumName: string, artistName: string): Promise<AlbumMetadata | null> {
        return enqueue(async () => {
            const store = useStore.getState();

            try {
                const query = `album:${cleanName(albumName)} artist:${cleanName(artistName)}`;
                const res = await backendSpotifyFetch(`${API_BASE}/search?q=${encodeURIComponent(query)}&type=album&limit=5`);

                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while searching album',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (!res.ok) {
                    throw new SpotifyApiError(
                        `Failed to search for album: ${albumName}`,
                        res.status
                    );
                }

                const data = await res.json();
                if (!data.albums || data.albums.items.length === 0) {
                    return null;
                }

                const album = data.albums.items.find((item: any) => item);
                if (!album) return null;
                
                const fullAlbumRes = await backendSpotifyFetch(`${API_BASE}/albums/${album.id}`);

                if (fullAlbumRes.status === 429) {
                    const retryAfter = fullAlbumRes.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while fetching album details',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (!fullAlbumRes.ok) {
                    throw new SpotifyApiError(
                        `Failed to fetch album details for: ${album.id}`,
                        fullAlbumRes.status
                    );
                }

                const fullAlbum = await fullAlbumRes.json();

                const coverUrl = fullAlbum.images && fullAlbum.images.length > 0 ? fullAlbum.images[0].url : '';
                const copyright = fullAlbum.copyrights && fullAlbum.copyrights.length > 0 ? fullAlbum.copyrights[0].text : '';

                return {
                    spotifyId: fullAlbum.id,
                    name: fullAlbum.name,
                    artist: fullAlbum.artists[0].name,
                    coverUrl,
                    description: `Released ${fullAlbum.release_date}. ${fullAlbum.total_tracks} tracks.`, 
                    genre: fullAlbum.genres && fullAlbum.genres.length > 0 ? fullAlbum.genres[0] : '',
                    releaseDate: fullAlbum.release_date,
                    url: fullAlbum.external_urls.spotify,
                    copyright,
                    fetchedAt: Date.now()
                };

            } catch (error) {
                if (error instanceof SpotifyAuthError || error instanceof SpotifyRateLimitError || error instanceof SpotifyApiError) {
                    store.addLog('error', `Spotify Album Search Error: ${albumName}`, error);
                    throw error;
                }
                store.addLog('error', `Spotify Album Search Error: ${albumName}`, error);
                throw new SpotifyNetworkError(`Network error searching for album: ${albumName}`, error);
            }
        });
    },

    async search(
        query: string, 
        types: string[] = ['album', 'playlist', 'track', 'artist'],
        limit: number = 20,
        offset: number = 0,
        options: SpotifySearchOptions = {}
    ): Promise<any> {
        return runInteractive(async () => {
            const store = useStore.getState();
            const generation = store.spotifySessionGeneration;

            try {
                const typeStr = types.join(',');
                const params = new URLSearchParams({
                    q: query,
                    type: typeStr,
                    limit: limit.toString(),
                    offset: offset.toString()
                });
                
                const res = await backendSpotifyFetch(`${API_BASE}/search?${params.toString()}`, options.signal);

                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited during search',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (!res.ok) {
                    throw new SpotifyApiError(
                        `Spotify search failed: ${res.statusText}`,
                        res.status
                    );
                }

                const results = await res.json();
                options.signal?.throwIfAborted();
                assertSpotifySession(generation);
                options.onCatalogResults?.(results);
                
                // If searching for playlists and this is the first page (offset 0),
                // enhance results with fallback scraper for first-party playlists
                if (types.includes('playlist') && offset === 0) {
                    try {
                        const { api } = await import('./api');
                        options.signal?.throwIfAborted();
                        assertSpotifySession(generation);
                        const fallbackResults = await api.searchPlaylistsFallback(query, options.signal);
                        options.signal?.throwIfAborted();
                        assertSpotifySession(generation);
                        
                        if (fallbackResults?.playlists?.items?.length > 0) {
                            // Get existing playlist IDs from API results
                            const existingIds = new Set(
                                (results.playlists?.items || []).map((p: any) => p?.id).filter(Boolean)
                            );
                            
                            // Add unique playlists from fallback (first-party playlists)
                            const newPlaylists = fallbackResults.playlists.items.filter(
                                (p: any) => p?.id && !existingIds.has(p.id)
                            );
                            
                            if (newPlaylists.length > 0) {
                                console.log(`[SpotifyService] Added ${newPlaylists.length} playlists from fallback search`);
                                // Prepend first-party playlists as they're usually more relevant
                                results.playlists = {
                                    ...results.playlists,
                                    items: [...newPlaylists, ...(results.playlists?.items || [])],
                                    total: (results.playlists?.total || 0) + newPlaylists.length
                                };
                            }
                        }
                    } catch (fallbackError) {
                        if (options.signal?.aborted || fallbackError instanceof SpotifySessionChangedError) throw fallbackError;
                        // Don't fail the entire search if fallback fails
                        console.warn('[SpotifyService] Fallback playlist search failed:', fallbackError);
                    }
                }
                
                return results;
            } catch (error) {
                if (options.signal?.aborted || error instanceof SpotifySessionChangedError) throw error;
                if (error instanceof SpotifyAuthError || error instanceof SpotifyRateLimitError || error instanceof SpotifyApiError) {
                    store.addLog('error', `Spotify Search Error: ${query}`, error);
                    throw error;
                }
                store.addLog('error', `Spotify Search Error: ${query}`, error);
                throw new SpotifyNetworkError(`Network error during search: ${query}`, error);
            }
        }, options.signal);
    },

    async getRecentlyPlayed(limit: number = 20): Promise<any> {
        return enqueue(async () => {
            const store = useStore.getState();

            try {
                const res = await backendSpotifyFetch(`${API_BASE}/me/player/recently-played?limit=${limit}`);

                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while fetching recently played',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (res.status === 401) {
                    throw new SpotifyAuthError('Unauthorized - requires user authentication', res.status);
                }

                if (!res.ok) {
                    throw new SpotifyApiError(
                        `Failed to fetch recently played: ${res.statusText}`,
                        res.status
                    );
                }

                return await res.json();
            } catch (error) {
                if (error instanceof SpotifyAuthError || error instanceof SpotifyRateLimitError || error instanceof SpotifyApiError) {
                    store.addLog('error', 'Spotify Recently Played Error', error);
                    throw error;
                }
                store.addLog('error', 'Spotify Recently Played Error', error);
                throw new SpotifyNetworkError('Network error fetching recently played', error);
            }
        });
    },

    async getSavedAlbums(limit: number = 20, offset: number = 0): Promise<any> {
        return enqueue(async () => {
            const store = useStore.getState();

            try {
                const params = new URLSearchParams({
                    limit: limit.toString(),
                    offset: offset.toString()
                });

                const res = await backendSpotifyFetch(`${API_BASE}/me/albums?${params.toString()}`);

                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while fetching saved albums',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (res.status === 401) {
                    throw new SpotifyAuthError('Unauthorized - requires user authentication', res.status);
                }

                if (!res.ok) {
                    throw new SpotifyApiError(
                        `Failed to fetch saved albums: ${res.statusText}`,
                        res.status
                    );
                }

                return await res.json();
            } catch (error) {
                if (error instanceof SpotifyAuthError || error instanceof SpotifyRateLimitError || error instanceof SpotifyApiError) {
                    store.addLog('error', 'Spotify Saved Albums Error', error);
                    throw error;
                }
                store.addLog('error', 'Spotify Saved Albums Error', error);
                throw new SpotifyNetworkError('Network error fetching saved albums', error);
            }
        });
    },

    async getSavedPlaylists(limit: number = 20, offset: number = 0): Promise<any> {
        return enqueue(async () => {
            const store = useStore.getState();

            try {
                const params = new URLSearchParams({
                    limit: limit.toString(),
                    offset: offset.toString()
                });

                const res = await backendSpotifyFetch(`${API_BASE}/me/playlists?${params.toString()}`);

                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while fetching saved playlists',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (res.status === 401) {
                    throw new SpotifyAuthError('Unauthorized - requires user authentication', res.status);
                }

                if (!res.ok) {
                    throw new SpotifyApiError(
                        `Failed to fetch saved playlists: ${res.statusText}`,
                        res.status
                    );
                }

                return await res.json();
            } catch (error) {
                if (error instanceof SpotifyAuthError || error instanceof SpotifyRateLimitError || error instanceof SpotifyApiError) {
                    store.addLog('error', 'Spotify Saved Playlists Error', error);
                    throw error;
                }
                store.addLog('error', 'Spotify Saved Playlists Error', error);
                throw new SpotifyNetworkError('Network error fetching saved playlists', error);
            }
        });
    },

    /**
     * Get an artist's top tracks.
     * Returns the artist's most popular tracks for streaming.
     * 
     * @param artistId - Spotify artist ID
     * @param market - ISO 3166-1 alpha-2 country code (defaults to US)
     * @returns Promise resolving to artist's top tracks
     */
    async getArtistTopTracks(artistId: string, market: string = 'US'): Promise<any> {
        return enqueue(async () => {
            const store = useStore.getState();

            try {
                const res = await backendSpotifyFetch(`${API_BASE}/artists/${artistId}/top-tracks?market=${market}`);

                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while fetching artist top tracks',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (res.status === 401) {
                    throw new SpotifyAuthError('Unauthorized - requires user authentication', res.status);
                }

                if (!res.ok) {
                    throw new SpotifyApiError(
                        `Failed to fetch artist top tracks: ${res.statusText}`,
                        res.status
                    );
                }

                return await res.json();
            } catch (error) {
                if (error instanceof SpotifyAuthError || error instanceof SpotifyRateLimitError || error instanceof SpotifyApiError) {
                    store.addLog('error', 'Spotify Artist Top Tracks Error', error);
                    throw error;
                }
                store.addLog('error', 'Spotify Artist Top Tracks Error', error);
                throw new SpotifyNetworkError('Network error fetching artist top tracks', error);
            }
        });
    },

    /**
     * Get artist details.
     * Returns artist profile information including images, followers, genres.
     * 
     * @param artistId - Spotify artist ID
     * @returns Promise resolving to artist object
     */
    async getArtist(artistId: string): Promise<any> {
        return enqueue(async () => {
            const store = useStore.getState();

            try {
                const res = await backendSpotifyFetch(`${API_BASE}/artists/${artistId}`);

                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while fetching artist',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (!res.ok) {
                    throw new SpotifyApiError(
                        `Failed to fetch artist: ${res.statusText}`,
                        res.status
                    );
                }

                return await res.json();
            } catch (error) {
                if (error instanceof SpotifyAuthError || error instanceof SpotifyRateLimitError || error instanceof SpotifyApiError) {
                    store.addLog('error', 'Spotify Artist Error', error);
                    throw error;
                }
                store.addLog('error', 'Spotify Artist Error', error);
                throw new SpotifyNetworkError('Network error fetching artist', error);
            }
        });
    }
};

