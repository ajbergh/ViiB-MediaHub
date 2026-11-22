
import React, { useEffect, useState } from 'react';
import { Link } from 'react-router';
import { api } from '../../services/api';

/** No unfinished podcast features should appear in normal production UI.
 * Preview is explicitly enabled in Settings and defaults off. */
export const PodcastPreviewGate: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [enabled, setEnabled] = useState<boolean | null>(null);
  useEffect(() => {
    let alive = true;
    void api.getSetting('podcasts_preview_enabled')
      .then(value => { if (alive) setEnabled(value === 'true'); })
      .catch(() => { if (alive) setEnabled(false); });
    return () => { alive = false; };
  }, []);
  if (enabled === null) return <p className="p-8 text-text-secondary" role="status">Checking Podcast Preview availability…</p>;
  if (!enabled) return (
    <section className="mx-auto mt-16 max-w-lg rounded-xl border border-surface-3 bg-surface-2 p-8">
      <h1 className="text-xl font-bold text-text-main">Podcast Preview is disabled</h1>
      <p className="mt-3 text-text-secondary">Podcast subscriptions are under development. Enable the preview in Settings to test RSS following and episode browsing. Playback and downloads are not available yet.</p>
      <Link className="mt-5 inline-block rounded-lg bg-brand px-4 py-2 font-semibold text-surface-0" to="/settings" state={{ tab: 'integrations' }}>Open Settings</Link>
    </section>
  );
  return <>{children}</>;
};
