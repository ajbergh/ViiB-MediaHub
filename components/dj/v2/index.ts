/**
 * ViiB MediaHub - DJ Components V2 Index
 * 
 * Export all v2 DJ mode components for the redesigned interface.
 * 
 * @module components/dj/v2
 */

export { DJTopBar } from './DJTopBar';
export { DJDualWaveform } from './DJDualWaveform';
export { DJJogWheel } from './DJJogWheel';
export { DJHotCuePad } from './DJHotCuePad';
export { DJTransportButtons } from './DJTransportButtons';
export { DJLoopSection } from './DJLoopSection';
export { DJEQKnob } from './DJEQKnob';
export { DJVolumeFader } from './DJVolumeFader';
export { DJCrossfader } from './DJCrossfader';
export { DJTempoSlider } from './DJTempoSlider';
export { DJNudgeButtons } from './DJNudgeButtons';
export { DJAudioSetup } from './DJAudioSetup';
export { DJBeatFXPanel } from './DJBeatFXPanel';
export { DJLibraryBrowserV2 } from './DJLibraryBrowserV2';
export { DeckTimeDisplay, DeckHasTrack, DeckBpmBadge } from './DJDeckComponents';
export { DJChannelStrip, DJMasterKnob, DJCrossfaderSelfSub, DJTempoSliderSelfSub } from './DJMixerComponents';

// WebGL high-performance waveform (Phase 2)
export { DJWebGLWaveform } from './webgl';
export type { DJWaveformRenderState, DJWebGLRendererOptions } from './webgl';
