/**
 * ViiB MediaHub - Spotify OAuth Callback Page
 * 
 * Handles the OAuth 2.0 PKCE callback from Spotify authorization.
 * 
 * Flow:
 * 1. User redirected here after Spotify login
 * 2. Extracts authorization code from URL params
 * 3. Fetches client credentials from backend (supports cross-origin popups)
 * 4. Exchanges code for access/refresh tokens
 * 5. Fetches user profile
 * 6. Saves credentials to backend for download functionality
 * 7. Closes popup - main window detects auth via backend polling
 * 
 * Cross-Origin Handling (Wails):
 * - Main window: runs on http://wails.localhost (WebView2)
 * - OAuth popup: runs on http://127.0.0.1:PORT (API server)
 * - These are different origins, so localStorage is NOT shared
 * - We CANNOT access window.opener properties (throws security error)
 * - Communication happens via backend: popup saves tokens → main window polls
 * 
 * @module SpotifyCallback
 */

import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useStore } from '../store';
import { SpotifyService } from '../services/spotifyService';
import { Loader2, XCircle, CheckCircle } from 'lucide-react';
import { isWailsEnvironment } from '../utils';

import { api } from '../services/api';

/**
 * Detects if we're running in a cross-origin context where we cannot
 * directly communicate with window.opener.
 * 
 * Cross-origin scenarios:
 * 1. Wails build: popup on 127.0.0.1, main window on wails.localhost
 * 2. Any case where opener origin differs from current origin
 * 
 * IMPORTANT: We cannot access window.opener.origin directly when cross-origin
 * as the browser will throw a security error. We must use indirect detection.
 * 
 * @returns true if we're in a cross-origin popup context
 */
const detectCrossOriginContext = (): boolean => {
    // In Wails, the popup runs on the API server (127.0.0.1) while
    // the main window runs on wails.localhost - these are cross-origin
    const isPopupOnApiServer = window.location.hostname === '127.0.0.1';
    
    // If we're on the API server (not wails.localhost), we're definitely cross-origin
    if (isPopupOnApiServer && !isWailsEnvironment()) {
        return true;
    }
    
    // For non-Wails environments, try to detect via window.opener
    if (window.opener) {
        try {
            // Attempting to access window.opener.origin will throw if cross-origin
            // If it doesn't throw, compare origins
            const openerOrigin = window.opener.location.origin;
            return openerOrigin !== window.location.origin;
        } catch {
            // SecurityError thrown - we're definitely cross-origin
            return true;
        }
    }
    
    // No opener means we navigated here directly (not a popup)
    return false;
};

