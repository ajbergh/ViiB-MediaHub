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
	matched, _ := stemSourceMatchesContext(context.Background(), song, manifest, "")
	return matched
}

func stemSourceMatchesContext(ctx context.Context, song *db.Song, manifest stems.Manifest, knownFullHash string) (bool, error) {
	fullHash := knownFullHash
	var err error
	if fullHash == "" {
		fullHash, err = stemSourceHashes.SHA256Context(ctx, song.FilePath)
	}
	if err == nil && strings.EqualFold(fullHash, manifest.Source.SHA256) {
		return true, nil
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return stemAudioIdentityMatchesContext(ctx, song, manifest)
}

func stemAudioIdentityMatches(song *db.Song, manifest stems.Manifest) bool {
	matched, _ := stemAudioIdentityMatchesContext(context.Background(), song, manifest)
	return matched
}

func stemAudioIdentityMatchesContext(ctx context.Context, song *db.Song, manifest stems.Manifest) (bool, error) {
	if manifest.Source.AudioSHA256 == "" {
		return false, nil
	}
	pcmHash, err := stemSourceAudioHashes.SHA256(ctx, decoderRegistry(), analysis.ResolvedSource{Name: filepath.Base(song.FilePath), Path: song.FilePath}, manifest.Audio.SampleRate, manifest.Audio.Channels)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(pcmHash, manifest.Source.AudioSHA256), nil
}
