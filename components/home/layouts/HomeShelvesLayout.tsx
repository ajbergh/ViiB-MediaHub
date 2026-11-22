/** Arranges Home content into music shelves. */

import React from 'react';
import { useNavigate } from 'react-router';
import { SmartMixCard } from '../../SmartMixCard';
import { Skeleton } from '../../Skeleton';
import { HomeAlbumCard } from '../HomeAlbumCard';
import { HomeArtistCard } from '../HomeArtistCard';
import { HomeQuickTile } from '../HomeQuickTile';
import { HomeRecentTracks } from '../HomeRecentTracks';
import { HomeSearchBar } from '../HomeSearchBar';
import { HomeShelf } from '../HomeShelf';
import { HomeSpotlight } from '../HomeSpotlight';
import { HomeStatsCompact } from '../HomeStatsCompact';
import { HomeContent } from '../useHomeContent';

interface HomeShelvesLayoutProps {
  content: HomeContent;
  isComputingSmartMixes?: boolean;
}

const getGreeting = () => {
  const hour = new Date().getHours();
  if (hour < 12) return 'Good morning';
  if (hour < 18) return 'Good afternoon';
  return 'Good evening';
};

export const HomeShelvesLayout: React.FC<HomeShelvesLayoutProps> = ({
  content,
  isComputingSmartMixes = Boolean(content.showSmartMixes && content.songs.length > 0 && content.smartMixes.length === 0),
}) => {
  const navigate = useNavigate();

  return (
    <div className="space-y-2">
      <header className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <h1 className="text-section font-bold tracking-tight text-text-main md:text-display">
          {getGreeting()}
        </h1>
        <div className="w-full sm:w-80 md:w-96">
          <HomeSearchBar compact />
        </div>
      </header>

      {content.quickTiles.length > 0 && (
        <section className="mb-8 grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3">
          {content.quickTiles.slice(0, 6).map((tile) => (
            <HomeQuickTile key={tile.id} item={tile} compact />
          ))}
        </section>
      )}

      <section className="mb-8">
        <HomeSpotlight item={content.spotlightItem} compact />
      </section>

      {content.showSmartMixes && (content.smartMixes.length > 0 || isComputingSmartMixes) ? (
        <HomeShelf title="Made For You" subtitle="Auto-generated mixes from your library">
          {content.smartMixes.length > 0
            ? content.smartMixes.map((mix) => (
                <SmartMixCard
                  key={mix.id}
                  mix={mix}
                  onPlay={() => content.playMix(mix)}
                  onClick={() => content.navigateToSmartMix(mix)}
                  onContextMenu={(e) => content.openSmartMixMenu(e, mix)}
                />
              ))
            : Array.from({ length: 5 }).map((_, idx) => (
                <div key={idx} className="w-44 flex-shrink-0 rounded-lg p-3" aria-hidden="true">
                  <Skeleton className="mb-3 aspect-square w-full rounded-md" />
                  <Skeleton className="h-4 w-3/4" />
                  <Skeleton className="mt-1.5 h-3 w-full" />
                  <Skeleton className="mt-1 h-3 w-2/3" />
                </div>
              ))}
        </HomeShelf>
      ) : null}

      <HomeShelf
        title="Recently Added Albums"
        actionLabel="Albums"
        onAction={() => navigate('/albums')}
      >
        {content.recentlyAddedAlbums.slice(0, 10).map((album) => (
          <HomeAlbumCard
            key={`${album.name}-${album.artist}`}
            album={album}
            onClick={() => content.navigateToAlbum(album)}
            onPlay={() => content.playAlbum(album)}
            onContextMenu={(e) => content.openAlbumMenu(e, album)}
          />
        ))}
      </HomeShelf>

      <HomeShelf
        title="Top Artists"
        actionLabel="Artists"
        onAction={() => navigate('/artists')}
      >
        {content.topArtistsByLibrary.slice(0, 10).map((artist) => (
          <HomeArtistCard
            key={artist.name}
            artist={artist}
            imageUrl={content.getArtistImage(artist)}
            onClick={() => content.navigateToArtist(artist)}
            onPlay={() => content.playArtist(artist)}
            onContextMenu={(e) => content.openArtistMenu(e, artist)}
          />
        ))}
      </HomeShelf>

      <HomeRecentTracks
        tracks={content.recentlyPlayedTracks}
        formatRelativeTime={content.formatRelativeTime}
        onPlay={content.playTrack}
        onContextMenu={content.openSongMenu}
      />

      <HomeStatsCompact
        songsCount={content.songs.length}
        albumsCount={content.albums.length}
        artistsCount={content.artists.length}
      />
    </div>
  );
};
