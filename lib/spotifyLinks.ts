/** Parses direct Spotify catalog links without following user-supplied URLs. */
export function parseSpotifyLink(input: string) {
    const value = input.trim();
    let target = value;
    if (!value.startsWith('spotify:')) {
        try {
            const url = new URL(value);
            if (url.origin !== 'https://open.spotify.com' || url.username || url.password) return null;
            target = 'spotify:' + url.pathname.replace(/^\/(?:intl-[a-z-]+\/)?/i, '').replace(/\/$/, '').replace('/', ':');
        } catch {
            return null;
        }
    }
    const match = /^spotify:(track|album|artist|playlist):([a-zA-Z0-9]{22})$/.exec(target);
    return match ? {type: match[1] as 'track' | 'album' | 'artist' | 'playlist', id: match[2]} : null;
}