export const SpotifyCallback: React.FC = () => {
    const [searchParams] = useSearchParams();
    const navigate = useNavigate();
    const { spotifyClientId, spotifyClientSecret, setSpotifyTokens, setSpotifyUser, addLog } = useStore();
    const [status, setStatus] = useState<'processing' | 'error' | 'success'>('processing');
    const [errorMsg, setErrorMsg] = useState('');

    useEffect(() => {
        const code = searchParams.get('code');
        const error = searchParams.get('error');

        if (error) {
            setStatus('error');
            setErrorMsg(error);
            return;
        }

        if (!code) {
            setStatus('error');
            setErrorMsg('No authorization code returned');
            return;
        }

        const processAuth = async () => {
            try {
                // Get credentials - try Zustand first, fall back to backend API
                // This is needed because in cross-origin popup (Wails), Zustand is empty
                let clientId = spotifyClientId;
                let clientSecret = spotifyClientSecret;
                
                if (!clientId || !clientSecret) {
                    console.log('[SpotifyCallback] No credentials in Zustand, fetching from backend...');
                    try {
                        const creds = await api.getSpotifyCredentials();
                        if (creds && creds.clientId && creds.clientSecret) {
                            clientId = creds.clientId;
                            clientSecret = creds.clientSecret;
                            console.log('[SpotifyCallback] Got credentials from backend');
                        }
                    } catch (e) {
                        console.error('[SpotifyCallback] Failed to fetch credentials from backend:', e);
                    }
                }
                
                if (!clientId || !clientSecret) {
                    throw new Error("Missing Spotify credentials. Please configure them in Settings.");
                }

                // Get the redirect URI that was used to initiate the auth flow
                let redirectUri = localStorage.getItem('spotify_redirect_uri');
                let codeVerifier = localStorage.getItem('spotify_code_verifier');
                
                // If missing from localStorage (common in Wails cross-origin popup), try fetching from backend
                // In Wails, the popup runs on 127.0.0.1 while the app runs on wails.localhost, so localStorage is not shared
                if ((!codeVerifier || !redirectUri) && (isWailsEnvironment() || window.location.hostname === '127.0.0.1')) {
                    console.log('[SpotifyCallback] Missing auth data in localStorage, fetching from backend...');
                    try {
                        const creds = await api.getSpotifyCredentials();
                        console.log('[SpotifyCallback] Backend credentials response:', JSON.stringify(creds));
                        
                        if (creds && creds.codeVerifier) {
                            codeVerifier = creds.codeVerifier;
                            console.log('[SpotifyCallback] Got code verifier from backend');
                        }
                        
                        // If redirectUri is still missing, we can try to reconstruct it or use the default
                        if (!redirectUri) {
                            redirectUri = `${window.location.origin}/callback`;
                            console.log('[SpotifyCallback] Using default redirect URI:', redirectUri);
                        }
                    } catch (e) {
                        console.error('[SpotifyCallback] Failed to fetch auth data from backend:', e);
                    }
                }

                if (!redirectUri) {
                    redirectUri = `${window.location.origin}/callback`;
                }

                if (!codeVerifier) {
                    throw new Error("Missing code verifier. Please try logging in again.");
                }

                // Exchange Code
                const data = await SpotifyService.exchangeCode(clientId, clientSecret, code, redirectUri, codeVerifier);
                
                // Clear stored auth data
                localStorage.removeItem('spotify_code_verifier');
                localStorage.removeItem('spotify_redirect_uri');

                const expiry = Date.now() + (data.expires_in * 1000);
                
                // Detect cross-origin context using our safe helper function
                // This avoids the SecurityError from accessing window.opener.origin
                const isCrossOrigin = detectCrossOriginContext();
                console.log('[SpotifyCallback] Cross-origin context:', isCrossOrigin);
                
                // Update Zustand if we're in the same context (not cross-origin)
                if (!isCrossOrigin) {
                    setSpotifyTokens(
                        data.access_token, 
                        data.refresh_token, 
                        expiry
                    );
                }
                
                // Always sync to backend - this is the primary communication mechanism for cross-origin
                try {
                    await api.saveSpotifyCredentials({
                        clientId: clientId,
                        clientSecret: clientSecret,
                        accessToken: data.access_token,
                        refreshToken: data.refresh_token,
                        expiry: expiry
                    });
                    console.log('[SpotifyCallback] Credentials synced to backend');
                } catch (e) {
                    console.error("Failed to sync credentials to backend", e);
                    throw new Error("Failed to save credentials to server");
                }

                // Fetch User Profile to verify tokens work
                // Use fresh tokens directly since SpotifyService may not have them yet
                const profileResponse = await fetch('https://api.spotify.com/v1/me', {
                    headers: { 'Authorization': `Bearer ${data.access_token}` }
                });
                
                if (!profileResponse.ok) {
                    throw new Error("Failed to fetch user profile");
                }
                
                const profile = await profileResponse.json();
                console.log('[SpotifyCallback] Profile fetched:', profile.display_name);
                
                // Update Zustand if same context
                if (!isCrossOrigin) {
                    setSpotifyUser(profile);
                    addLog('success', `Logged in as ${profile.display_name}`);
                }
                
                setStatus('success');
                
                // Handle popup/redirect based on context
                // In Wails cross-origin scenario, the main window polls the backend for tokens
                // so we just need to close the popup - no postMessage needed
                if (window.opener) {
                    // Try to send message to opener - this only works if same-origin
                    // In Wails (cross-origin), this will fail silently and that's OK
                    // because the main window polls the backend for auth completion
                    if (!isCrossOrigin) {
                        try {
                            window.opener.postMessage({
                                type: 'SPOTIFY_AUTH_SUCCESS',
                                accessToken: data.access_token,
                                refreshToken: data.refresh_token,
                                expiry: expiry,
                                user: profile
                            }, window.location.origin);
                            console.log('[SpotifyCallback] Posted auth success to opener');
                        } catch (e) {
                            console.log('[SpotifyCallback] Could not post message to opener:', e);
                        }
                    } else {
                        console.log('[SpotifyCallback] Cross-origin: skipping postMessage, main window will poll backend');
                    }
                    
                    // Close popup after a brief moment
                    setTimeout(() => window.close(), 1500);
                } else {
                    setTimeout(() => navigate('/spotify'), 1500);
                }
            } catch (err: any) {
                console.error(err);
                setStatus('error');
                setErrorMsg(err.message || 'Authentication failed');
                addLog('error', 'Spotify Auth Failed', err);
            }
        };

        processAuth();
    }, [searchParams, spotifyClientId, spotifyClientSecret, setSpotifyTokens, setSpotifyUser, addLog, navigate]);

    return (
        <div className="flex flex-col items-center justify-center min-h-screen bg-black text-white p-8">
            <div className="bg-surface-2 p-8 rounded-xl border border-surface-3 max-w-md w-full text-center shadow-2xl">
                {status === 'processing' && (
                    <>
                        <Loader2 size={48} className="animate-spin text-brand mx-auto mb-4" />
                        <h2 className="text-xl font-bold mb-2">Connecting to Spotify...</h2>
                        <p className="text-text-secondary">Exchanging keys and fetching profile.</p>
                    </>
                )}
                
                {status === 'success' && (
                    <>
                        <CheckCircle size={48} className="text-brand mx-auto mb-4" />
                        <h2 className="text-xl font-bold mb-2">Success!</h2>
                        <p className="text-text-secondary">
                            {window.opener ? 'This window will close automatically.' : 'Redirecting you back to the app...'}
                        </p>
                    </>
                )}

                {status === 'error' && (
                    <>
                        <XCircle size={48} className="text-red-500 mx-auto mb-4" />
                        <h2 className="text-xl font-bold mb-2">Connection Failed</h2>
                        <p className="text-red-400 mb-6">{errorMsg}</p>
                        <button 
                            onClick={() => window.close()}
                            className="bg-surface-3 hover:bg-surface-hover text-white font-bold py-2 px-6 rounded-full transition-all duration-200"
                        >
                            Close
                        </button>
                    </>
                )}
            </div>
        </div>
    );
};