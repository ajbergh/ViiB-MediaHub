/**
 * ViiB MediaHub - Home Page
 *
 * Renders the selected Home layout variant. Layout preference is stored in the
 * UI slice and can be changed from Settings > Personalization.
 *
 * @module Home
 */

import React, { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { EmptyLibrary } from '../components/EmptyState';
import { HomeSearchBar } from '../components/home/HomeSearchBar';
import { HomeCoverWallLayout } from '../components/home/layouts/HomeCoverWallLayout';
import { HomeDashboardLayout } from '../components/home/layouts/HomeDashboardLayout';
import { HomeShelvesLayout } from '../components/home/layouts/HomeShelvesLayout';
import { useHomeContent } from '../components/home/useHomeContent';
import { Page } from '../components/ui/Page';
import { useStore } from '../store';

export const Home: React.FC = () => {
  const navigate = useNavigate();
  const { homeLayoutVariant, refreshSmartMixes } = useStore();
  const content = useHomeContent();

  useEffect(() => {
    if (content.songs.length > 0) {
      refreshSmartMixes();
    }
  }, [content.songs.length, refreshSmartMixes]);

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
        <EmptyLibrary onOpenSettings={() => navigate('/settings')} />
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
