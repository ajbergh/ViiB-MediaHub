/** Tests and fixtures for spotify OAuth Context behavior. */

import { describe, expect, it } from 'vitest';
import { selectSpotifyOAuthContext } from './spotifyOAuthContext';

const current = {
    clientId: 'current-client',
    state: 'current-state',
    redirectUri: 'http://127.0.0.1:34115/callback',
    codeVerifier: 'current-verifier',
};

describe('Spotify callback context', () => {
    it('uses the current backend attempt on the desktop callback even when the browser has stale values', () => {
        const stale = {
            clientId: 'old-client',
            state: 'old-state',
            redirectUri: 'http://127.0.0.1:34115/callback',
            codeVerifier: 'old-verifier',
        };
        expect(selectSpotifyOAuthContext(stale, current, true)).toEqual(current);
    });

    it('keeps browser-origin values for an ordinary web callback', () => {
        expect(selectSpotifyOAuthContext(current, {
            clientId: 'other-client', state: 'other-state', redirectUri: null, codeVerifier: null,
        }, false)).toEqual(current);
    });
});
