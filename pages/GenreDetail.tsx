/** Displays catalog songs for the selected genre. */

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
    <div className="h-full flex flex-col bg-surface-1 overflow-hidden">
      {/* Header */}
      <div className="bg-gradient-to-b from-surface-3/80 to-surface-1 border-b border-surface-border p-8">
        <button 
          onClick={() => navigate(-1)}
          className="flex items-center text-text-secondary hover:text-text-main mb-6 transition-colors text-sm font-medium"
        >
          <ChevronLeft size={18} className="mr-1" />
          Back
        </button>

        <div className="flex items-end gap-6">
          <div className="w-44 h-44 bg-surface-3 rounded-lg shadow-2xl flex items-center justify-center overflow-hidden">
             {genreSongs[0]?.coverUrl ? (
                <img src={genreSongs[0].coverUrl} alt={genreName} className="w-full h-full object-cover" />
             ) : (
                <Music size={56} className="text-brand" />
             )}
          </div>
          
          <div className="flex-1">
            <h4 className="text-xs font-bold text-text-secondary uppercase tracking-wider mb-1">Genre</h4>
            <h1 className="text-display font-extrabold tracking-tight text-text-main mb-3">{genreName}</h1>
            <div className="flex items-center text-text-secondary text-sm">
              <span className="font-semibold text-text-main">{genreSongs.length} songs</span>
              <span className="mx-2">•</span>
              <span>{formatTime(totalDuration)}</span>
            </div>
          </div>

          <div className="flex gap-3">
            <button
              onClick={handlePlay}
              className="flex items-center gap-2 px-7 py-3 bg-brand text-surface-0 rounded-full font-bold hover:bg-brand-hover transition-all shadow-lg hover:scale-105 active:scale-95"
            >
              <Play size={20} fill="currentColor" />
              Play
            </button>
            <button
              onClick={handleShuffle}
              className="p-3 bg-surface-3 text-text-main rounded-full hover:bg-surface-highlight transition-all shadow-lg hover:scale-105 active:scale-95"
              title="Shuffle"
            >
              <Shuffle size={20} />
            </button>
          </div>
        </div>
      </div>

      {/* Song List */}
      <div className="flex-1 overflow-y-auto px-6 pb-32">
        <table className="w-full text-left border-collapse">
          <thead className="bg-surface-1 sticky top-0 z-10 text-text-subtle text-xs uppercase tracking-wider font-semibold border-b border-surface-border">
            <tr>
              <th className="px-4 py-3 w-12 text-center">#</th>
              <th className="px-4 py-3">Title</th>
              <th className="px-4 py-3">Artist</th>
              <th className="px-4 py-3">Album</th>
              <th className="px-4 py-3 w-24 text-right"><Clock size={14} className="inline" /></th>
            </tr>
          </thead>
          <tbody>
            {genreSongs.map((song, index) => {
              const isCurrentSong = currentSong?.id === song.id;
              
              return (
                <tr 
                  key={song.id}
                  onClick={() => handleSongPlay(song.id)}
                  onContextMenu={(e) => openContextMenu(e, ContextMenuType.SONG, song)}
                  className={`group hover:bg-surface-hover transition-colors cursor-pointer ${
                    isCurrentSong ? 'bg-surface-hover/70' : ''
                  }`}
                >
                  <td className="px-4 py-2.5 text-text-subtle w-12 text-center">
                    <span className="group-hover:hidden text-xs tabular-nums">{index + 1}</span>
                    <button className="hidden group-hover:inline-flex text-text-main">
                      {isCurrentSong && isPlaying ? (
                        <div className="w-3 h-3 bg-brand rounded-full animate-pulse" />
                      ) : (
                        <Play size={14} fill="currentColor" />
                      )}
                    </button>
                  </td>
                  <td className="px-4 py-2.5">
                    <div className={`font-semibold text-sm ${isCurrentSong ? 'text-brand' : 'text-text-main'}`}>
                      {song.title}
                    </div>
                  </td>
                  <td className="px-4 py-2.5 text-sm text-text-secondary hover:text-text-main transition-colors">
                    {song.artist}
                  </td>
                  <td className="px-4 py-2.5 text-sm text-text-secondary hover:text-text-main transition-colors">
                    {song.album}
                  </td>
                  <td className="px-4 py-2.5 text-text-secondary text-right font-mono text-xs tabular-nums">
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
