/**
 * Canonical per-format and per-source media capability matrix and reusable
 * capability error helpers for ViiB MediaHub.
 *
 * This module mirrors backend/internal/audio/capabilities.go so UI components,
 * browser-only folder import, playback handlers, and DJ waveform/analysis flows
 * use one factual source of truth rather than inferring support from filenames.
 */

export type CapabilitySupportState =
  | 'supported'
  | 'runtime_dependent'
  | 'client_fallback'
  | 'unsupported';

export type CapabilityErrorCode =
  | 'unsupported_media_format'
  | 'browser_import_format_unsupported'
  | 'backend_decoder_unavailable'
  | 'client_waveform_fallback_required'
  | 'track_analysis_unsupported_format'
  | 'playback_decoder_unavailable';

export interface SourceFormatCapability {
  ingestion: CapabilitySupportState;
  metadata: CapabilitySupportState;
  playback: CapabilitySupportState;
  backendWaveform: CapabilitySupportState;
  clientWaveform: CapabilitySupportState;
  backendAnalysis: CapabilitySupportState;
  fallbackBehavior?: string;
}

export interface FormatCapability {
  id: string;
  label: string;
  extensions: string[];
  mimeTypes: string[];
  scannerSupported: boolean;
  browserImportSupported: boolean;
  metadataExtractor: string;
  backendDecoderId?: string;
  backendWaveform: CapabilitySupportState;
  clientWaveform: CapabilitySupportState;
  backendAnalysis: CapabilitySupportState;
  playback: CapabilitySupportState;
  sources: Record<string, SourceFormatCapability>;
  notes?: string;
}

export interface CapabilityErrorPayload {
  code: CapabilityErrorCode;
  formatId?: string;
  extension?: string;
  operation: string;
  supportState: CapabilitySupportState;
  fallbackAvailable: boolean;
  fallbackBehavior?: string;
  message: string;
}

export interface CapabilityMatrix {
  version: string;
  formats: FormatCapability[];
}

export const CAPABILITY_MATRIX_VERSION = 'media-capabilities-v1';

