/**
 * ViiB MediaHub - DJ Performance Workspace (v2)
 *
 * Layout-only shell (Plan §4.1): the waveform band above a 3-column
 * Deck A | Mixer | Deck B grid. Owns no audio logic or store state.
 * The mixer column's center matches the waveform's 50/50 divider because
 * both deck columns are equal `minmax(0, 1fr)` tracks.
 *
 * @module components/dj/v2/DJPerformanceWorkspace
 */

import React from 'react';

interface DJPerformanceWorkspaceProps {
  upper?: React.ReactNode;
  deckA: React.ReactNode;
  waveA?: React.ReactNode;
  mixer: React.ReactNode;
  waveB?: React.ReactNode;
  deckB: React.ReactNode;
}

export const DJPerformanceWorkspace: React.FC<DJPerformanceWorkspaceProps> = ({
  upper,
  deckA,
  waveA,
  mixer,
  waveB,
  deckB,
}) => (
  <>
    {upper ? <div className='dj-upper'>{upper}</div> : null}
    <div className='dj-main-grid' data-dj-workspace>
      {deckA}
      {waveA}
      {mixer}
      {waveB}
      {deckB}
    </div>
  </>
);
