export interface SpotifyOAuthContext {
    clientId: string;
    state: string | null;
    redirectUri: string | null;
    codeVerifier: string | null;
}

/** The desktop browser has separate, potentially stale localStorage. */
export function selectSpotifyOAuthContext(
    local: SpotifyOAuthContext,
    backend: SpotifyOAuthContext,
    isDesktopCallback: boolean,
): SpotifyOAuthContext {
    if (isDesktopCallback) return backend;
    return {
        clientId: local.clientId || backend.clientId,
        state: local.state || backend.state,
        redirectUri: local.redirectUri || backend.redirectUri,
        codeVerifier: local.codeVerifier || backend.codeVerifier,
    };
}
