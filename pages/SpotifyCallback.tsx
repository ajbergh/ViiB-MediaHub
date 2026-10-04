/** Keeps the legacy callback route reachable and directs users to the Spotify session connection page. */

import React from 'react';
import { Link } from 'react-router';

export const SpotifyCallback: React.FC = () => <div className="p-8">
    <h1 className="text-section mb-4">Connect your Spotify session</h1>
    <p className="text-text-secondary mb-4">Sign in from the Spotify page using your Web Player session.</p>
    <Link className="text-brand underline" to="/spotify">Open Spotify connection</Link>
</div>;
