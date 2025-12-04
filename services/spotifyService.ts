/**
 * Spotify Web API Integration Service
 * 
 * Provides OAuth 2.0 PKCE authentication and API access for Spotify.
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
 * - Automatic token refresh with mutex to prevent race conditions
 * - Request queuing to respect rate limits (200ms between requests)
 * - Typed error handling (SpotifyAuthError, SpotifyRateLimitError, etc.)
 * - Fuzzy string matching for artist/album metadata
 * - Levenshtein distance algorithm for improved matching accuracy
 * 
 * The access tokens are also used by the backend for librespot downloads,
 * providing seamless integration between Web API and download functionality.
 */

import { ArtistMetadata, AlbumMetadata, SpotifyProfile } from '../types';
import { useStore } from '../store';
import { SpotifyAuthError, SpotifyRateLimitError, SpotifyApiError, SpotifyNetworkError } from '../lib/spotifyErrors';

const AUTH_URL = 'https://accounts.spotify.com/authorize';
const TOKEN_URL = 'https://accounts.spotify.com/api/token';
const API_BASE = 'https://api.spotify.com/v1';

// Request queue to prevent flooding the API and respect rate limits
// Spotify allows ~180 requests per minute; we use 200ms delay for safety
let requestQueue: (() => Promise<void>)[] = [];
let isProcessingQueue = false;

// Token refresh mutex to prevent race conditions during concurrent requests
// Multiple simultaneous requests should wait for a single token refresh
let isRefreshing = false;
let refreshPromise: Promise<string | null> | null = null;

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
    return new Promise((resolve, reject) => {
        requestQueue.push(async () => {
            try {
                const result = await task();
                resolve(result);
            } catch (error) {
                reject(error);
            }
        });
        processQueue();
    });
};

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
 * @returns Promise resolving to ArrayBuffer containing hash
 */
