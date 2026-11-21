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
import { Skeleton, SkeletonAlbumCard, SkeletonAlbumGrid } from '../components/Skeleton';
import { HomeSearchBar } from '../components/home/HomeSearchBar';
import { HomeCoverWallLayout } from '../components/home/layouts/HomeCoverWallLayout';
import { HomeDashboardLayout } from '../components/home/layouts/HomeDashboardLayout';
import { HomeShelvesLayout } from '../components/home/layouts/HomeShelvesLayout';
import { useHomeContent } from '../components/home/useHomeContent';
import { Page } from '../components/ui/Page';
import { useStore } from '../store';
import { HomeLayoutVariant } from '../types';

const HomeLoadingSkeleton: React.FC<{ variant: HomeLayoutVariant }> = ({ variant }) => {
  if (variant === 'coverWall') {
    return (
      <div className="space-y-8" data-layout-skeleton="coverWall">
        <Skeleton className="min-h-[310px] w-full rounded-xl" />
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, idx) => (
            <Skeleton key={idx} className="h-14 rounded-md" />
          ))}
        </div>
        <SkeletonAlbumGrid count={5} />
      </div>
    );
  }

  if (variant === 'dashboard') {
    return (
      <div className="space-y-8" data-layout-skeleton="dashboard">
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 6 }).map((_, idx) => (
            <Skeleton key={idx} className="h-14 rounded-md" />
          ))}
        </div>
        <div className="grid gap-8 2xl:grid-cols-2">
          {[0, 1].map((panel) => (
            <div key={panel} className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-3">
              {Array.from({ length: 6 }).map((_, idx) => (
                <SkeletonAlbumCard key={idx} />
              ))}
            </div>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-8" data-layout-skeleton="shelves">
      <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, idx) => (
          <Skeleton key={idx} className="h-14 rounded-md" />
        ))}
      </div>
      <Skeleton className="min-h-[180px] w-full rounded-lg" />
      <SkeletonAlbumGrid count={10} />
    </div>
  );
};

export const Home: React.FC = () => {
  const navigate = useNavigate();
  const homeLayoutVariant = useStore(state => state.homeLayoutVariant);
  const isLibraryInitializing = useStore(state => state.isLibraryInitializing);
  const isScanning = useStore(state => state.isScanning);
  const content = useHomeContent();
  const isComputingSmartMixes = Boolean(
    content.showSmartMixes && content.songs.length > 0 && content.smartMixes.length === 0
  );

  if (content.songs.length === 0) {
    return (
      <Page>
        <header className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <h1 className="text-section font-bold tracking-tight text-text-main md:text-display">Home</h1>
          <div className="w-full sm:w-80 md:w-96">
            <HomeSearchBar compact />
          </div>
        </header>
        {isLibraryInitializing || isScanning ? (
          <section aria-busy="true" aria-label="Loading library">
            <p role="status" className="mb-6 text-text-secondary">Loading library…</p>
            <div aria-hidden="true"><HomeLoadingSkeleton variant={homeLayoutVariant} /></div>
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
        <HomeCoverWallLayout content={content} isComputingSmartMixes={isComputingSmartMixes} />
      ) : homeLayoutVariant === 'dashboard' ? (
        <HomeDashboardLayout content={content} isComputingSmartMixes={isComputingSmartMixes} />
      ) : (
        <HomeShelvesLayout content={content} isComputingSmartMixes={isComputingSmartMixes} />
      )}
    </Page>
  );
};
