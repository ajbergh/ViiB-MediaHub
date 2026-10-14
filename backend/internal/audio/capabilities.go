// Defines the canonical per-format and per-source media capability matrix and
// reusable capability error classifications for ViiB MediaHub.
package audio

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// CapabilitySupportState describes how a given media operation is supported for
// a format or source.
type CapabilitySupportState string

const (
	// CapabilitySupported indicates first-class backend/native support.
	CapabilitySupported CapabilitySupportState = "supported"
	// CapabilityRuntimeDependent indicates support depends on the client
	// browser/WebView media decoder at runtime.
	CapabilityRuntimeDependent CapabilitySupportState = "runtime_dependent"
	// CapabilityClientFallback indicates the backend intentionally defers the
	// operation (such as DJ waveform generation) to the client Web Audio fallback.
	CapabilityClientFallback CapabilitySupportState = "client_fallback"
	// CapabilityUnsupported indicates the operation is not supported for this
	// format in the current runtime.
	CapabilityUnsupported CapabilitySupportState = "unsupported"
)

// CapabilityErrorCode identifies reusable machine-readable media capability
// failures across API and UI boundaries.
type CapabilityErrorCode string

const (
	CapabilityErrorUnsupportedFormat      CapabilityErrorCode = "unsupported_media_format"
	CapabilityErrorBrowserImportExcluded  CapabilityErrorCode = "browser_import_format_unsupported"
	CapabilityErrorBackendDecodeRequired  CapabilityErrorCode = "backend_decoder_unavailable"
	CapabilityErrorClientWaveformFallback CapabilityErrorCode = "client_waveform_fallback_required"
	CapabilityErrorAnalysisUnsupported    CapabilityErrorCode = "track_analysis_unsupported_format"
	CapabilityErrorPlaybackDecoderFailed  CapabilityErrorCode = "playback_decoder_unavailable"
)

// SourceFormatCapability describes support for one format within a specific
// catalog/media source (local filesystem, browser-only import, Plex, Spotify).
type SourceFormatCapability struct {
	Ingestion        CapabilitySupportState `json:"ingestion"`
	Metadata         CapabilitySupportState `json:"metadata"`
	Playback         CapabilitySupportState `json:"playback"`
	BackendWaveform  CapabilitySupportState `json:"backendWaveform"`
	ClientWaveform   CapabilitySupportState `json:"clientWaveform"`
	BackendAnalysis  CapabilitySupportState `json:"backendAnalysis"`
	FallbackBehavior string                 `json:"fallbackBehavior,omitempty"`
}

// FormatCapability describes the canonical support contract for one audio format.
type FormatCapability struct {
	ID                     string                            `json:"id"`
	Label                  string                            `json:"label"`
	Extensions             []string                          `json:"extensions"`
	MIMETypes              []string                          `json:"mimeTypes"`
	ScannerSupported       bool                              `json:"scannerSupported"`
	BrowserImportSupported bool                              `json:"browserImportSupported"`
	MetadataExtractor      string                            `json:"metadataExtractor"`
	BackendDecoderID       string                            `json:"backendDecoderId,omitempty"`
	BackendWaveform        CapabilitySupportState            `json:"backendWaveform"`
	ClientWaveform         CapabilitySupportState            `json:"clientWaveform"`
	BackendAnalysis        CapabilitySupportState            `json:"backendAnalysis"`
	Playback               CapabilitySupportState            `json:"playback"`
	Sources                map[string]SourceFormatCapability `json:"sources"`
	Notes                  string                            `json:"notes,omitempty"`
}

// CapabilityError is a reusable structured error returned or surfaced when an
// operation is not supported for a media format or source.
type CapabilityError struct {
	Code              CapabilityErrorCode    `json:"code"`
	FormatID          string                 `json:"formatId,omitempty"`
	Extension         string                 `json:"extension,omitempty"`
	Operation         string                 `json:"operation"`
	SupportState      CapabilitySupportState `json:"supportState"`
	FallbackAvailable bool                   `json:"fallbackAvailable"`
	FallbackBehavior  string                 `json:"fallbackBehavior,omitempty"`
	Message           string                 `json:"message"`
}

func (e *CapabilityError) Error() string {
	return e.Message
}

// CapabilityMatrix is the top-level canonical capability inventory.
type CapabilityMatrix struct {
	Version string             `json:"version"`
	Formats []FormatCapability `json:"formats"`
}

const CapabilityMatrixVersion = "media-capabilities-v1"

//go:embed media-capabilities.json
var canonicalMatrixJSON []byte

var canonicalFormats = func() []FormatCapability {
	var matrix CapabilityMatrix
	if err := json.Unmarshal(canonicalMatrixJSON, &matrix); err != nil {
		panic(err)
	}
	if matrix.Version != CapabilityMatrixVersion {
		panic("media capability matrix version mismatch")
	}
	return matrix.Formats
}()

