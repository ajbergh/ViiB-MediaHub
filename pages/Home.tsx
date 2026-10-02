/**
 * ViiB MediaHub - Home Page
 *
 * Renders the selected Home layout variant. Layout preference is stored in the
 * UI slice and can be changed from Settings > Appearance & Now Playing.
 *
 * @module Home
 */

import React from 'react';
import { useNavigate } from 'react-router';
import { EmptyLibrary } from '../components/EmptyState';
import { SkeletonAlbumGrid } from '../components/Skeleton';
import { HomeSearchBar } from '../components/home/HomeSearchBar';
import { HomeCoverWallLayout } from '../components/home/layouts/HomeCoverWallLayout';
import { HomeDashboardLayout } from '../components/home/layouts/HomeDashboardLayout';
import { HomeShelvesLayout } from '../components/home/layouts/HomeShelvesLayout';
import { useHomeContent } from '../components/home/useHomeContent';
import { Page } from '../components/ui/Page';
import { useStore } from '../store';

export const Home: React.FC = () => {
  const navigate = useNavigate();
  const homeLayoutVariant = useStore(state => state.homeLayoutVariant);
  const isLibraryInitializing = useStore(state => state.isLibraryInitializing);
  const isScanning = useStore(state => state.isScanning);
  const content = useHomeContent();

  if (content.songs.length === 0) {
    return (
      <Page>
        <header className="mb-8">
          <div className="mb-5 flex flex-col gap-2">
            <p className="text-sm font-semibold uppercase tracking-wide text-brand">Home</p>
            <h1 className="text-display text-text-main">Let's ViiB</h1>
          </div>
          <div className="max-w-3xl">
            <HomeSearchBar />
          </div>
        </header>
        {isLibraryInitializing || isScanning ? (
          <section aria-busy="true" aria-label="Loading library">
            <p role="status" className="mb-6 text-text-secondary">Loading library…</p>
            <div aria-hidden="true"><SkeletonAlbumGrid count={10} /></div>
          </section>
        ) : (
          <EmptyLibrary onOpenSettings={() => navigate('/settings')} />
        )}
      </Page>
    );
  }

  return (
    <Page>
      {homeLayoutVariant === 'coverWall' ? (
        <HomeCoverWallLayout content={content} />
      ) : homeLayoutVariant === 'dashboard' ? (
        <HomeDashboardLayout content={content} />
      ) : (
        <HomeShelvesLayout content={content} />
      )}
    </Page>
  );
};
