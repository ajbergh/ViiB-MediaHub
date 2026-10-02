import React, { useLayoutEffect, useRef, useState, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { Check } from 'lucide-react';
import { Menu, MenuItem } from './ui/Menu';
import { DEFAULT_SONG_COLUMNS, SONG_COLUMNS, SongColumnId } from '../lib/songColumns';

export const SongColumnMenu: React.FC<{
  position: { x: number; y: number };
  columns: SongColumnId[];
  onChange: (columns: SongColumnId[]) => void;
  onClose: () => void;
}> = ({ position, columns, onChange, onClose }) => {
  const panel = useRef<HTMLDivElement>(null);
  const [location, setLocation] = useState(position);
  useLayoutEffect(() => {
    const rect = panel.current?.getBoundingClientRect();
    if (rect) setLocation({ x: Math.max(8, Math.min(position.x, window.innerWidth - rect.width - 8)), y: Math.max(8, Math.min(position.y, window.innerHeight - rect.height - 8)) });
  }, [position]);
  useEffect(() => {
    const closeOnScroll = (event: Event) => { if (!(event.target instanceof Node) || !panel.current?.contains(event.target)) onClose(); };
    window.addEventListener('resize', onClose);
    window.addEventListener('scroll', closeOnScroll, true);
    return () => { window.removeEventListener('resize', onClose); window.removeEventListener('scroll', closeOnScroll, true); };
  }, [onClose]);
  return createPortal(<>
    <button type="button" tabIndex={-1} aria-label="Close column chooser" className="fixed inset-0 z-[9998] cursor-default" onClick={onClose} onContextMenu={event => { event.preventDefault(); onClose(); }} />
    <div ref={panel} className="fixed z-[9999] w-56 max-w-[calc(100vw-16px)]" style={{ left: location.x, top: location.y }}>
      <Menu aria-label="Track columns" onRequestClose={onClose} className="max-h-[calc(100vh-16px)] overflow-y-auto">
        <p className="px-4 py-2 text-xs font-semibold text-text-secondary">Show columns</p>
        <MenuItem disabled>Title (always shown)</MenuItem>
        {SONG_COLUMNS.map(column => <MenuItem key={column.id} role="menuitemcheckbox" aria-checked={columns.includes(column.id)} onClick={() => onChange(columns.includes(column.id) ? columns.filter(id => id !== column.id) : [...columns, column.id])}>
          <span className="flex items-center gap-3"><span className="w-4">{columns.includes(column.id) && <Check size={14} />}</span>{column.label}</span>
        </MenuItem>)}
        <MenuItem onClick={() => onChange([...DEFAULT_SONG_COLUMNS])}>Reset to default</MenuItem>
      </Menu>
    </div>
  </>, document.body);
};
