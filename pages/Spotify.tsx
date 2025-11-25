import React, { useEffect } from 'react';
import { Wifi, LogOut, ExternalLink, CheckCircle } from 'lucide-react';
import { useStore } from '../store';
import { SpotifyService } from '../services/spotifyService';

export const Spotify: React.FC = () => {
  const { 
      spotifyClientId, spotifyClientSecret, spotifyUser, 
      logoutSpotify, setSpotifyTokens, setSpotifyUser, addLog 
  } = useStore();

  useEffect(() => {
      const handleMessage = (event: MessageEvent) => {
          if (event.origin !== window.location.origin) return;
          
          if (event.data?.type === 'SPOTIFY_AUTH_SUCCESS') {
              const { accessToken, refreshToken, expiry, user } = event.data;
              setSpotifyTokens(accessToken, refreshToken, expiry);
              setSpotifyUser(user);
              addLog('success', `Logged in as ${user.display_name}`);
          }
      };
      
      window.addEventListener('message', handleMessage);
      return () => window.removeEventListener('message', handleMessage);
  }, [setSpotifyTokens, setSpotifyUser, addLog]);

  const handleLogin = () => {
    if (!spotifyClientId || !spotifyClientSecret) {
        alert("Please configure your Client ID and Client Secret in Settings first.");
        return;
    }
    const redirectUri = `${window.location.origin}/callback`;
    const url = SpotifyService.generateAuthUrl(spotifyClientId, redirectUri);
    
    // Open popup
    const width = 600;
    const height = 800;
    const left = window.screen.width / 2 - width / 2;
    const top = window.screen.height / 2 - height / 2;
    window.open(url, 'Spotify Auth', `width=${width},height=${height},left=${left},top=${top}`);
  };

  return (
    <div className="p-8 pb-32 flex flex-col items-center justify-center min-h-[60vh]">
        <div className={`w-24 h-24 rounded-full flex items-center justify-center mb-6 shadow-xl transition-colors ${spotifyUser ? 'bg-[#1db954] shadow-green-900/20' : 'bg-[#333]'}`}>
            <Wifi size={40} className={spotifyUser ? 'text-black' : 'text-gray-500'} />
        </div>
        
        {spotifyUser ? (
             <div className="text-center">
                 <h1 className="text-3xl font-bold mb-2">Connected as {spotifyUser.display_name}</h1>
                 <p className="text-[#b3b8c1] mb-2">{spotifyUser.product === 'premium' ? 'Premium Plan' : 'Free Plan'}</p>
                 <p className="text-[#6f7480] text-sm mb-8">{spotifyUser.email}</p>
                 
                 <div className="flex gap-4 justify-center">
                     <a 
                        href="https://open.spotify.com" 
                        target="_blank" 
                        rel="noreferrer"
                        className="bg-white/10 hover:bg-white/20 text-white font-bold py-3 px-6 rounded-full flex items-center gap-2 transition-colors"
                     >
                        <ExternalLink size={18} /> Open Spotify Web
                     </a>
                     <button 
                        onClick={logoutSpotify}
                        className="bg-red-500/10 hover:bg-red-500/20 text-red-500 font-bold py-3 px-6 rounded-full flex items-center gap-2 transition-colors border border-red-500/20"
                     >
                        <LogOut size={18} /> Disconnect
                     </button>
                 </div>
                 
                 {spotifyUser.product !== 'premium' && (
                     <div className="mt-8 p-4 bg-yellow-500/10 border border-yellow-500/30 rounded-lg max-w-md mx-auto">
                         <p className="text-yellow-500 text-sm">
                             Note: Full playback features require a Spotify Premium account. Metadata and library syncing are still available.
                         </p>
                     </div>
                 )}
             </div>
        ) : (
            <div className="flex flex-col items-center">
                <h1 className="text-3xl font-bold mb-3">Connect to Spotify</h1>
                <p className="text-[#b3b8c1] text-center max-w-md mb-8">
                    Unlock Premium features to enhance your library with high-quality metadata and streaming integration.
                </p>

                <button 
                    onClick={handleLogin}
                    disabled={!spotifyClientId || !spotifyClientSecret}
                    className="bg-[#1db954] hover:bg-[#1ed760] disabled:opacity-50 disabled:cursor-not-allowed text-black font-bold py-3 px-8 rounded-full flex items-center gap-2 transition-transform hover:scale-105 mb-12 shadow-lg shadow-green-900/40"
                >
                    <Wifi size={20} /> Connect with Spotify
                </button>

                <div className="w-full max-w-lg bg-[#181818] border border-[#ffaa00]/30 rounded-xl p-6 relative overflow-hidden text-left">
                    <div className="absolute top-0 left-0 w-1 h-full bg-[#ffaa00]"></div>
                    <h4 className="text-[#ffaa00] font-bold text-sm uppercase tracking-wide mb-3 flex items-center gap-2">
                        Configuration Required
                    </h4>
                    <div className="text-sm text-[#b3b8c1] space-y-3">
                        <p>To enable integration:</p>
                        <ol className="list-decimal list-inside space-y-2 ml-1">
                            <li>Create a Spotify App at <a href="https://developer.spotify.com/dashboard" target="_blank" rel="noreferrer" className="text-green-500 hover:underline">developer.spotify.com</a></li>
                            <li>
                                Go to <span className="text-white font-bold">Settings</span> in this app and enter both your 
                                <span className="text-white font-mono bg-[#333] px-1 rounded mx-1">Client ID</span> 
                                and 
                                <span className="text-white font-mono bg-[#333] px-1 rounded mx-1">Client Secret</span>.
                            </li>
                            <li>Add this Redirect URI in your Spotify Dashboard:</li>
                        </ol>
                        <div className="mt-2 bg-[#121212] p-3 rounded font-mono text-xs text-gray-400 break-all select-all border border-[#333]">
                            {window.location.origin}/callback
                        </div>
                    </div>
                </div>
            </div>
        )}
    </div>
  );
};