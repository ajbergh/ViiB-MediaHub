import React, { useMemo } from 'react';
import { useParams, useNavigate } from 'react-router';
import { useStore } from '../store';
import { ChevronLeft, Play, Shuffle, Clock, Calendar, Music } from 'lucide-react';
import { formatTime } from '../utils';
import { ContextMenuType } from '../types';

export const GenreDetail: React.FC = () => {
  const { genreId } = useParams<{ genreId: string }>();
  const navigate = useNavigate();
  const { songs, playSong, currentSong, isPlaying, openContextMenu } = useStore();
  
  const genreName = decodeURIComponent(genreId || '');

  const genreSongs = useMemo(() => {
    if (!genreName) return [];
    return songs.filter(s => s.genre?.includes(genreName));
  }, [songs, genreName]);

  const totalDuration = useMemo(() => {
    return genreSongs.reduce((acc, song) => acc + song.duration, 0);
  }, [genreSongs]);

  const handlePlay = () => {
    if (genreSongs.length > 0) {
      playSong(genreSongs[0], genreSongs);
    }
  };

  const handleShuffle = () => {
    if (genreSongs.length > 0) {
      const shuffled = [...genreSongs];
      for (let i = shuffled.length - 1; i > 0; i--) {
        const j = Math.floor(Math.random() * (i + 1));
        [shuffled[i], shuffled[j]] = [shuffled[j], shuffled[i]];
      }
      playSong(shuffled[0], shuffled);
    }
  };

  const handleSongPlay = (songId: string) => {
    const songIndex = genreSongs.findIndex(s => s.id === songId);
    if (songIndex !== -1) {
      playSong(genreSongs[songIndex], genreSongs);
    }
  };

  if (!genreName) return null;

  return (
    <div className="h-full flex flex-col bg-surface-0 overflow-hidden">
      {/* Header */}
      <div className="bg-surface-1 border-b border-surface-highlight p-8">
        <button 
          onClick={() => navigate(-1)}
          className="flex items-center text-text-secondary hover:text-text-main mb-6 transition-colors"
        >
          <ChevronLeft size={20} className="mr-1" />
          Back
        </button>

        <div className="flex items-end gap-6">
          <div className="w-48 h-48 bg-surface-3 rounded-lg shadow-xl flex items-center justify-center overflow-hidden">
             {genreSongs[0]?.coverUrl ? (
                <img src={genreSongs[0].coverUrl} alt={genreName} className="w-full h-full object-cover" />
             ) : (
                <Music size={64} className="text-brand" />
             )}
          </div>
          
          <div className="flex-1">
            <h4 className="text-sm font-bold text-brand uppercase tracking-wider mb-2">Genre</h4>
            <h1 className="text-display font-bold text-text-main mb-4">{genreName}</h1>
            <div className="flex items-center text-text-secondary text-sm">
              <span>{genreSongs.length} songs</span>
              <span className="mx-2">•</span>
              <span>{formatTime(totalDuration)}</span>
            </div>
          </div>

          <div className="flex gap-3">
            <button
              onClick={handlePlay}
              className="flex items-center gap-2 px-8 py-3 bg-brand text-white rounded-full font-bold hover:bg-brand-hover transition-colors shadow-lg hover:scale-105 active:scale-95"
            >
              <Play size={20} fill="currentColor" />
              Play
            </button>
            <button
              onClick={handleShuffle}
              className="p-3 bg-surface-3 text-text-main rounded-full hover:bg-surface-4 transition-colors shadow-lg hover:scale-105 active:scale-95"
              title="Shuffle"
            >
              <Shuffle size={20} />
            </button>
          </div>
        </div>
      </div>

      {/* Song List */}
      <div className="flex-1 overflow-y-auto">
        <table className="w-full text-left border-collapse">
          <thead className="bg-surface-1 sticky top-0 z-10 text-text-subtle text-xs uppercase tracking-wider font-medium">
            <tr>
              <th className="px-6 py-3 w-12">#</th>
              <th className="px-6 py-3">Title</th>
              <th className="px-6 py-3">Artist</th>
              <th className="px-6 py-3">Album</th>
              <th className="px-6 py-3 w-24 text-right"><Clock size={14} className="inline" /></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-surface-highlight">
            {genreSongs.map((song, index) => {
              const isCurrentSong = currentSong?.id === song.id;
              
              return (
                <tr 
                  key={song.id}
                  onClick={() => handleSongPlay(song.id)}
                  onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                  className={`group hover:bg-surface-highlight/50 transition-colors cursor-pointer ${
                    isCurrentSong ? 'bg-surface-highlight/30' : ''
                  }`}
                >
                  <td className="px-6 py-3 text-text-subtle w-12 text-center">
                    <span className="group-hover:hidden">{index + 1}</span>
                    <button className="hidden group-hover:block text-text-main">
                      {isCurrentSong && isPlaying ? (
                        <div className="w-3 h-3 bg-brand rounded-full animate-pulse" />
                      ) : (
                        <Play size={14} fill="currentColor" />
                      )}
                    </button>
                  </td>
                  <td className="px-6 py-3">
                    <div className={`font-medium ${isCurrentSong ? 'text-brand' : 'text-text-main'}`}>
                      {song.title}
                    </div>
                  </td>
                  <td className="px-6 py-3 text-text-secondary hover:text-text-main transition-colors">
                    {song.artist}
                  </td>
                  <td className="px-6 py-3 text-text-secondary hover:text-text-main transition-colors">
                    {song.album}
                  </td>
                  <td className="px-6 py-3 text-text-subtle text-right font-mono text-xs">
                    {formatTime(song.duration)}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
};
