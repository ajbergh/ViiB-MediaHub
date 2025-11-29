
import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useStore, useAlbums, useArtists } from '../store';
import { Sparkles, Search, Play } from 'lucide-react';
import { generateGradient, coverBackground } from '../utils';
import { ContextMenuType } from '../types';

export const Home: React.FC = () => {
  const { songs, smartMixes, refreshSmartMixes, playSong, openContextMenu, showSmartMixes } = useStore();
  const albums = useAlbums();
  const artists = useArtists();
  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState('');

  // Initial refresh of smart mixes just in case
  useEffect(() => {
      if (songs.length > 0) {
          refreshSmartMixes();
      }
  }, [songs.length]);

  const handleSearch = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && searchQuery.trim()) {
        navigate('/search', { state: { query: searchQuery } });
    }
  };

  const playMix = (mixId: string) => {
      const mix = smartMixes.find(m => m.id === mixId);
      if (mix && mix.songIds.length > 0) {
          const mixSongs = songs.filter(s => mix.songIds.includes(s.id));
          if (mixSongs.length > 0) {
              playSong(mixSongs[0], mixSongs);
          }
      }
  };

  return (
    <div className="p-8 pb-32">
      {/* Header Section */}
      <section className="mb-12 flex flex-col items-center justify-center pt-8">
        <h1 className="text-4xl md:text-5xl font-bold mb-8 text-center bg-clip-text text-transparent bg-gradient-to-r from-white to-gray-400">Let's ViiB</h1>
        
        {/* Global Search Input */}
        <div className="relative w-full max-w-2xl flex justify-center">
             <div className="relative w-full">
                <Search className="absolute left-5 top-1/2 -translate-y-1/2 text-text-secondary" size={22} />
                <input 
                    type="text" 
                    placeholder="What do you want to play?" 
                    className="w-full bg-surface-highlight hover:bg-surface-hover focus:bg-surface-hover border border-transparent focus:border-surface-slider rounded-full py-4 pl-14 pr-6 text-white outline-none transition-all placeholder-text-subtle text-lg shadow-lg"
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    onKeyDown={handleSearch}
                />
            </div>
        </div>
      </section>

      {/* Smart Mixes Section */}
      {songs.length > 0 && showSmartMixes && (
          <section className="mb-12">
            <div className="flex items-center gap-2 mb-6">
                <Sparkles className="text-purple-400" size={24} />
                <h2 className="text-2xl font-bold">Smart Mixes</h2>
            </div>
            
            <div className="flex overflow-x-auto gap-6 pb-4 scrollbar-hide snap-x">
                {smartMixes.map((mix) => (
                    <div 
                        key={mix.id}
                        className="flex-shrink-0 w-80 bg-surface-2 rounded-xl overflow-hidden group cursor-pointer border border-transparent hover:border-surface-border transition-all relative snap-start"
                        onClick={() => navigate(`/smart-mix/${mix.id}`)}
                        onContextMenu={(e) => openContextMenu(e, ContextMenuType.SMART_MIX, mix)}
                    >
                        <div 
                            className="h-40 p-6 flex flex-col justify-end relative"
                            style={{ background: `linear-gradient(135deg, ${mix.coverColors[0]}, ${mix.coverColors[1]})` }}
                        >
                            <h3 className="text-2xl font-bold text-white shadow-black drop-shadow-md">{mix.name}</h3>
                            <p className="text-white/80 text-sm font-medium drop-shadow">{mix.songIds.length} tracks</p>
                            
                            <button 
                                onClick={(e) => { e.stopPropagation(); playMix(mix.id); }}
                                className="absolute bottom-4 right-4 w-12 h-12 bg-brand text-black rounded-full flex items-center justify-center opacity-0 group-hover:opacity-100 transform translate-y-4 group-hover:translate-y-0 transition-all shadow-xl hover:scale-105 hover:bg-brand-hover"
                            >
                                <Play size={24} className="fill-current ml-1" />
                            </button>
                        </div>
                        <div className="p-4">
                            <p className="text-text-secondary text-sm line-clamp-2">{mix.description}</p>
                        </div>
                    </div>
                ))}
            </div>
          </section>
      )}

      {/* Stats Cards */}
      <section className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-12">
        <div 
            className="bg-surface-2 p-6 rounded-xl border border-surface-3 hover:bg-surface-hover transition-colors group relative overflow-hidden cursor-pointer"
            onClick={() => navigate('/songs')}
        >
            <div className="relative z-10">
                <MusicIcon className="text-green-500 mb-4" />
                <h3 className="text-3xl font-bold mb-1">{songs.length}</h3>
                <p className="text-text-secondary text-sm font-medium">Total Songs</p>
            </div>
            <div className="absolute top-0 right-0 w-24 h-24 bg-green-500/10 rounded-full -mr-4 -mt-4 blur-2xl transition-all group-hover:bg-green-500/20"></div>
        </div>

        <div 
            className="bg-surface-2 p-6 rounded-xl border border-surface-3 hover:bg-surface-hover transition-colors group relative overflow-hidden cursor-pointer"
            onClick={() => navigate('/albums')}
        >
            <div className="relative z-10">
                <AlbumIcon className="text-purple-500 mb-4" />
                <h3 className="text-3xl font-bold mb-1">{albums.length}</h3>
                <p className="text-text-secondary text-sm font-medium">Albums</p>
            </div>
            <div className="absolute top-0 right-0 w-24 h-24 bg-purple-500/10 rounded-full -mr-4 -mt-4 blur-2xl transition-all group-hover:bg-purple-500/20"></div>
        </div>

        <div 
            className="bg-surface-2 p-6 rounded-xl border border-surface-3 hover:bg-surface-hover transition-colors group relative overflow-hidden cursor-pointer"
            onClick={() => navigate('/artists')}
        >
             <div className="relative z-10">
                <ArtistIcon className="text-blue-500 mb-4" />
                <h3 className="text-3xl font-bold mb-1">{artists.length}</h3>
                <p className="text-text-secondary text-sm font-medium">Artists</p>
            </div>
            <div className="absolute top-0 right-0 w-24 h-24 bg-blue-500/10 rounded-full -mr-4 -mt-4 blur-2xl transition-all group-hover:bg-blue-500/20"></div>
        </div>
      </section>

      {/* Recently Added Albums */}
      <section>
        <h2 className="text-2xl font-bold mb-6">Recently Added Albums</h2>
        {albums.length === 0 ? (
           <div className="bg-surface-2 rounded-xl p-8 text-center border border-dashed border-surface-border">
                <p className="text-text-secondary">No albums found. Import some music in Settings to get started!</p>
           </div>
        ) : (
            <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 gap-6">
            {albums.slice(0, 5).map((album, idx) => (
                <div 
                    key={idx} 
                    className="bg-surface-2 p-4 rounded-lg hover:bg-surface-3 transition-all group cursor-pointer"
                    onClick={() => navigate(`/album/${encodeURIComponent(album.name)}`)}
                >
                <div 
                    className="w-full aspect-square rounded-md mb-4 shadow-lg flex items-center justify-center text-4xl font-bold text-white/20 relative overflow-hidden bg-surface-3"
                    style={{ background: coverBackground(album.coverUrl, album.name) }}
                >
                     {!album.coverUrl && album.name.charAt(0)}
                </div>
                <h4 className="font-bold truncate text-text-main mb-1 group-hover:text-brand transition-colors">{album.name}</h4>
                <p className="text-sm text-text-secondary truncate">{album.artist}</p>
                </div>
            ))}
            </div>
        )}
      </section>
    </div>
  );
};

// Icons Helpers
const MusicIcon = ({ className }: {className?: string}) => (
    <svg className={`w-8 h-8 ${className}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3" />
    </svg>
)
const AlbumIcon = ({ className }: {className?: string}) => (
    <svg className={`w-8 h-8 ${className}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
    </svg>
)
const ArtistIcon = ({ className }: {className?: string}) => (
    <svg className={`w-8 h-8 ${className}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
    </svg>
)
