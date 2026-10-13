// @vitest-environment jsdom
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect,it,vi } from 'vitest';
const mocks=vi.hoisted(()=>({metadata:vi.fn(),catalog:vi.fn()}));
vi.mock('../services/api',()=>({api:{getTrackAnalysisFeature:mocks.metadata}}));
vi.mock('../services/importedCatalog',async importOriginal=>({...await importOriginal<typeof import('../services/importedCatalog')>(),loadImportedCatalog:mocks.catalog}));
vi.mock('../store',()=>({useStore:(select:any)=>select({spotifySessionGeneration:0})}));
import { SongAudioMetadata } from './SongAudioMetadata';
it('keeps catalog inspection available when ordinary metadata fails',async()=>{
 (globalThis as any).IS_REACT_ACT_ENVIRONMENT=true;
 mocks.metadata.mockRejectedValue(new Error('temporary metadata failure'));
 mocks.catalog.mockResolvedValue({recordingId:'A'.repeat(22),sourceFingerprint:'fp',provenance:'spotify_download_import',snapshots:[],relations:[],origins:[]});
 const host=document.createElement('div'),root=createRoot(host);
 try{await act(async()=>root.render(<SongAudioMetadata songId="song"/>));
 expect(host.textContent).toContain('Audio metadata could not be loaded');expect(mocks.catalog).not.toHaveBeenCalled();
 await act(async()=>{const details=host.querySelector('details')!;details.open=true;details.dispatchEvent(new Event('toggle'));});
 expect(host.textContent).toContain('Spotify recording');expect(host.textContent).toContain('Audio metadata could not be loaded');expect(mocks.metadata).toHaveBeenCalledTimes(1);
 }finally{await act(async()=>root.unmount());}
});