export const CANONICAL_MEDIA_CAPABILITIES: CapabilityMatrix = {
  version: CAPABILITY_MATRIX_VERSION,
  formats: [
    {
      id: 'mp3',
      label: 'MP3 (MPEG-1/2 Audio Layer III)',
      extensions: ['.mp3'],
      mimeTypes: ['audio/mpeg', 'audio/mp3'],
      scannerSupported: true,
      browserImportSupported: true,
      metadataExtractor: 'taglib+dhowden',
      backendDecoderId: 'go-mp3-v0.3.4',
      backendWaveform: 'supported',
      clientWaveform: 'supported',
      backendAnalysis: 'supported',
      playback: 'supported',
      sources: {
        local: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'supported',
          backendWaveform: 'supported',
          clientWaveform: 'supported',
          backendAnalysis: 'supported',
        },
        browser: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'supported',
          backendWaveform: 'unsupported',
          clientWaveform: 'supported',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'Browser-only mode uses Web Audio client waveform and metadata.ts extraction.',
        },
        plex: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'supported',
          backendWaveform: 'supported',
          clientWaveform: 'supported',
          backendAnalysis: 'supported',
        },
      },
      notes: 'Full end-to-end support across native scan, browser import, playback, backend DJ waveform, and track analysis.',
    },
    {
      id: 'ogg_vorbis',
      label: 'OGG Vorbis',
      extensions: ['.ogg', '.oga'],
      mimeTypes: ['audio/ogg', 'application/ogg'],
      scannerSupported: true,
      browserImportSupported: true,
      metadataExtractor: 'taglib+dhowden',
      backendDecoderId: 'oggvorbis-v1.0.5',
      backendWaveform: 'supported',
      clientWaveform: 'runtime_dependent',
      backendAnalysis: 'supported',
      playback: 'runtime_dependent',
      sources: {
        local: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'supported',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'supported',
        },
        browser: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'unsupported',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'unsupported',
        },
        plex: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'supported',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'supported',
        },
        spotify: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'supported',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'supported',
          fallbackBehavior: 'Spotify downloads can optionally convert OGG Vorbis to MP3 for broader playback compatibility.',
        },
      },
      notes: 'Backend analysis and DJ waveform decode OGG Vorbis (.ogg, .oga). OGG Opus in an .ogg container is rejected by the Vorbis decoder.',
    },
    {
      id: 'wav',
      label: 'WAV (RIFF PCM16 / Float32)',
      extensions: ['.wav', '.wave'],
      mimeTypes: ['audio/wav', 'audio/x-wav', 'audio/wave'],
      scannerSupported: true,
      browserImportSupported: false,
      metadataExtractor: 'taglib+filename',
      backendDecoderId: 'wav-pcm16-v1',
      backendWaveform: 'supported',
      clientWaveform: 'supported',
      backendAnalysis: 'supported',
      playback: 'supported',
      sources: {
        local: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'supported',
          backendWaveform: 'supported',
          clientWaveform: 'supported',
          backendAnalysis: 'supported',
        },
        browser: {
          ingestion: 'unsupported',
          metadata: 'unsupported',
          playback: 'supported',
          backendWaveform: 'unsupported',
          clientWaveform: 'supported',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'Browser-only folder import does not currently admit .wav/.wave.',
        },
        plex: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'supported',
          backendWaveform: 'supported',
          clientWaveform: 'supported',
          backendAnalysis: 'supported',
        },
      },
      notes: 'Backend decoder supports RIFF/WAVE signed 16-bit PCM and 32-bit IEEE float.',
    },
    {
      id: 'flac',
      label: 'FLAC (Free Lossless Audio Codec)',
      extensions: ['.flac'],
      mimeTypes: ['audio/flac', 'audio/x-flac'],
      scannerSupported: true,
      browserImportSupported: false,
      metadataExtractor: 'taglib+dhowden',
      backendWaveform: 'client_fallback',
      clientWaveform: 'runtime_dependent',
      backendAnalysis: 'unsupported',
      playback: 'runtime_dependent',
      sources: {
        local: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'client_fallback',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'DJ waveform falls back to client Web Audio decode; backend BPM/key/beatgrid analysis is explicitly unsupported.',
        },
        browser: {
          ingestion: 'unsupported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'unsupported',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'Browser-only folder import currently filters to .mp3 and .ogg (tracked in P1.2).',
        },
        plex: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'client_fallback',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'Direct-play stream; DJ waveform uses client Web Audio fallback and backend analysis marks unsupported_codec.',
        },
      },
      notes: 'Accepted by native scanner and TagLib/dhowden metadata extraction. No backend PCM decoder is registered yet; DJ waveform defers to client Web Audio (HTTP 422) and backend track analysis records unsupported_codec.',
    },
    {
      id: 'm4a_aac',
      label: 'M4A / AAC (MP4 Audio)',
      extensions: ['.m4a', '.aac'],
      mimeTypes: ['audio/mp4', 'audio/x-m4a', 'audio/aac'],
      scannerSupported: true,
      browserImportSupported: false,
      metadataExtractor: 'taglib+dhowden',
      backendWaveform: 'client_fallback',
      clientWaveform: 'runtime_dependent',
      backendAnalysis: 'unsupported',
      playback: 'runtime_dependent',
      sources: {
        local: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'client_fallback',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'DJ waveform falls back to client Web Audio decode; backend BPM/key/beatgrid analysis is explicitly unsupported.',
        },
        browser: {
          ingestion: 'unsupported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'unsupported',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'Browser-only folder import currently filters to .mp3 and .ogg (tracked in P1.2).',
        },
        plex: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'client_fallback',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'Direct-play stream; DJ waveform uses client Web Audio fallback and backend analysis marks unsupported_codec.',
        },
      },
      notes: 'M4A tags are extracted via TagLib/dhowden; raw ADTS .aac files fall back to TagLib or filename metadata. Backend DSP analysis is unsupported.',
    },
    {
      id: 'opus',
      label: 'Opus (.opus)',
      extensions: ['.opus'],
      mimeTypes: ['audio/opus', 'audio/ogg; codecs=opus'],
      scannerSupported: true,
      browserImportSupported: false,
      metadataExtractor: 'taglib+dhowden',
      backendWaveform: 'client_fallback',
      clientWaveform: 'runtime_dependent',
      backendAnalysis: 'unsupported',
      playback: 'runtime_dependent',
      sources: {
        local: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'client_fallback',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'DJ waveform falls back to client Web Audio decode; backend BPM/key/beatgrid analysis is explicitly unsupported.',
        },
        browser: {
          ingestion: 'unsupported',
          metadata: 'unsupported',
          playback: 'runtime_dependent',
          backendWaveform: 'unsupported',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'Browser-only folder import does not admit .opus.',
        },
        plex: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'client_fallback',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'Direct-play stream; DJ waveform uses client Web Audio fallback and backend analysis marks unsupported_codec.',
        },
      },
      notes: 'Accepted by native scanner; not registered in the backend Vorbis/PCM analysis decoder registry.',
    },
    {
      id: 'wma',
      label: 'Windows Media Audio (.wma)',
      extensions: ['.wma'],
      mimeTypes: ['audio/x-ms-wma'],
      scannerSupported: true,
      browserImportSupported: false,
      metadataExtractor: 'taglib+filename',
      backendWaveform: 'client_fallback',
      clientWaveform: 'runtime_dependent',
      backendAnalysis: 'unsupported',
      playback: 'runtime_dependent',
      sources: {
        local: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'client_fallback',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'Legacy scanner extension only; most WebView/browser runtimes cannot decode WMA natively.',
        },
        browser: {
          ingestion: 'unsupported',
          metadata: 'unsupported',
          playback: 'unsupported',
          backendWaveform: 'unsupported',
          clientWaveform: 'unsupported',
          backendAnalysis: 'unsupported',
        },
        plex: {
          ingestion: 'supported',
          metadata: 'supported',
          playback: 'runtime_dependent',
          backendWaveform: 'client_fallback',
          clientWaveform: 'runtime_dependent',
          backendAnalysis: 'unsupported',
          fallbackBehavior: 'Direct-play depends on client decoder support; transcoding is not enabled.',
        },
      },
      notes: 'Legacy Windows format accepted by scanner; no backend DSP decoder and limited browser/WebView playback support.',
    },
  ],
};

