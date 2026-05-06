import React from 'react';
import { useNavigate } from 'react-router-dom';
import { SmartMixCard } from '../../SmartMixCard';
import { HomeAlbumCard } from '../HomeAlbumCard';
import { HomeArtistCard } from '../HomeArtistCard';
import { HomeQuickTile } from '../HomeQuickTile';
import { HomeSearchBar } from '../HomeSearchBar';
import { HomeShelf } from '../HomeShelf';
import { HomeStatsCompact } from '../HomeStatsCompact';
import { HomeContent } from '../useHomeContent';

interface HomeDashboardLayoutProps {
  content: HomeContent;
}

export const HomeDashboardLayout: React.FC<HomeDashboardLayoutProps> = ({ content }) => {
  const navigate = useNavigate();

  return (
    <div>
      <header className="mb-6 grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(320px,520px)] lg:items-end">
        <div>
          <p className="text-sm font-semibold uppercase tracking-wide text-brand">Compact Dashboard</p>
          <h1 className="mt-2 text-section font-semibold text-text-main">Jump into your library</h1>
        </div>
        <HomeSearchBar compact />
      </header>

      {content.quickTiles.length > 0 ? (
        <HomeShelf
          title="Jump Back In"
          subtitle="Recent tracks and fast starts"
          contentClassName="grid grid-cols-1 gap-3 overflow-visible pb-0 md:grid-cols-2 xl:grid-cols-3"
          className="mb-8"
        >
          {content.quickTiles.slice(0, 6).map((tile) => (
            <HomeQuickTile key={tile.id} item={tile} compact className="min-w-0" />
          ))}
        </HomeShelf>
      ) : null}

      <div className="grid gap-8 2xl:grid-cols-2">
        <HomeShelf
          title="Top Albums"
          subtitle="Ranked by listening when available"
          actionLabel="Albums"
          onAction={() => navigate('/albums')}
          contentClassName="grid grid-cols-2 gap-4 overflow-visible pb-0 sm:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-3"
        >
          {content.topAlbumsByPlays.slice(0, 8).map((album) => (
            <HomeAlbumCard
              key={`${album.name}-${album.artist}`}
              album={album}
              size="compact"
              className="w-full"
              onClick={() => content.navigateToAlbum(album)}
              onPlay={() => content.playAlbum(album)}
              onContextMenu={(e) => content.openAlbumMenu(e, album)}
            />
          ))}
        </HomeShelf>

        <HomeShelf
          title="Top Artists"
          subtitle="Your most represented artists"
          actionLabel="Artists"
          onAction={() => navigate('/artists')}
          contentClassName="grid grid-cols-2 gap-4 overflow-visible pb-0 sm:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-3"
        >
          {content.topArtistsByPlays.slice(0, 8).map((artist) => (
            <HomeArtistCard
              key={artist.name}
              artist={artist}
              imageUrl={content.getArtistImage(artist)}
              size="compact"
              className="w-full"
              onClick={() => content.navigateToArtist(artist)}
              onPlay={() => content.playArtist(artist)}
              onContextMenu={(e) => content.openArtistMenu(e, artist)}
            />
          ))}
        </HomeShelf>
      </div>

      <HomeShelf
        title="Fresh From Your Library"
        subtitle="Recently imported albums"
        actionLabel="Albums"
        onAction={() => navigate('/albums')}
      >
        {content.recentlyAddedAlbums.slice(0, 10).map((album) => (
          <HomeAlbumCard
            key={`${album.name}-${album.artist}`}
            album={album}
            size="compact"
            onClick={() => content.navigateToAlbum(album)}
            onPlay={() => content.playAlbum(album)}
            onContextMenu={(e) => content.openAlbumMenu(e, album)}
          />
        ))}
      </HomeShelf>

      {content.showSmartMixes && content.smartMixes.length > 0 ? (
        <HomeShelf title="Smart Mixes" subtitle="Generated shortcuts when you want the app to choose">
          {content.smartMixes.map((mix) => (
            <SmartMixCard
              key={mix.id}
              mix={mix}
              onPlay={() => content.playMix(mix)}
              onClick={() => content.navigateToSmartMix(mix)}
              onContextMenu={(e) => content.openSmartMixMenu(e, mix)}
            />
          ))}
        </HomeShelf>
      ) : null}

      <HomeStatsCompact
        songsCount={content.songs.length}
        albumsCount={content.albums.length}
        artistsCount={content.artists.length}
      />
    </div>
  );
};
