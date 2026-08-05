import React from 'react';
import { useNavigate } from 'react-router';
import { SmartMixCard } from '../../SmartMixCard';
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
}

export const HomeShelvesLayout: React.FC<HomeShelvesLayoutProps> = ({ content }) => {
  const navigate = useNavigate();

  return (
    <div className="space-y-2">
      <header className="mb-8">
        <div className="mb-5 flex flex-col gap-2">
          <p className="text-sm font-semibold uppercase tracking-wide text-brand">Home</p>
          <h1 className="text-display text-text-main">Let's ViiB</h1>
        </div>
        <div className="max-w-3xl">
          <HomeSearchBar />
        </div>
      </header>

      <section className="mb-10 grid gap-5 xl:grid-cols-[minmax(0,1.35fr)_minmax(360px,0.65fr)]">
        <HomeSpotlight item={content.spotlightItem} className="h-full" />
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1">
          {content.quickTiles.slice(0, 4).map((tile) => (
            <HomeQuickTile key={tile.id} item={tile} compact className="min-w-0" />
          ))}
        </div>
      </section>

      <HomeShelf
        title="Recently Added Albums"
        subtitle="Fresh from your library"
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
        subtitle="Artists with the most music in your library"
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

      {content.showSmartMixes && content.smartMixes.length > 0 ? (
        <HomeShelf title="Smart Mixes" subtitle="Auto-generated starts for your next session">
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