const sha256 = async (plain: string) => {
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
        const codeChallenge = base64encode(hashed);

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
            code_challenge: codeChallenge
        });

        const url = `${AUTH_URL}?${params.toString()}`;
        
        // Log for debugging
        console.log('[SpotifyService] Generated Auth URL:', url);
        console.log('[SpotifyService] Redirect URI:', redirectUri);
        console.log('[SpotifyService] Client ID:', clientId);
        
        return {
            url,
            codeVerifier
        };
    },

    async exchangeCode(clientId: string, clientSecret: string, code: string, redirectUri: string, codeVerifier: string) {
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

        if (clientSecret) {
             const credentials = btoa(`${clientId}:${clientSecret}`);
             headers['Authorization'] = `Basic ${credentials}`;
        }

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

    async getAccessToken(): Promise<string | null> {
        const store = useStore.getState();
        const { 
            spotifyClientId, spotifyClientSecret, 
            spotifyAccessToken, spotifyRefreshToken, spotifyTokenExpiry,
            setSpotifyTokens
        } = store;

        if (!spotifyClientId || !spotifyClientSecret) {
            return null;
        }

        // 1. Check User Token
        if (spotifyAccessToken && spotifyRefreshToken) {
            if (Date.now() < spotifyTokenExpiry) {
                return spotifyAccessToken;
            }
            
            // Mutex: If already refreshing, wait for that promise
            if (isRefreshing && refreshPromise) {
                return refreshPromise;
            }
            
            // Start refresh with mutex
            isRefreshing = true;
            refreshPromise = (async () => {
                try {
                    store.addLog('info', 'Refreshing Spotify User Token...');
                    const credentials = btoa(`${spotifyClientId}:${spotifyClientSecret}`);
                    const response = await fetch(TOKEN_URL, {
                        method: 'POST',
                        headers: {
                            'Authorization': `Basic ${credentials}`,
                            'Content-Type': 'application/x-www-form-urlencoded'
                        },
                        body: new URLSearchParams({
                            grant_type: 'refresh_token',
                            refresh_token: spotifyRefreshToken
                        })
                    });
                    
                    if (response.status === 429) {
                        const retryAfter = response.headers.get('Retry-After');
                        throw new SpotifyRateLimitError(
                            'Rate limited while refreshing token',
                            retryAfter ? parseInt(retryAfter) : 60
                        );
                    }
                    
                    if (response.ok) {
                        const data = await response.json();
                        setSpotifyTokens(
                            data.access_token,
                            data.refresh_token || spotifyRefreshToken,
                            Date.now() + (data.expires_in * 1000)
                        );
                        return data.access_token;
                    } else {
                        const errorData = await response.json().catch(() => ({}));
                        throw new SpotifyAuthError(
                            errorData.error_description || 'Failed to refresh token',
                            response.status
                        );
                    }
                } catch (error) {
                    store.addLog('error', 'Error refreshing user token', error);
                    if (error instanceof SpotifyRateLimitError || error instanceof SpotifyAuthError) {
                        throw error;
                    }
                    throw new SpotifyNetworkError('Network error during token refresh', error);
                } finally {
                    isRefreshing = false;
                    refreshPromise = null;
                }
            })();
            
            try {
                return await refreshPromise;
            } catch (error) {
                store.addLog('warn', 'Token refresh failed. Falling back to client credentials.');
                // Continue to client credentials fallback
            }
        }

        // 2. Fallback: Client Credentials Flow
        try {
            const credentials = btoa(`${spotifyClientId}:${spotifyClientSecret}`);
            const response = await fetch(TOKEN_URL, {
                method: 'POST',
                headers: {
                    'Authorization': `Basic ${credentials}`,
                    'Content-Type': 'application/x-www-form-urlencoded'
                },
                body: 'grant_type=client_credentials'
            });

            if (response.status === 429) {
                const retryAfter = response.headers.get('Retry-After');
                throw new SpotifyRateLimitError(
                    'Rate limited on client credentials',
                    retryAfter ? parseInt(retryAfter) : 60
                );
            }

            if (response.ok) {
                const data = await response.json();
                return data.access_token;
            } else {
                throw new SpotifyAuthError('Client credentials authentication failed', response.status);
            }
        } catch (error) {
            if (error instanceof SpotifyRateLimitError || error instanceof SpotifyAuthError) {
                store.addLog('error', 'Spotify Auth Failed', error);
            } else {
                store.addLog('error', 'Client Credentials Auth Failed', error);
            }
        }
        
        return null;
    },

    async getUserProfile(): Promise<SpotifyProfile | null> {
        return enqueue(async () => {
            const token = await this.getAccessToken();
            if (!token) {
                throw new SpotifyAuthError('No access token available');
            }
            
            try {
                const res = await fetch(`${API_BASE}/me`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });
                
                if (res.status === 401 || res.status === 403) {
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
            const token = await this.getAccessToken();
            if (!token) {
                throw new SpotifyAuthError('No access token available');
            }

            try {
                const query = encodeURIComponent(artistName);
                const res = await fetch(`${API_BASE}/search?q=${query}&type=artist&limit=3`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

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

                const match = data.artists.items.find((a: any) => getSimilarity(a.name, artistName) > 0.8);
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
            const token = await this.getAccessToken();
            if (!token) {
                throw new SpotifyAuthError('No access token available');
            }

            try {
                const query = `album:${cleanName(albumName)} artist:${cleanName(artistName)}`;
                const res = await fetch(`${API_BASE}/search?q=${encodeURIComponent(query)}&type=album&limit=5`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

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

                const album = data.albums.items[0]; 
                
                const fullAlbumRes = await fetch(`${API_BASE}/albums/${album.id}`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

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
        offset: number = 0
    ): Promise<any> {
        return enqueue(async () => {
            const store = useStore.getState();
            const token = await this.getAccessToken();
            if (!token) {
                throw new SpotifyAuthError('No access token available');
            }

            try {
                const typeStr = types.join(',');
                const params = new URLSearchParams({
                    q: query,
                    type: typeStr,
                    limit: limit.toString(),
                    offset: offset.toString()
                });
                
                const res = await fetch(`${API_BASE}/search?${params.toString()}`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

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

                return await res.json();
            } catch (error) {
                if (error instanceof SpotifyAuthError || error instanceof SpotifyRateLimitError || error instanceof SpotifyApiError) {
                    store.addLog('error', `Spotify Search Error: ${query}`, error);
                    throw error;
                }
                store.addLog('error', `Spotify Search Error: ${query}`, error);
                throw new SpotifyNetworkError(`Network error during search: ${query}`, error);
            }
        });
    },

    async getRecentlyPlayed(limit: number = 20): Promise<any> {
        return enqueue(async () => {
            const store = useStore.getState();
            const token = await this.getAccessToken();
            if (!token) {
                throw new SpotifyAuthError('No access token available');
            }

            try {
                const res = await fetch(`${API_BASE}/me/player/recently-played?limit=${limit}`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while fetching recently played',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (res.status === 401 || res.status === 403) {
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
            const token = await this.getAccessToken();
            if (!token) {
                throw new SpotifyAuthError('No access token available');
            }

            try {
                const params = new URLSearchParams({
                    limit: limit.toString(),
                    offset: offset.toString()
                });

                const res = await fetch(`${API_BASE}/me/albums?${params.toString()}`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while fetching saved albums',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (res.status === 401 || res.status === 403) {
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
            const token = await this.getAccessToken();
            if (!token) {
                throw new SpotifyAuthError('No access token available');
            }

            try {
                const params = new URLSearchParams({
                    limit: limit.toString(),
                    offset: offset.toString()
                });

                const res = await fetch(`${API_BASE}/me/playlists?${params.toString()}`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while fetching saved playlists',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (res.status === 401 || res.status === 403) {
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
            const token = await this.getAccessToken();
            if (!token) {
                throw new SpotifyAuthError('No access token available');
            }

            try {
                const res = await fetch(`${API_BASE}/artists/${artistId}/top-tracks?market=${market}`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

                if (res.status === 429) {
                    const retryAfter = res.headers.get('Retry-After');
                    throw new SpotifyRateLimitError(
                        'Rate limited while fetching artist top tracks',
                        retryAfter ? parseInt(retryAfter) : 60
                    );
                }

                if (res.status === 401 || res.status === 403) {
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
            const token = await this.getAccessToken();
            if (!token) {
                throw new SpotifyAuthError('No access token available');
            }

            try {
                const res = await fetch(`${API_BASE}/artists/${artistId}`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

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

