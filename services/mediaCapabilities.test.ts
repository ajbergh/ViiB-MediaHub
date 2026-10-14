/** Tests for the canonical frontend media capability matrix and error helpers. */

import { describe, expect, it } from 'vitest';
import {
  CANONICAL_MEDIA_CAPABILITIES,
  CAPABILITY_MATRIX_VERSION,
  createMediaCapabilityError,
  lookupFormatCapability,
  normalizeMediaExtension,
  isBrowserImportSupportedFile,
} from './mediaCapabilities';

describe('mediaCapabilities', () => {
  it('defines canonical capabilities for MP3, OGG Vorbis, WAV, FLAC, M4A/AAC, Opus, and WMA with consistent browser import support', () => {
    expect(CANONICAL_MEDIA_CAPABILITIES.version).toBe(CAPABILITY_MATRIX_VERSION);
    const ids = CANONICAL_MEDIA_CAPABILITIES.formats.map(f => f.id);
    expect(ids).toEqual(['mp3', 'ogg_vorbis', 'wav', 'flac', 'm4a_aac', 'aac', 'opus', 'wma']);

    for (const format of CANONICAL_MEDIA_CAPABILITIES.formats) {
      expect(format.sources.browser.ingestion).toBe(format.browserImportSupported ? 'supported' : 'unsupported');
      for (const extension of format.extensions) {
        expect(isBrowserImportSupportedFile(`track${extension}`)).toBe(format.browserImportSupported);
      }
    }
  });

  it('normalizes extensions from filenames, paths, and bare extensions', () => {
    expect(normalizeMediaExtension('.FLAC')).toBe('.flac');
    expect(normalizeMediaExtension('/music/album/01 - Track.M4A?token=1')).toBe('.m4a');
    expect(normalizeMediaExtension('C:\\Music\\song.ogg')).toBe('.ogg');
    expect(normalizeMediaExtension('no-extension')).toBe('');
  });

  it('distinguishes backend-analyzed formats from client-fallback formats', () => {
    const mp3 = lookupFormatCapability('song.mp3');
    expect(mp3?.backendAnalysis).toBe('supported');
    expect(mp3?.backendWaveform).toBe('supported');

    const flac = lookupFormatCapability('song.flac');
    expect(flac?.scannerSupported).toBe(true);
    expect(flac?.backendAnalysis).toBe('unsupported');
    expect(flac?.backendWaveform).toBe('client_fallback');

    const m4a = lookupFormatCapability('song.m4a');
    expect(m4a?.scannerSupported).toBe(true);
    expect(m4a?.backendAnalysis).toBe('unsupported');
    expect(m4a?.backendWaveform).toBe('client_fallback');
  });

  it('produces structured capability errors with explicit fallback state', () => {
    const waveformFallback = createMediaCapabilityError('backend_waveform', 'track.flac')!;
    expect(waveformFallback.code).toBe('client_waveform_fallback_required');
    expect(waveformFallback.fallbackAvailable).toBe(true);
    expect(waveformFallback.formatId).toBe('flac');

    const analysisUnsupported = createMediaCapabilityError('backend_analysis', 'track.m4a')!;
    expect(analysisUnsupported.code).toBe('track_analysis_unsupported_format');
    expect(analysisUnsupported.supportState).toBe('unsupported');
    expect(analysisUnsupported.formatId).toBe('m4a_aac');

    const browserImportUnsupported = createMediaCapabilityError('browser_import', 'track.aac')!;
    expect(browserImportUnsupported.code).toBe('browser_import_format_unsupported');
    expect(browserImportUnsupported.fallbackAvailable).toBe(true);
  });
});

it('does not manufacture capability errors for supported operations', () => {
  for (const operation of ['browser_import', 'backend_waveform', 'backend_analysis'] as const) {
    expect(createMediaCapabilityError(operation, 'track.mp3')).toBeNull();
  }
});

it('admits FLAC and M4A without advertising raw AAC browser import', () => {
  for (const extension of ['.mp3', '.ogg', '.oga', '.flac', '.m4a']) {
    expect(isBrowserImportSupportedFile(`TRACK${extension.toUpperCase()}`)).toBe(true);
    expect(createMediaCapabilityError('browser_import', `track${extension}`)).toBeNull();
  }
  expect(isBrowserImportSupportedFile('track.aac')).toBe(false);
  expect(lookupFormatCapability('.aac')?.sources.browser.ingestion).toBe('unsupported');
});