// DefaultCapabilityMatrix returns a deep copy of the canonical media capability matrix.
func DefaultCapabilityMatrix() CapabilityMatrix {
	formats := make([]FormatCapability, len(canonicalFormats))
	for i, f := range canonicalFormats {
		exts := append([]string(nil), f.Extensions...)
		mimes := append([]string(nil), f.MIMETypes...)
		sources := make(map[string]SourceFormatCapability, len(f.Sources))
		for k, v := range f.Sources {
			sources[k] = v
		}
		f.Extensions = exts
		f.MIMETypes = mimes
		f.Sources = sources
		formats[i] = f
	}
	return CapabilityMatrix{
		Version: CapabilityMatrixVersion,
		Formats: formats,
	}
}

// NormalizeExtension returns a lowercase leading-dot file extension, or empty
// string if none is present.
func NormalizeExtension(nameOrExt string) string {
	trimmed := strings.TrimSpace(strings.ToLower(nameOrExt))
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, ".") && !strings.Contains(trimmed[1:], ".") && !strings.ContainsAny(trimmed, `/\`) {
		return trimmed
	}
	ext := filepath.Ext(trimmed)
	return ext
}

// LookupFormatCapability finds the canonical FormatCapability for a filename or
// extension (e.g. "song.flac" or ".flac").
func LookupFormatCapability(nameOrExt string) (FormatCapability, bool) {
	ext := NormalizeExtension(nameOrExt)
	if ext == "" {
		return FormatCapability{}, false
	}
	for _, f := range canonicalFormats {
		for _, candidate := range f.Extensions {
			if candidate == ext {
				return copyFormatCapability(f), true
			}
		}
	}
	return FormatCapability{}, false
}

func copyFormatCapability(f FormatCapability) FormatCapability {
	f.Extensions = append([]string(nil), f.Extensions...)
	f.MIMETypes = append([]string(nil), f.MIMETypes...)
	sources := make(map[string]SourceFormatCapability, len(f.Sources))
	for k, v := range f.Sources {
		sources[k] = v
	}
	f.Sources = sources
	return f
}

// ScannerSupportedExtensions returns the map of extensions accepted by the
// native filesystem scanner, derived from the canonical capability matrix.
func ScannerSupportedExtensions() map[string]bool {
	out := make(map[string]bool)
	for _, f := range canonicalFormats {
		if !f.ScannerSupported {
			continue
		}
		for _, ext := range f.Extensions {
			out[ext] = true
		}
	}
	return out
}

// NewWaveformCapabilityError builds a structured CapabilityError for a format
// that cannot be decoded by the backend waveform generator.
func NewWaveformCapabilityError(nameOrExt string) *CapabilityError {
	ext := NormalizeExtension(nameOrExt)
	capInfo, ok := LookupFormatCapability(ext)
	extLabel := strings.TrimPrefix(ext, ".")
	if extLabel == "" {
		extLabel = "unknown"
	}
	if ok && capInfo.BackendWaveform == CapabilityClientFallback {
		return &CapabilityError{
			Code:              CapabilityErrorClientWaveformFallback,
			FormatID:          capInfo.ID,
			Extension:         ext,
			Operation:         "backend_waveform",
			SupportState:      CapabilityClientFallback,
			FallbackAvailable: true,
			FallbackBehavior:  "Waveform will be generated by the client Web Audio decoder when supported by the runtime.",
			Message:           fmt.Sprintf("Waveform will be generated by the client for this audio format (%s)", extLabel),
		}
	}
	return &CapabilityError{
		Code:              CapabilityErrorBackendDecodeRequired,
		FormatID:          capInfo.ID,
		Extension:         ext,
		Operation:         "backend_waveform",
		SupportState:      CapabilityUnsupported,
		FallbackAvailable: false,
		Message:           fmt.Sprintf("Backend waveform generation is not supported for format %s", extLabel),
	}
}

// NewAnalysisCapabilityError builds a structured CapabilityError when backend
// track analysis (BPM/key/beatgrid/energy) is requested for an unsupported format.
func NewAnalysisCapabilityError(nameOrExt string) *CapabilityError {
	ext := NormalizeExtension(nameOrExt)
	capInfo, ok := LookupFormatCapability(ext)
	extLabel := strings.TrimPrefix(ext, ".")
	if extLabel == "" {
		extLabel = "unknown"
	}
	fallback := ""
	if ok {
		if src, hasLocal := capInfo.Sources["local"]; hasLocal {
			fallback = src.FallbackBehavior
		}
	}
	return &CapabilityError{
		Code:              CapabilityErrorAnalysisUnsupported,
		FormatID:          capInfo.ID,
		Extension:         ext,
		Operation:         "backend_analysis",
		SupportState:      CapabilityUnsupported,
		FallbackAvailable: ok && capInfo.ClientWaveform != CapabilityUnsupported,
		FallbackBehavior:  fallback,
		Message:           fmt.Sprintf("Backend track analysis (BPM, key, beatgrid, loudness) is not supported for %s files", extLabel),
	}
}
