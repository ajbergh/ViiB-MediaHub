/** Displays genre groupings derived from the ViiB catalog. */

import React, { useMemo, useState, useEffect } from 'react';
import { useNavigate } from 'react-router';
import { useStore } from '../store';
import { Music, Search, Play, Shuffle, Loader2, ArrowUpDown, ChevronDown } from 'lucide-react';
import { api, GenreStat } from '../services/api';
import { Button } from '../components/ui/Button';
import { Menu, MenuItem } from '../components/ui/Menu';
import { CardSizeSlider, MAX_CARD_COLUMNS } from '../components/ui/CardSizeSlider';
import { LibraryListHeader, LibraryListRow } from '../components/ui/LibraryList';
import { TextInput } from '../components/ui/TextInput';
import { generateGradient } from '../utils';

type GenreSortOption = 'count-desc' | 'count-asc' | 'name-asc' | 'name-desc';

const genreSortLabels: Record<GenreSortOption, string> = {
  'count-desc': 'Most Tracks',
  'count-asc': 'Fewest Tracks',
  'name-asc': 'Name (A-Z)',
  'name-desc': 'Name (Z-A)',
};

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
  const songs = useStore(state => state.songs);
  const playSong = useStore(state => state.playSong);
  const navigate = useNavigate();
  const [searchQuery, setSearchQuery] = useState('');
  const [sortBy, setSortBy] = useState<GenreSortOption>('count-desc');
  const [showSortMenu, setShowSortMenu] = useState(false);
  const [genres, setGenres] = useState<GenreStat[]>([]);
  const [failedCovers, setFailedCovers] = useState<Record<string, boolean>>({});
  const [isLoading, setIsLoading] = useState(true);
  const [cardCols, setCardCols] = useState(() => Number(localStorage.getItem('genres-card-cols') ?? 4));
  const listView = cardCols === MAX_CARD_COLUMNS;
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

  const sortedGenres = useMemo(() => {
    const sorted = [...genres];
    switch (sortBy) {
      case 'count-desc':
        return sorted.sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
      case 'count-asc':
        return sorted.sort((a, b) => a.count - b.count || a.name.localeCompare(b.name));
      case 'name-asc':
        return sorted.sort((a, b) => a.name.localeCompare(b.name));
      case 'name-desc':
        return sorted.sort((a, b) => b.name.localeCompare(a.name));
      default:
        return sorted;
    }
  }, [genres, sortBy]);

  const filteredGenres = useMemo(() => {
    if (!searchQuery) return sortedGenres;
    const query = searchQuery.toLowerCase();
    return sortedGenres.filter(g => g.name.toLowerCase().includes(query));
  }, [sortedGenres, searchQuery]);

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
      <div className="h-full flex items-center justify-center bg-surface-1">
        <Loader2 className="animate-spin text-brand" size={48} />
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-surface-1 overflow-hidden">
      {/* Header */}
      <div className="px-6 md:px-8 pt-6 pb-4 flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <h1 className="text-display font-bold tracking-tight text-text-main">Genres</h1>
          <p className="text-sm text-text-secondary mt-1">
            {filteredGenres.length}{searchQuery ? ` of ${genres.length}` : ''} categories in your library
          </p>
        </div>
        <div className="flex items-center gap-3">
          <CardSizeSlider listAtMinimum value={cardCols} onChange={handleCardColsChange} />
          <div className="relative">
            <Button
              variant="secondary"
              onClick={() => setShowSortMenu(!showSortMenu)}
              className="rounded-full whitespace-nowrap"
              leftIcon={<ArrowUpDown size={16} className="text-text-secondary" aria-hidden="true" />}
              rightIcon={<ChevronDown size={16} className={`text-text-secondary transition-transform ${showSortMenu ? 'rotate-180' : ''}`} aria-hidden="true" />}
              aria-haspopup="menu"
              aria-expanded={showSortMenu}
            >
              {genreSortLabels[sortBy]}
            </Button>
            {showSortMenu && (
              <>
                <button
                  type="button"
                  tabIndex={-1}
                  aria-label="Close sort menu"
                  className="fixed inset-0 z-40"
                  onClick={() => setShowSortMenu(false)}
                />
                <Menu
                  className="absolute right-0 top-full mt-2 z-50 min-w-[180px]"
                  onRequestClose={() => setShowSortMenu(false)}
                >
                  {(Object.keys(genreSortLabels) as GenreSortOption[]).map((option) => (
                    <MenuItem
                      key={option}
                      onClick={() => { setSortBy(option); setShowSortMenu(false); }}
                      active={sortBy === option}
                    >
                      {genreSortLabels[option]}
                    </MenuItem>
                  ))}
                </Menu>
              </>
            )}
          </div>
          <div className="w-full md:w-72">
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
      </div>

      {/* Genre Grid */}
      <div className="flex-1 overflow-y-auto px-6 md:px-8 pb-24 pt-2">
        {listView && <LibraryListHeader title="Genre" detail="Top artists" stats="Tracks" />}
        <div
          className={listView ? 'grid gap-0' : 'grid gap-4'}
          style={{ gridTemplateColumns: `repeat(${listView ? 1 : cardCols}, minmax(0, 1fr))` }}
        >
          {filteredGenres.map((genre, index) => {
            const hasValidCover = Boolean(genre.coverUrl && !failedCovers[genre.coverUrl]);
            return listView ? (
            <LibraryListRow key={genre.name} index={index} name={genre.name}
              artwork={hasValidCover ? <img src={genre.coverUrl} alt="" onError={() => genre.coverUrl && setFailedCovers(prev => ({ ...prev, [genre.coverUrl!]: true }))} className="w-full h-full object-cover" /> : <Music className="w-full h-full p-2 text-brand" />}
              detail={genre.topArtists?.join(', ')} stats={String(genre.count)}
              onOpen={() => navigate(`/genres/${encodeURIComponent(genre.name)}`)}
              actions={<><button type="button" aria-label={`Play ${genre.name}`} className="p-2 rounded hover:bg-surface-3 text-brand" onClick={e => handlePlayGenre(e, genre.name, false)}><Play size={16} /></button><button type="button" aria-label={`Shuffle ${genre.name}`} className="p-2 rounded hover:bg-surface-3 text-text-secondary" onClick={e => handlePlayGenre(e, genre.name, true)}><Shuffle size={16} /></button></>} />
          ) : (
            <div
              key={genre.name}
              onClick={() => navigate(`/genres/${encodeURIComponent(genre.name)}`)}
              style={{ background: generateGradient(genre.name) }}
              className="group relative h-40 rounded-lg p-4 cursor-pointer overflow-hidden shadow-md transition-transform duration-200 hover:scale-[1.01]"
            >
              <div className="absolute inset-0 bg-surface-0/25 group-hover:bg-surface-0/15 transition-colors" />

              <div className="relative z-10 flex flex-col h-full justify-between">
                <div className="max-w-[75%]">
                  <h3 className="text-lg font-extrabold tracking-tight text-text-main leading-snug line-clamp-2 drop-shadow-sm" title={genre.name}>
                    {genre.name}
                  </h3>
                  <p className="text-xs font-semibold text-text-main/80 mt-1">
                    {genre.count} {genre.count === 1 ? 'track' : 'tracks'}
                  </p>
                  {genre.topArtists && genre.topArtists.length > 0 && (
                    <p className="text-[11px] text-text-main/70 truncate mt-0.5">
                      {genre.topArtists.join(', ')}
                    </p>
                  )}
                </div>

                <div className="flex items-center gap-2 opacity-0 translate-y-1 group-hover:opacity-100 group-hover:translate-y-0 transition-all duration-200">
                  <button
                    type="button"
                    aria-label={`Play ${genre.name}`}
                    onClick={(e) => handlePlayGenre(e, genre.name, false)}
                    className="w-9 h-9 bg-brand text-surface-0 rounded-full hover:bg-brand-hover flex items-center justify-center shadow-lg hover:scale-105 transition-all"
                    title="Play Genre"
                  >
                    <Play size={16} className="fill-current ml-0.5" />
                  </button>
                  <button
                    type="button"
                    aria-label={`Shuffle ${genre.name}`}
                    onClick={(e) => handlePlayGenre(e, genre.name, true)}
                    className="w-9 h-9 bg-surface-0/60 backdrop-blur-sm text-text-main rounded-full hover:bg-surface-0/80 flex items-center justify-center shadow-lg hover:scale-105 transition-all"
                    title="Shuffle Genre"
                  >
                    <Shuffle size={15} />
                  </button>
                </div>
              </div>

              {/* Spotify-style rotated artwork in bottom-right corner */}
              <div className="absolute -bottom-2 -right-2 w-24 h-24 rounded-md bg-surface-3/80 shadow-xl rotate-[25deg] translate-x-2 translate-y-1 overflow-hidden flex items-center justify-center pointer-events-none">
                {hasValidCover ? (
                  <img
                    src={genre.coverUrl}
                    alt=""
                    onError={() => genre.coverUrl && setFailedCovers(prev => ({ ...prev, [genre.coverUrl!]: true }))}
                    className="w-full h-full object-cover"
                  />
                ) : (
                  <Music size={32} className="text-text-main/50" />
                )}
              </div>
            </div>
          );
          })}
        </div>

        {filteredGenres.length === 0 && (
          <div className="text-center py-20">
            <div className="inline-flex items-center justify-center w-16 h-16 rounded-full bg-surface-2 mb-4">
              <Search size={32} className="text-text-subtle" />
            </div>
            <h3 className="text-xl font-bold text-text-main mb-2">No genres found</h3>
            <p className="text-text-secondary">
              Try adjusting your search terms
            </p>
          </div>
        )}
      </div>
    </div>
  );
};
