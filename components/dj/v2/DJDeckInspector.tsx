/**
 * ViiB MediaHub - DJ Deck Inspector (v2)
 *
 * Anchored popover that holds the editing and analysis tools which used to
 * consume permanent deck height: energy insights, key verification, BPM
 * correction, cue editing and beat-grid editing (Plan §6.3).
 *
 * Panels stay mounted while the inspector is closed so analysis widgets keep
 * their subscriptions and in-flight work, and opening it never remounts the
 * jog or waveform. Escape and outside clicks close it.
 *
 * @module components/dj/v2/DJDeckInspector
 */

import React, { useEffect, useId, useRef } from 'react';
import { Disc3, X } from 'lucide-react';
import { useStore } from '../../../store';
import type { DeckId } from '../../../slices/djMixerSlice';
import { DJErrorBoundary } from './DJErrorBoundary';
import { DJEnergyInsights } from './DJEnergyInsights';
import { DJKeyVerificationKeyboard } from './DJKeyVerificationKeyboard';
import { DJBpmEditor } from './DJBpmEditor';
import { DJAnalysisCueEditor } from './DJAnalysisCueEditor';
import { DJBeatGridEdit } from './DJBeatGridEdit';
import { DJBeatGridStatus } from './DJBeatGridStatus';
import { SongAudioMetadata } from '../../SongAudioMetadata';

export type DJDeckInspectorTab = 'analysis' | 'audio' | 'key' | 'bpm' | 'cues' | 'grid';

const TABS: ReadonlyArray<{ id: DJDeckInspectorTab; label: string }> = [
  { id: 'analysis', label: 'Analysis' },
  { id: 'audio', label: 'Audio' },
  { id: 'key', label: 'Key' },
  { id: 'bpm', label: 'BPM' },
  { id: 'cues', label: 'Cues' },
  { id: 'grid', label: 'Grid' },
];

interface DJDeckInspectorProps {
  deck: DeckId;
  open: boolean;
  tab: DJDeckInspectorTab;
  onTabChange: (tab: DJDeckInspectorTab) => void;
  onClose: () => void;
  /** Trigger element; focus returns here on close and clicks on it are not "outside". */
  triggerRef: React.RefObject<HTMLElement | null>;
}

