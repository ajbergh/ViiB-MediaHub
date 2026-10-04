/** Links Spotify artist names to their catalog pages without triggering the surrounding track action. */
import React from 'react';
import { Link } from 'react-router';

export const SpotifyArtistLinks: React.FC<{
    artists: Array<{ id?: string; name: string }>;
}> = ({ artists }) => (
    <>
        {artists.map((artist, index) => (
            <React.Fragment key={artist.id || index}>
                {index > 0 && ', '}
                {artist.id ? (
                    <Link
                        to={'/spotify/artist/' + encodeURIComponent(artist.id)}
                        onClick={(event) => event.stopPropagation()}
                        className="hover:underline"
                    >
                        {artist.name}
                    </Link>
                ) : (
                    artist.name
                )}
            </React.Fragment>
        ))}
    </>
);
