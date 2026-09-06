package analysisbench

// CodecCapability records the current evidence and Phase 0 decision state. It
// intentionally separates "available now" from a production-support claim.
type CodecCapability struct {
	Format       string   `json:"format"`
	Extensions   []string `json:"extensions"`
	Path         string   `json:"path"`
	LicenseClass string   `json:"licenseClass"`
	Phase0State  string   `json:"phase0State"`
	Notes        string   `json:"notes"`
}

// CodecMatrix is the machine-readable counterpart of roadmap §7.2. Update it
// alongside the roadmap whenever a decoder spike gains new evidence.
func CodecMatrix() []CodecCapability {
	return []CodecCapability{
		{Format: "MP3", Extensions: []string{".mp3"}, Path: "existing go-mp3", LicenseClass: "Apache-2.0 / pure Go", Phase0State: "ready-to-benchmark", Notes: "Existing waveform decoder."},
		{Format: "Ogg Vorbis", Extensions: []string{".ogg", ".oga"}, Path: "existing oggvorbis", LicenseClass: "MIT / pure Go", Phase0State: "ready-to-benchmark", Notes: "Existing conversion decoder; Opus is excluded."},
		{Format: "WAV PCM/float", Extensions: []string{".wav", ".wave"}, Path: "Phase 0 header validator", LicenseClass: "in-house / pure Go", Phase0State: "header-validation-ready", Notes: "Streaming PCM decoder remains a Phase 1 decision."},
		{Format: "FLAC", Extensions: []string{".flac"}, Path: "mewkiz/flac candidate", LicenseClass: "Unlicense / pure Go", Phase0State: "candidate", Notes: "No dependency added until malformed-input and throughput checks pass."},
		{Format: "AAC ADTS", Extensions: []string{".aac"}, Path: "pure-Go or WASM candidate", LicenseClass: "LGPL or artifact-specific", Phase0State: "legal-and-throughput-gated", Notes: "Do not treat technical decoding as shipping approval."},
		{Format: "M4A/AAC", Extensions: []string{".m4a"}, Path: "MP4 demux plus AAC candidate", LicenseClass: "mixed", Phase0State: "legal-and-throughput-gated", Notes: "Container and codec decisions are independent."},
		{Format: "Opus in Ogg", Extensions: []string{".opus"}, Path: "WASM or pure-Go candidate", LicenseClass: "artifact-specific", Phase0State: "unsupported-pending-spike", Notes: "Never route this through the Vorbis decoder."},
		{Format: "AIFF/AIFC", Extensions: []string{".aiff", ".aif"}, Path: "in-house candidate", LicenseClass: "in-house / pure Go", Phase0State: "blocked-by-ingestion-decision", Notes: "Scanner does not ingest these extensions yet."},
		{Format: "WMA", Extensions: []string{".wma"}, Path: "none", LicenseClass: "not established", Phase0State: "unsupported", Notes: "Explicitly unsupported for initial analysis."},
	}
}
