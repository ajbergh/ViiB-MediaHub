/** Tests for the canonical frontend media capability matrix and error helpers. */

import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import {
  CANONICAL_MEDIA_CAPABILITIES,
  CAPABILITY_MATRIX_VERSION,
  createMediaCapabilityError,
  lookupFormatCapability,
  normalizeMediaExtension,
} from './mediaCapabilities';

describe('mediaCapabilities', () => {
  it('defines canonical capabilities for MP3, OGG Vorbis, WAV, FLAC, M4A/AAC, Opus, and WMA and matches backend audio/capabilities.go', () => {
    expect(CANONICAL_MEDIA_CAPABILITIES.version).toBe(CAPABILITY_MATRIX_VERSION);
    const ids = CANONICAL_MEDIA_CAPABILITIES.formats.map(f => f.id);
    expect(ids).toEqual(['mp3', 'ogg_vorbis', 'wav', 'flac', 'm4a_aac', 'opus', 'wma']);

    const goSource = fs.readFileSync(
      path.resolve(__dirname, '../backend/internal/audio/capabilities.go'),
      'utf8',
    );
    expect(goSource).toContain(`CapabilityMatrixVersion = "${CAPABILITY_MATRIX_VERSION}"`);
    for (const fmt of CANONICAL_MEDIA_CAPABILITIES.formats) {
      expect(goSource).toContain(`ID:                     "${fmt.id}"`);
      for (const ext of fmt.extensions) {
        expect(goSource).toContain(`"${ext}"`);
      }
      if (fmt.backendDecoderId) {
        expect(goSource).toContain(`BackendDecoderID:       "${fmt.backendDecoderId}"`);
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
    const waveformFallback = createMediaCapabilityError('backend_waveform', 'track.flac');
    expect(waveformFallback.code).toBe('client_waveform_fallback_required');
    expect(waveformFallback.fallbackAvailable).toBe(true);
    expect(waveformFallback.formatId).toBe('flac');

    const analysisUnsupported = createMediaCapabilityError('backend_analysis', 'track.m4a');
    expect(analysisUnsupported.code).toBe('track_analysis_unsupported_format');
    expect(analysisUnsupported.supportState).toBe('unsupported');
    expect(analysisUnsupported.formatId).toBe('m4a_aac');

    const browserImportFlac = createMediaCapabilityError('browser_import', 'track.flac');
    expect(browserImportFlac.code).toBe('browser_import_format_unsupported');
    expect(browserImportFlac.supportState).toBe('supported');

    const browserImportWav = createMediaCapabilityError('browser_import', 'track.wav');
    expect(browserImportWav.code).toBe('browser_import_format_unsupported');
    expect(browserImportWav.supportState).toBe('unsupported');
    expect(browserImportWav.fallbackAvailable).toBe(true);
  });
});
