import { createHash } from 'node:crypto';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { SpotifyService } from './spotifyService';

afterEach(() => vi.unstubAllGlobals());

describe('Spotify PKCE on macOS Wails', () => {
    it('uses the native hash when SubtleCrypto is unavailable', async () => {
        const nativeHash = vi.fn(async (verifier: string) =>
            createHash('sha256').update(verifier).digest('base64url'));
        vi.stubGlobal('window', {
            location: { hostname: 'wails', protocol: 'wails:' },
            crypto: {},
            go: { main: { App: { GenerateSpotifyCodeChallenge: nativeHash } } },
        });
        vi.stubGlobal('crypto', {
            getRandomValues: (bytes: Uint8Array) => bytes.map((_, index) => index),
        });

        const { url, codeVerifier, state } = await SpotifyService.generateAuthUrl(
            'test-client', 'http://127.0.0.1:34115/callback');
        const query = new URL(url).searchParams;
        expect(nativeHash).toHaveBeenCalledWith(codeVerifier);
        expect(query.get('code_challenge')).toBe(
            createHash('sha256').update(codeVerifier).digest('base64url'));
        expect(query.get('code_challenge_method')).toBe('S256');
        expect(query.get('redirect_uri')).toBe('http://127.0.0.1:34115/callback');
        expect(query.get('state')).toBe(state);
    });
});