export function normalizeMediaExtension(nameOrExt: string): string {
  const trimmed = nameOrExt.trim().toLowerCase();
  if (!trimmed) return '';
  if (trimmed.startsWith('.') && !trimmed.slice(1).includes('.') && !/[/\\]/.test(trimmed)) {
    return trimmed;
  }
  const cleanPath = trimmed.split('?')[0].split('#')[0];
  const lastDot = cleanPath.lastIndexOf('.');
  const lastSep = Math.max(cleanPath.lastIndexOf('/'), cleanPath.lastIndexOf('\\'));
  if (lastDot <= lastSep) return '';
  return cleanPath.slice(lastDot);
}

export function lookupFormatCapability(nameOrExt: string): FormatCapability | null {
  const ext = normalizeMediaExtension(nameOrExt);
  if (!ext) return null;
  return CANONICAL_MEDIA_CAPABILITIES.formats.find(f => f.extensions.includes(ext)) ?? null;
}

export function createMediaCapabilityError(
  operation: 'browser_import' | 'backend_waveform' | 'backend_analysis' | 'playback',
  nameOrExt: string,
): CapabilityErrorPayload {
  const ext = normalizeMediaExtension(nameOrExt);
  const cap = lookupFormatCapability(ext);
  const extLabel = ext ? ext.slice(1).toUpperCase() : 'UNKNOWN';

  if (operation === 'browser_import') {
    return {
      code: 'browser_import_format_unsupported',
      formatId: cap?.id,
      extension: ext,
      operation,
      supportState: cap?.browserImportSupported ? 'supported' : 'unsupported',
      fallbackAvailable: Boolean(cap?.scannerSupported),
      fallbackBehavior: cap?.scannerSupported
        ? 'Use the desktop/server folder scanner to index this format.'
        : undefined,
      message: `${extLabel} files are not supported by browser-only folder import.`,
    };
  }

  if (operation === 'backend_waveform') {
    const clientFallback = cap?.backendWaveform === 'client_fallback';
    return {
      code: clientFallback ? 'client_waveform_fallback_required' : 'backend_decoder_unavailable',
      formatId: cap?.id,
      extension: ext,
      operation,
      supportState: cap?.backendWaveform ?? 'unsupported',
      fallbackAvailable: clientFallback,
      fallbackBehavior: clientFallback
        ? 'Waveform will be generated by the client Web Audio decoder when supported by the runtime.'
        : undefined,
      message: clientFallback
        ? `Waveform will be generated by the client for ${extLabel} files.`
        : `Backend waveform generation is not supported for ${extLabel} files.`,
    };
  }

  if (operation === 'backend_analysis') {
    return {
      code: 'track_analysis_unsupported_format',
      formatId: cap?.id,
      extension: ext,
      operation,
      supportState: cap?.backendAnalysis ?? 'unsupported',
      fallbackAvailable: Boolean(cap && cap.clientWaveform !== 'unsupported'),
      fallbackBehavior: cap?.sources.local?.fallbackBehavior,
      message: `Backend track analysis (BPM, key, beatgrid, loudness) is not supported for ${extLabel} files.`,
    };
  }

  return {
    code: 'playback_decoder_unavailable',
    formatId: cap?.id,
    extension: ext,
    operation,
    supportState: cap?.playback ?? 'unsupported',
    fallbackAvailable: false,
    message: `This browser/WebView runtime could not decode ${extLabel} audio.`,
  };
}
