/**
 * ViiB MediaHub - Spotify OAuth Callback Page
 * 
 * Handles the OAuth 2.0 PKCE callback from Spotify authorization.
 * 
 * Flow:
 * 1. User redirected here after Spotify login
 * 2. Extracts authorization code from URL params
 * 3. Exchanges code for access/refresh tokens
 * 4. Fetches user profile
 * 5. Saves credentials to backend for download functionality
 * 6. Posts success message to parent window (if popup)
 * 7. Redirects to Spotify page or closes popup
 * 
 * Handles errors gracefully with visual feedback.
 * 
 * @module SpotifyCallback
 */

import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useStore } from '../store';
import { SpotifyService } from '../services/spotifyService';
import { Loader2, XCircle, CheckCircle } from 'lucide-react';

import { api } from '../services/api';

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
                const redirectUri = `${window.location.origin}/callback`;
                const codeVerifier = localStorage.getItem('spotify_code_verifier');
                
                if (!codeVerifier) {
                    throw new Error("Missing code verifier. Please try logging in again.");
                }

                // Exchange Code
                const data = await SpotifyService.exchangeCode(spotifyClientId, spotifyClientSecret, code, redirectUri, codeVerifier);
                
                // Clear verifier
                localStorage.removeItem('spotify_code_verifier');

                const expiry = Date.now() + (data.expires_in * 1000);
                
                setSpotifyTokens(
                    data.access_token, 
                    data.refresh_token, 
                    expiry
                );
                
                // Sync to Backend
                try {
                    await api.saveSpotifyCredentials({
                        clientId: spotifyClientId,
                        clientSecret: spotifyClientSecret,
                        accessToken: data.access_token,
                        refreshToken: data.refresh_token,
                        expiry: expiry
                    });
                    addLog('success', 'Spotify credentials synced to backend');
                } catch (e) {
                    console.error("Failed to sync credentials to backend", e);
                    addLog('warn', 'Failed to sync Spotify credentials to backend');
                }

                // Fetch User Profile immediately to verify and store
                const profile = await SpotifyService.getUserProfile();
                if (profile) {
                    setSpotifyUser(profile);
                    addLog('success', `Logged in as ${profile.display_name}`);
                    setStatus('success');
                    
                    if (window.opener) {
                         // Send data back to main window
                         window.opener.postMessage({
                             type: 'SPOTIFY_AUTH_SUCCESS',
                             accessToken: data.access_token,
                             refreshToken: data.refresh_token,
                             expiry: expiry,
                             user: profile
                         }, window.location.origin);
                         
                         // Close popup after a brief moment
                         setTimeout(() => window.close(), 1000);
                    } else {
                        setTimeout(() => navigate('/spotify'), 1500);
                    }
                } else {
                    throw new Error("Failed to fetch user profile");
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
            <div className="bg-[#181818] p-8 rounded-xl border border-[#333] max-w-md w-full text-center shadow-2xl">
                {status === 'processing' && (
                    <>
                        <Loader2 size={48} className="animate-spin text-green-500 mx-auto mb-4" />
                        <h2 className="text-xl font-bold mb-2">Connecting to Spotify...</h2>
                        <p className="text-[#b3b8c1]">Exchanging keys and fetching profile.</p>
                    </>
                )}
                
                {status === 'success' && (
                    <>
                        <CheckCircle size={48} className="text-green-500 mx-auto mb-4" />
                        <h2 className="text-xl font-bold mb-2">Success!</h2>
                        <p className="text-[#b3b8c1]">
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
                            className="bg-[#333] hover:bg-[#444] text-white font-bold py-2 px-6 rounded-full transition-colors"
                        >
                            Close
                        </button>
                    </>
                )}
            </div>
        </div>
    );
};