import React from 'react';
import { useNavigate } from 'react-router-dom';
import { Music2 } from 'lucide-react';
import { SmartMixCard } from '../../SmartMixCard';
import { coverBackground } from '../../../utils';
import { HomeAlbumCard } from '../HomeAlbumCard';
import { HomeArtistCard } from '../HomeArtistCard';
import { HomeQuickTile } from '../HomeQuickTile';
import { HomeSearchBar } from '../HomeSearchBar';
import { HomeShelf } from '../HomeShelf';
import { HomeSpotlight } from '../HomeSpotlight';
import { HomeStatsCompact } from '../HomeStatsCompact';
import { HomeContent } from '../useHomeContent';

interface HomeCoverWallLayoutProps {
  content: HomeContent;
}

const CoverMosaic: React.FC<{ content: HomeContent }> = ({ content }) => {
  const albums = content.coverWallAlbums.slice(0, 14);

  if (albums.length < 4) {
    return <HomeSpotlight item={content.spotlightItem} compact className="h-full" />;
  }

  return (
    <div className="relative min-h-[310px] overflow-hidden rounded-xl bg-surface-2 ring-1 ring-surface-3">
      <div className="absolute inset-0 grid grid-cols-4 gap-2 p-3 sm:grid-cols-5 lg:grid-cols-7">
        {albums.map((album, index) => (
          <button
            key={`${album.name}-${album.artist}-${index}`}
            type="button"
            onClick={() => content.navigateToAlbum(album)}
            onContextMenu={(e) => content.openAlbumMenu(e, album)}
            className={`group overflow-hidden rounded-lg bg-surface-3 shadow-lg ring-1 ring-black/20 transition-transform hover:scale-[1.02] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70 ${
              index % 5 === 0 ? 'row-span-2' : ''
            } ${index % 7 === 0 ? 'col-span-2' : ''}`}
            style={{ background: coverBackground(album.coverUrl, album.name) }}
            aria-label={`Open ${album.name}`}
          >
            {!album.coverUrl ? (
              <span className="flex h-full min-h-24 w-full items-center justify-center text-3xl font-bold text-white/30">
                {album.name.charAt(0)}
              </span>
            ) : null}
            <span className="block h-full min-h-24 bg-black/0 transition-colors group-hover:bg-black/20" />
          </button>
        ))}
      </div>

      <div className="absolute inset-0 bg-gradient-to-r from-surface-0 via-surface-0/70 to-surface-0/10" />
      <div className="relative z-10 flex min-h-[310px] max-w-3xl flex-col justify-end p-6 md:p-8">
        <div className="mb-4 inline-flex w-fit items-center gap-2 rounded-full bg-surface-1/90 px-3 py-1 text-sm font-semibold text-text-secondary ring-1 ring-surface-3">
          <Music2 size={16} className="text-brand" aria-hidden="true" />
          <span>Cover Wall</span>
        </div>
        <h1 className="text-display text-text-main">Your library, up front</h1>
        <p className="mt-3 max-w-xl text-text-secondary">
          Browse from real album artwork, then keep moving through recent albums, top artists, and Smart Mixes.
        </p>
        <div className="mt-6 max-w-2xl">
          <HomeSearchBar />
        </div>
      </div>
    </div>
  );
};

export const HomeCoverWallLayout: React.FC<HomeCoverWallLayoutProps> = ({ content }) => {
  const navigate = useNavigate();

  return (
    <div>
      <section className="mb-8">
        <CoverMosaic content={content} />
      </section>

      {content.quickTiles.length > 0 ? (
        <HomeShelf
          title="Featured Starts"
          subtitle="A few fast choices from your recent activity"
          contentClassName="grid grid-cols-1 gap-3 overflow-visible pb-0 md:grid-cols-2 xl:grid-cols-3"
        >
          {content.quickTiles.slice(0, 6).map((tile) => (
            <HomeQuickTile key={tile.id} item={tile} compact className="min-w-0" />
          ))}
        </HomeShelf>
      ) : null}

      <HomeShelf
        title="Recently Added Albums"
        subtitle="New artwork and records from your library"
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
        subtitle="Artist artwork and library depth"
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

      {content.showSmartMixes && content.smartMixes.length > 0 ? (
        <HomeShelf title="Smart Mixes" subtitle="Generated mixes below the artwork view">
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
