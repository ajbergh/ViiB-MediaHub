/** Defines shared library-list header and row presentation. */

import React from 'react';

const columns = 'grid grid-cols-[28px_minmax(0,1fr)_80px] md:grid-cols-[36px_minmax(0,2fr)_minmax(0,1fr)_120px] gap-3 items-center';

export const LibraryListHeader: React.FC<{ title: string; detail?: string; stats: string }> = ({ title, detail, stats }) => (
  <div className="flex items-center px-4 py-3 bg-surface-1 border-b border-surface-3 text-xs uppercase tracking-wider text-text-secondary" aria-hidden="true">
    <div className={`${columns} flex-1 min-w-0`}><span>#</span><span>{title}</span><span className="hidden md:block">{detail}</span><span className="text-right">{stats}</span></div>
    <div className="w-24 shrink-0" />
  </div>
);

export const LibraryListRow: React.FC<{
  index: number;
  name: string;
  artwork: React.ReactNode;
  detail?: string;
  stats: string;
  onOpen: () => void;
  onContextMenu?: React.MouseEventHandler<HTMLDivElement>;
  actions?: React.ReactNode;
}> = ({ index, name, artwork, detail, stats, onOpen, onContextMenu, actions }) => (
  <div className="flex items-center px-4 bg-surface-1 hover:bg-surface-hover border-b border-surface-3 group" onContextMenu={onContextMenu}>
    <button type="button" onClick={onOpen} aria-label={`Open ${name}`} className={`${columns} flex-1 min-w-0 text-left py-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand rounded`}>
      <span className="text-sm text-text-subtle font-mono">{index + 1}</span>
      <span className="flex items-center gap-3 min-w-0">
        <span className="w-10 h-10 shrink-0 overflow-hidden rounded bg-surface-3">{artwork}</span>
        <span className="min-w-0"><span className="block truncate font-medium text-text-main">{name}</span><span className="block md:hidden truncate text-xs text-text-secondary">{detail}</span></span>
      </span>
      <span className="hidden md:block truncate text-sm text-text-secondary">{detail}</span>
      <span className="text-right text-sm text-text-secondary">{stats}</span>
    </button>
    <div className="w-24 shrink-0 flex items-center justify-end gap-2">{actions}</div>
  </div>
);