export const DJDeckInspector = React.memo(function DJDeckInspector({ deck, open, tab, onTabChange, onClose, triggerRef }: DJDeckInspectorProps) {
  const track = useStore(s => (deck === 'A' ? s.djDeckA : s.djDeckB).track);
  const hasGrid = useStore(s => Boolean((deck === 'A' ? s.djDeckA : s.djDeckB).beatGrid?.length));
  const rootRef = useRef<HTMLElement>(null);
  const baseId = useId().replace(/:/g, '');

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (rootRef.current?.contains(target) || triggerRef.current?.contains(target)) return;
      // The library drawer overlays decks; interacting with it should not
      // silently discard an open inspector's context.
      if ((target as Element).closest?.('#dj-library-drawer')) return;
      onClose();
    };
    document.addEventListener('pointerdown', onPointerDown, true);
    return () => document.removeEventListener('pointerdown', onPointerDown, true);
  }, [open, onClose, triggerRef]);

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key !== 'Escape') return;
    event.stopPropagation();
    onClose();
    triggerRef.current?.focus({ preventScroll: true });
  };

  const moveTab = (event: React.KeyboardEvent, index: number) => {
    if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') return;
    event.preventDefault();
    const next = TABS[(index + (event.key === 'ArrowRight' ? 1 : TABS.length - 1)) % TABS.length];
    onTabChange(next.id);
    document.getElementById(`${baseId}-tab-${next.id}`)?.focus();
  };

  const emptyNotice = (hint: string) => (
    <div className='dj-inspector-empty' role='status'>
      <Disc3 size={24} className='opacity-60' aria-hidden='true' />
      <p className='font-semibold text-[var(--dj-text-primary)]'>No track loaded on Deck {deck}</p>
      <p className='text-xs text-[var(--dj-text-secondary)] max-w-xs'>{hint}</p>
    </div>
  );

  return (
    <section
      ref={rootRef}
      className='dj-deck-inspector'
      data-dj-deck-inspector={deck}
      aria-label={`Deck ${deck} analysis and editing`}
      hidden={!open}
      aria-hidden={!open}
      onKeyDown={handleKeyDown}
    >
      <div className='dj-deck-inspector-tabs' role='tablist' aria-label={`Deck ${deck} inspector sections`}>
        {TABS.map(({ id, label }, index) => (
          <button key={id} id={`${baseId}-tab-${id}`} type='button' role='tab' className='dj-btn dj-btn-xs' data-deck-accent={deck}
            aria-selected={tab === id} aria-pressed={tab === id} aria-controls={`${baseId}-panel-${id}`} tabIndex={tab === id ? 0 : -1}
            onClick={() => onTabChange(id)} onKeyDown={event => moveTab(event, index)}>{label}</button>
        ))}
        <button type='button' className='dj-btn dj-btn-xs dj-btn-icon dj-btn-ghost' style={{ marginLeft: 'auto' }}
          aria-label={`Close Deck ${deck} inspector`} onClick={() => { onClose(); triggerRef.current?.focus({ preventScroll: true }); }}>
          <X size={14} aria-hidden='true' />
        </button>
      </div>
      <div className='dj-deck-inspector-body'>
        <DJErrorBoundary componentName={`Deck ${deck} inspector`}>
          <div role='tabpanel' id={`${baseId}-panel-analysis`} aria-labelledby={`${baseId}-tab-analysis`} hidden={tab !== 'analysis'} aria-hidden={tab !== 'analysis'}>
            {/* Always mounted, as before the inspector existed; hidden without a track
                because its empty-deck state reads as "not analysed". */}
            <div hidden={!track} aria-hidden={!track}><DJEnergyInsights trackID={track?.id} deck={deck} /></div>
            {!track && emptyNotice('Load a track from the Library drawer to view energy insights and Mix Next recommendations.')}
          </div>
          <div role='tabpanel' id={`${baseId}-panel-audio`} aria-labelledby={`${baseId}-tab-audio`} hidden={tab !== 'audio'} aria-hidden={tab !== 'audio'}>
            {track ? <>
              <p className='px-3 py-2 text-xs text-[var(--dj-text-muted)]'>Compare local and Spotify band overviews and inspect metadata for this deck. Overview duration compatibility does not establish beat alignment; use the deck waveform and Grid tools for timing.</p>
              {open && tab === 'audio' && <SongAudioMetadata key={track.id} songId={track.id} />}
            </> : emptyNotice('Load a track from the Library drawer to inspect its waveforms and audio metadata.')}
          </div>
          <div role='tabpanel' id={`${baseId}-panel-key`} aria-labelledby={`${baseId}-tab-key`} hidden={tab !== 'key'} aria-hidden={tab !== 'key'}>
            <DJKeyVerificationKeyboard trackID={track?.id} deck={deck} embedded />
            {!track && emptyNotice('Load a track from the Library drawer to verify its musical key.')}
          </div>
          <div role='tabpanel' id={`${baseId}-panel-bpm`} aria-labelledby={`${baseId}-tab-bpm`} hidden={tab !== 'bpm'} aria-hidden={tab !== 'bpm'}>
            <DJBpmEditor track={track} deck={deck} embedded />
            {!track && emptyNotice('Load a track from the Library drawer to inspect or correct its BPM.')}
          </div>
          <div role='tabpanel' id={`${baseId}-panel-cues`} aria-labelledby={`${baseId}-tab-cues`} hidden={tab !== 'cues'} aria-hidden={tab !== 'cues'}>
            <DJAnalysisCueEditor trackID={track?.id} deck={deck} embedded />
            {!track && emptyNotice('Load a track from the Library drawer to edit its hot cues.')}
          </div>
          <div role='tabpanel' id={`${baseId}-panel-grid`} aria-labelledby={`${baseId}-tab-grid`} hidden={tab !== 'grid'} aria-hidden={tab !== 'grid'}>
            {hasGrid ? (
              <div className='dj-grid-editor-panel'>
                <p className='dj-label' style={{ padding: '0 8px 6px' }}><DJBeatGridStatus deck={deck} /></p>
                <DJBeatGridEdit deck={deck} />
              </div>
            ) : emptyNotice(track ? 'This track has no beat grid yet.' : 'Load a track from the Library drawer to edit its beat grid.')}
          </div>
        </DJErrorBoundary>
      </div>
    </section>
  );
});
