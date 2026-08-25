import React, { useMemo, useState, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { useStore } from '../store';
import { Music, Search, ChevronRight, Play, Shuffle, Loader2 } from 'lucide-react';
import { Song } from '../types';
import { api, GenreStat } from '../services/api';
import { CardSizeSlider } from '../components/ui/CardSizeSlider';
import { TextInput } from '../components/ui/TextInput';

/**
 * Genres Page Component
 * 
 * Displays a grid of all music genres found in the user's library with statistics.
 * Genre data is pre-computed by the backend in the genre_stats table.
 * 
 * Genre Handling:
 * - Individual genres are tracked separately (not combined strings)
 * - A song with multiple genres (e.g., ["Rock", "Alternative Rock"]) contributes to each
 * - Each genre card shows: name, track count, top 3 artists, and representative cover
 * 
 * Cover Images:
 * - Cover URLs come from /api/cover/{songId} endpoint
 * - Selected from the most popular artist in each genre
 * - Served through the backend's serveCover handler with proper security checks
 * 
 * Features:
 * - Search/filter genres by name
 * - Play or shuffle all tracks in a genre
 * - Navigate to genre detail page
 * - Auto-refresh on library updates
 */
export const Genres: React.FC = () => {
  const { songs, playSong } = useStore();
  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState('');
  const [genres, setGenres] = useState<GenreStat[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [cardCols, setCardCols] = useState(() => Number(localStorage.getItem('genres-card-cols') ?? 4));
  const handleCardColsChange = (v: number) => { setCardCols(v); localStorage.setItem('genres-card-cols', String(v)); };

  useEffect(() => {
    const fetchGenres = async () => {
      try {
        // Fetch pre-computed genre statistics from backend
        // Backend parses each song's genre array and tracks genres individually
        const data = await api.getGenres();
        setGenres(data);
      } catch (error) {
        console.error('Failed to fetch genres:', error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchGenres();

    // Listen for library updates to refresh genre statistics
    // This handles both scan completion and background enrichment updates
    const handleLibraryUpdate = () => {
      fetchGenres();
    };

    const handleEnrichmentComplete = () => {
      // Genre enrichment may add new genres or increase genre counts
      fetchGenres();
    };

    window.addEventListener('library_updated', handleLibraryUpdate);
    window.addEventListener('enrichment_complete', handleEnrichmentComplete);
    
    return () => {
      window.removeEventListener('library_updated', handleLibraryUpdate);
      window.removeEventListener('enrichment_complete', handleEnrichmentComplete);
    };
  }, []);

  const filteredGenres = useMemo(() => {
    if (!searchQuery) return genres;
    const query = searchQuery.toLowerCase();
    return genres.filter(g => g.name.toLowerCase().includes(query));
  }, [genres, searchQuery]);

  const handlePlayGenre = (e: React.MouseEvent, genreName: string, shuffle: boolean) => {
    e.stopPropagation();
    const genreSongs = songs.filter(s => s.genre?.includes(genreName));
    if (genreSongs.length > 0) {
      if (shuffle) {
        // Fisher-Yates shuffle
        for (let i = genreSongs.length - 1; i > 0; i--) {
          const j = Math.floor(Math.random() * (i + 1));
          [genreSongs[i], genreSongs[j]] = [genreSongs[j], genreSongs[i]];
        }
      }
      playSong(genreSongs[0], genreSongs);
    }
  };

  if (isLoading) {
    return (
      <div className="h-full flex items-center justify-center bg-surface-0">
        <Loader2 className="animate-spin text-brand" size={48} />
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-surface-0 overflow-hidden">
      {/* Header */}
      <div className="p-8 pb-4">
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-display text-text-main mb-2">Genres</h1>
            <p className="text-text-secondary">
              {genres.length} genres found in your library
            </p>
          </div>
          <CardSizeSlider value={cardCols} onChange={handleCardColsChange} />
        </div>

        {/* Search */}
        <div className="max-w-md">
          <TextInput
            leftIcon={<Search size={18} className="text-text-secondary" aria-hidden="true" />}
            type="text"
            placeholder="Search genres…"
            aria-label="Search genres"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="rounded-full"
          />
        </div>
      </div>

      {/* Genre Grid */}
      <div className="flex-1 overflow-y-auto p-8 pt-0">
        <div
          className="grid gap-4"
          style={{ gridTemplateColumns: `repeat(${cardCols}, minmax(0, 1fr))` }}
        >
          {filteredGenres.map((genre) => (
            <div
              key={genre.name}
              onClick={() => navigate(`/genres/${encodeURIComponent(genre.name)}`)}
              className="group bg-surface-1 hover:bg-surface-2 border border-surface-highlight rounded-xl p-4 transition-all cursor-pointer hover:shadow-lg hover:shadow-black/20 hover:-translate-y-1 relative overflow-hidden"
            >
              {/* Background Gradient based on cover or random color */}
              <div className="absolute inset-0 bg-gradient-to-br from-brand/5 to-transparent opacity-0 group-hover:opacity-100 transition-opacity" />

              <div className="flex items-start justify-between mb-4 relative z-10">
                <div className="w-12 h-12 rounded-lg bg-surface-3 flex items-center justify-center overflow-hidden shadow-md">
                  {genre.coverUrl ? (
                    <img src={genre.coverUrl} alt={genre.name} className="w-full h-full object-cover" />
                  ) : (
                    <Music size={24} className="text-brand" />
                  )}
                </div>
                
                <div className="flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                  <button
                    onClick={(e) => handlePlayGenre(e, genre.name, false)}
                    className="p-2 bg-brand text-white rounded-full hover:bg-brand-hover shadow-lg hover:scale-105 transition-all"
                    title="Play Genre"
                  >
                    <Play size={16} fill="currentColor" />
                  </button>
                  <button
                    onClick={(e) => handlePlayGenre(e, genre.name, true)}
                    className="p-2 bg-surface-3 text-text-main rounded-full hover:bg-surface-4 shadow-lg hover:scale-105 transition-all"
                    title="Shuffle Genre"
                  >
                    <Shuffle size={16} />
                  </button>
                </div>
              </div>

              <div className="relative z-10">
                <h3 className="text-lg font-bold text-white mb-1 truncate" title={genre.name}>
                  {genre.name}
                </h3>
                <p className="text-sm text-brand font-medium mb-2">
                  {genre.count} tracks
                </p>
                <div className="text-xs text-text-subtle truncate">
                  {genre.topArtists?.join(', ') || ''}
                </div>
              </div>
            </div>
          ))}
        </div>

        {filteredGenres.length === 0 && (
          <div className="text-center py-20">
            <div className="inline-flex items-center justify-center w-16 h-16 rounded-full bg-surface-1 mb-4">
              <Search size={32} className="text-text-subtle" />
            </div>
            <h3 className="text-xl font-bold text-white mb-2">No genres found</h3>
            <p className="text-text-secondary">
              Try adjusting your search terms
            </p>
          </div>
        )}
      </div>
    </div>
  );
};
