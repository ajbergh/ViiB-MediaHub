import { ArtistMetadata, AlbumMetadata, SpotifyProfile } from '../types';
import { useStore } from '../store';

const AUTH_URL = 'https://accounts.spotify.com/authorize';
const TOKEN_URL = 'https://accounts.spotify.com/api/token';
const API_BASE = 'https://api.spotify.com/v1';

// Simple queue to prevent flooding the API
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

// --- Helpers ---

const generateRandomString = (length: number) => {
    const possible = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    const values = crypto.getRandomValues(new Uint8Array(length));
    return values.reduce((acc, x) => acc + possible[x % possible.length], "");
}

const sha256 = async (plain: string) => {
    const encoder = new TextEncoder();
    const data = encoder.encode(plain);
    return window.crypto.subtle.digest('SHA-256', data);
}

const base64encode = (input: ArrayBuffer) => {
    return btoa(String.fromCharCode(...new Uint8Array(input)))
        .replace(/=/g, '')
        .replace(/\+/g, '-')
        .replace(/\//g, '_');
}

const cleanName = (str: string): string => {
    return str
        .replace(/[\(\[].*?[\)\]]/g, '') 
        .replace(/\b(remaster|remastered|deluxe|edition|version|feat|ft\.|vol\.|volume)\b.*$/i, '')
        .replace(/[^\w\s]/gi, '')
        .replace(/\s+/g, ' ')
        .trim()
        .toLowerCase();
};

const getSimilarity = (s1: string, s2: string): number => {
    const n1 = cleanName(s1);
    const n2 = cleanName(s2);
    if (n1 === n2) return 1.0;
    if (!n1 || !n2) return 0.0;
    if (n1.includes(n2) || n2.includes(n1)) return 0.9;
    return 0.0; // Simplified for now
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

        const response = await fetch(TOKEN_URL, {
            method: 'POST',
            headers,
            body: new URLSearchParams(bodyParams)
        });

        if (!response.ok) {
            const err = await response.json();
            throw new Error(err.error_description || 'Failed to exchange code');
        }

        return response.json(); // { access_token, refresh_token, expires_in, ... }
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
            
            // Refresh User Token
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
                
                if (response.ok) {
                    const data = await response.json();
                    setSpotifyTokens(data.access_token, data.refresh_token || spotifyRefreshToken, Date.now() + (data.expires_in * 1000));
                    return data.access_token;
                } else {
                    store.addLog('warn', 'Failed to refresh user token. Falling back.');
                }
            } catch (e) {
                store.addLog('error', 'Error refreshing user token', e);
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

            if (response.ok) {
                const data = await response.json();
                return data.access_token;
            }
        } catch (e) {
            store.addLog('error', 'Client Credentials Auth Failed', e);
        }
        
        return null;
    },

    async getUserProfile(): Promise<SpotifyProfile | null> {
        return enqueue(async () => {
            const token = await this.getAccessToken();
            if (!token) return null;
            
            try {
                const res = await fetch(`${API_BASE}/me`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });
                
                if (res.status === 401 || res.status === 403) {
                    return null;
                }
                
                if (res.ok) {
                    return await res.json();
                }
                return null;
            } catch (e) {
                return null;
            }
        });
    },

    async searchArtist(artistName: string): Promise<ArtistMetadata | null> {
        return enqueue(async () => {
            const store = useStore.getState();
            const token = await this.getAccessToken();
            if (!token) return null;

            try {
                const query = encodeURIComponent(artistName);
                const res = await fetch(`${API_BASE}/search?q=${query}&type=artist&limit=3`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

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

            } catch (e) {
                store.addLog('error', `Spotify Artist Search Error: ${artistName}`, e);
                return null;
            }
        });
    },

    async searchAlbum(albumName: string, artistName: string): Promise<AlbumMetadata | null> {
        return enqueue(async () => {
            const store = useStore.getState();
            const token = await this.getAccessToken();
            if (!token) return null;

            try {
                const query = `album:${cleanName(albumName)} artist:${cleanName(artistName)}`;
                const res = await fetch(`${API_BASE}/search?q=${encodeURIComponent(query)}&type=album&limit=5`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

                const data = await res.json();
                if (!data.albums || data.albums.items.length === 0) {
                    return null;
                }

                const album = data.albums.items[0]; 
                
                const fullAlbumRes = await fetch(`${API_BASE}/albums/${album.id}`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });
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

            } catch (e) {
                store.addLog('error', `Spotify Album Search Error: ${albumName}`, e);
                return null;
            }
        });
    },

    async search(query: string, types: string[] = ['album', 'playlist', 'track', 'artist']): Promise<any> {
        return enqueue(async () => {
            const store = useStore.getState();
            const token = await this.getAccessToken();
            if (!token) return null;

            try {
                const typeStr = types.join(',');
                const res = await fetch(`${API_BASE}/search?q=${encodeURIComponent(query)}&type=${typeStr}&limit=20`, {
                    headers: { 'Authorization': `Bearer ${token}` }
                });

                if (!res.ok) {
                    throw new Error(`Spotify API error: ${res.statusText}`);
                }

                return await res.json();
            } catch (e) {
                store.addLog('error', `Spotify Search Error: ${query}`, e);
                return null;
            }
        });
    }
};
