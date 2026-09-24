package api

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/ajbergh/viib-mediahub/internal/analysis"
	"github.com/ajbergh/viib-mediahub/internal/db"
	"github.com/ajbergh/viib-mediahub/internal/stems"
)

var stemSourceAudioHashes = analysis.NewSourceAudioHashCache()

// stemSourceMatches accepts the exact full-file identity first. A retagged
// source may fall back to the contract PCM32 identity only when the package
// supplies one and its exact geometry is supported by MediaHub's decoder.
func stemSourceMatches(song *db.Song, manifest stems.Manifest) bool {
	fullHash, err := stemSourceHashes.SHA256(song.FilePath)
	if err == nil && strings.EqualFold(fullHash, manifest.Source.SHA256) {
		return true
	}
	return stemAudioIdentityMatches(song, manifest)
}

func stemAudioIdentityMatches(song *db.Song, manifest stems.Manifest) bool {
	if manifest.Source.AudioSHA256 == "" {
		return false
	}
	pcmHash, err := stemSourceAudioHashes.SHA256(context.Background(), decoderRegistry(), analysis.ResolvedSource{Name: filepath.Base(song.FilePath), Path: song.FilePath}, manifest.Audio.SampleRate, manifest.Audio.Channels)
	return err == nil && strings.EqualFold(pcmHash, manifest.Source.AudioSHA256)
}
